package migrate

import (
	"context"
	"fmt"
	"sort"
	"time"

	mg "github.com/matthewmcneely/modusgraph"
)

// Schema is the schema portion of a Migration. Set exactly one field.
// Zero value means no schema change.
type Schema struct {
	// Types are struct templates applied via client.UpdateSchema (additive).
	Types []any
	// DQL is a raw Dgraph Schema Definition Language string applied via
	// client.AlterSchema (bypasses dgman, supports drops and renames).
	DQL string
}

// Migration is one ordered, run-once change. It is a value type — no I/O.
type Migration struct {
	// ID is a UTC timestamp YYYYMMDDHHMMSS uniquely identifying this migration.
	// Use timestamp IDs to avoid merge collisions across parallel branches.
	ID int64
	// Name is a human-readable label, e.g. "add_archive_status".
	Name string
	// Schema is the schema change for this migration.
	Schema Schema
	// Up is the optional data transform. It runs after Schema is applied.
	Up func(ctx context.Context, c mg.Client) error
	// Down is the optional rollback. May be nil; see ErrIrreversible.
	// Down does NOT auto-reverse Schema; callers must handle that explicitly.
	Down func(ctx context.Context, c mg.Client) error
}

// StatusEntry describes one migration's state.
type StatusEntry struct {
	ID       int64
	Name     string
	Applied  bool
	Checksum string // stored checksum; empty if not yet applied
	HasDrift bool   // true if stored checksum differs from computed
}

// StatusResult is the output of Status.
type StatusResult struct {
	Applied []StatusEntry
	Pending []StatusEntry
}

// Run applies all pending migrations in ascending ID order. It acquires a
// singleton lock before running and releases it on return. Migrations already
// recorded in the database are skipped; their stored checksums are validated
// against the registered list before any work starts.
func Run(ctx context.Context, c mg.Client, migrations []Migration) error {
	return run(ctx, c, newDgraphStore(c), migrations)
}

// run is the testable core of Run; it accepts a store so tests can inject a fake.
func run(ctx context.Context, c mg.Client, s store, migrations []Migration) error {
	if err := s.bootstrap(ctx); err != nil {
		return fmt.Errorf("migrate: bootstrap: %w", err)
	}

	if err := s.acquireLock(ctx); err != nil {
		return err
	}
	defer func() { _ = s.releaseLock(ctx) }()

	applied, err := s.loadApplied(ctx)
	if err != nil {
		return err
	}

	byID := make(map[int64]Migration, len(migrations))
	for _, m := range migrations {
		byID[m.ID] = m
	}

	// Validate checksums of all already-applied migrations still in the list.
	appliedSet := make(map[int64]struct{}, len(applied))
	for _, rec := range applied {
		reg, ok := byID[rec.ID]
		if !ok {
			return &ErrMissingApplied{ID: rec.ID}
		}
		if computed := checksumSchema(reg.Schema); rec.Checksum != computed {
			return &ErrChecksumMismatch{
				ID: rec.ID, Name: rec.Name,
				Stored: rec.Checksum, Computed: computed,
			}
		}
		appliedSet[rec.ID] = struct{}{}
	}

	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for _, m := range sorted {
		if _, done := appliedSet[m.ID]; done {
			continue
		}
		if err := applyOne(ctx, c, s, m); err != nil {
			return err
		}
	}
	return nil
}

// applyOne applies a single migration: schema first, then Up, then records it.
func applyOne(ctx context.Context, c mg.Client, s store, m Migration) error {
	start := time.Now()

	if err := applySchema(ctx, c, m.Schema); err != nil {
		return &ErrMigrationFailed{ID: m.ID, Name: m.Name, Err: fmt.Errorf("schema: %w", err)}
	}

	if m.Up != nil {
		if err := m.Up(ctx, c); err != nil {
			return &ErrMigrationFailed{ID: m.ID, Name: m.Name,
				Err: fmt.Errorf("Up: %w (schema was already applied and cannot be auto-rolled back)", err)}
		}
	}

	checksum := checksumSchema(m.Schema)
	durationMs := time.Since(start).Milliseconds()
	if err := s.save(ctx, m.ID, m.Name, checksum, durationMs); err != nil {
		return &ErrMigrationFailed{ID: m.ID, Name: m.Name, Err: fmt.Errorf("recording: %w", err)}
	}
	return nil
}

// applySchema applies the schema portion of a Migration.
func applySchema(ctx context.Context, c mg.Client, schema Schema) error {
	switch {
	case schema.DQL != "":
		return c.AlterSchema(ctx, schema.DQL)
	case len(schema.Types) > 0:
		return c.UpdateSchema(ctx, schema.Types...)
	}
	return nil // zero Schema = no-op
}

// Down rolls back all migrations with ID > toVersion, in descending ID order.
// All migrations in the range must have a non-nil Down; otherwise ErrIrreversible
// is returned before any work starts.
func Down(ctx context.Context, c mg.Client, migrations []Migration, toVersion int64) error {
	return down(ctx, c, newDgraphStore(c), migrations, toVersion)
}

func down(ctx context.Context, c mg.Client, s store, migrations []Migration, toVersion int64) error {
	if err := s.bootstrap(ctx); err != nil {
		return fmt.Errorf("migrate: bootstrap: %w", err)
	}

	if err := s.acquireLock(ctx); err != nil {
		return err
	}
	defer func() { _ = s.releaseLock(ctx) }()

	applied, err := s.loadApplied(ctx)
	if err != nil {
		return err
	}

	byID := make(map[int64]Migration, len(migrations))
	for _, m := range migrations {
		byID[m.ID] = m
	}

	var toRollback []appliedRecord
	for _, rec := range applied {
		if rec.ID > toVersion {
			toRollback = append(toRollback, rec)
		}
	}
	sort.Slice(toRollback, func(i, j int) bool { return toRollback[i].ID > toRollback[j].ID })

	// Validate all have Down before touching anything.
	for _, rec := range toRollback {
		m, ok := byID[rec.ID]
		if !ok {
			return &ErrMissingApplied{ID: rec.ID}
		}
		if m.Down == nil {
			return &ErrIrreversible{ID: m.ID, Name: m.Name}
		}
	}

	for _, rec := range toRollback {
		m := byID[rec.ID]
		if err := m.Down(ctx, c); err != nil {
			return &ErrMigrationFailed{ID: m.ID, Name: m.Name, Err: err}
		}
		if err := s.remove(ctx, m.ID); err != nil {
			return &ErrMigrationFailed{ID: m.ID, Name: m.Name,
				Err: fmt.Errorf("removing record: %w", err)}
		}
	}
	return nil
}

// Status returns the applied and pending migration states.
func Status(ctx context.Context, c mg.Client, migrations []Migration) (StatusResult, error) {
	return status(ctx, newDgraphStore(c), migrations)
}

func status(ctx context.Context, s store, migrations []Migration) (StatusResult, error) {
	if err := s.bootstrap(ctx); err != nil {
		return StatusResult{}, fmt.Errorf("migrate: bootstrap: %w", err)
	}

	applied, err := s.loadApplied(ctx)
	if err != nil {
		return StatusResult{}, err
	}

	appliedSet := make(map[int64]appliedRecord, len(applied))
	for _, rec := range applied {
		appliedSet[rec.ID] = rec
	}

	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	var result StatusResult
	for _, m := range sorted {
		entry := StatusEntry{ID: m.ID, Name: m.Name}
		if rec, ok := appliedSet[m.ID]; ok {
			entry.Applied = true
			entry.Checksum = rec.Checksum
			entry.HasDrift = rec.Checksum != checksumSchema(m.Schema)
			result.Applied = append(result.Applied, entry)
		} else {
			result.Pending = append(result.Pending, entry)
		}
	}
	return result, nil
}

// Version returns the highest applied migration ID, or 0 if none.
func Version(ctx context.Context, c mg.Client) (int64, error) {
	return version(ctx, newDgraphStore(c))
}

func version(ctx context.Context, s store) (int64, error) {
	if err := s.bootstrap(ctx); err != nil {
		return 0, fmt.Errorf("migrate: bootstrap: %w", err)
	}
	applied, err := s.loadApplied(ctx)
	if err != nil {
		return 0, err
	}
	var max int64
	for _, r := range applied {
		if r.ID > max {
			max = r.ID
		}
	}
	return max, nil
}
