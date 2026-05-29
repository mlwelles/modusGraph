package migrate

import (
	"context"
	"fmt"
	"sort"
	"time"

	mg "github.com/matthewmcneely/modusgraph"
)

// SchemaChange is the schema portion of a Step. Set exactly one field; the zero
// value means the step makes no schema change.
type SchemaChange struct {
	// Ensure are struct templates whose derived schema is applied additively via
	// client.UpdateSchema (idempotent: ensures the predicates/types exist).
	Ensure []any
	// Alter is a raw Dgraph schema string applied via client.AlterSchema, for
	// changes Ensure cannot express: rename, drop, retype.
	Alter string
}

// Step is the atomic, independently-checkpointed unit of a Migration.
//
// On up the runner applies Schema (if set), then runs Up (if set), then records
// the step. Because a schema Alter auto-commits and a step's record is written
// separately, every Step MUST be idempotent: re-running it after a crash must
// converge to the same end state.
type Step struct {
	// Name is a stable, human-readable label, unique within the migration. It is
	// the checkpoint identity; renaming or reordering a shipped step trips a
	// checksum mismatch on resume.
	Name string
	// Schema is the optional schema change for this step.
	Schema SchemaChange
	// Up is the optional data transform, run after Schema is applied.
	Up func(ctx context.Context, c mg.Client) error
	// Down is the optional inverse. If any step in a down range has a nil Down,
	// the whole range is refused with ErrIrreversible.
	Down func(ctx context.Context, c mg.Client) error
}

// Migration is one ordered, run-once change identified by a UTC timestamp ID
// (YYYYMMDDHHMMSS). Use timestamp IDs to avoid merge collisions across branches.
type Migration struct {
	ID    int64
	Name  string
	Steps []Step
}

// StatusEntry describes one migration's state for Status.
type StatusEntry struct {
	ID            int64
	Name          string
	Applied       bool   // all steps applied and the migration record written
	StepsApplied  int    // number of completed steps (for in-progress migrations)
	StepsTotal    int    // number of registered steps
	Checksum      string // stored migration checksum; empty unless fully applied
	HasDrift      bool   // stored checksum differs from computed (fully applied only)
}

// StatusResult is the output of Status.
type StatusResult struct {
	Applied    []StatusEntry
	InProgress []StatusEntry
	Pending    []StatusEntry
}

// Run applies all pending migrations in ascending ID order, resuming any
// partially-applied migration at its next un-applied step. It acquires a
// singleton lock for the duration.
func Run(ctx context.Context, c mg.Client, migrations []Migration) error {
	return run(ctx, c, newDgraphStore(c), migrations)
}

func run(ctx context.Context, c mg.Client, s store, migrations []Migration) error {
	if err := s.bootstrap(ctx); err != nil {
		return fmt.Errorf("migrate: bootstrap: %w", err)
	}
	if err := s.acquireLock(ctx); err != nil {
		return err
	}
	defer func() { _ = s.releaseLock(ctx) }()

	applied, err := s.loadAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	byID := make(map[int64]Migration, len(migrations))
	for _, m := range migrations {
		byID[m.ID] = m
	}

	appliedSet := make(map[int64]struct{}, len(applied))
	for _, rec := range applied {
		reg, ok := byID[rec.ID]
		if !ok {
			return &ErrMissingApplied{ID: rec.ID}
		}
		if computed := migrationChecksum(reg); computed != rec.Checksum {
			return &ErrChecksumMismatch{ID: rec.ID, Name: rec.Name, Stored: rec.Checksum, Computed: computed}
		}
		appliedSet[rec.ID] = struct{}{}
	}

	sorted := append([]Migration(nil), migrations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for _, m := range sorted {
		if _, done := appliedSet[m.ID]; done {
			continue
		}
		if err := applyMigration(ctx, c, s, m); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs the un-applied steps of one migration in order, then
// records the migration. It validates any already-recorded steps' checksums
// (mid-flight immutability) before running.
func applyMigration(ctx context.Context, c mg.Client, s store, m Migration) error {
	stepRecs, err := s.loadStepRecords(ctx, m.ID)
	if err != nil {
		return err
	}
	completed := make(map[int]struct{}, len(stepRecs))
	for _, r := range stepRecs {
		if r.Index >= len(m.Steps) {
			return &ErrChecksumMismatch{ID: m.ID, Name: m.Name, Stored: r.Checksum, Computed: "<step removed>"}
		}
		if computed := stepChecksum(r.Index, m.Steps[r.Index]); computed != r.Checksum {
			return &ErrChecksumMismatch{ID: m.ID, Name: m.Name, Stored: r.Checksum, Computed: computed}
		}
		completed[r.Index] = struct{}{}
	}

	start := time.Now()
	for i, step := range m.Steps {
		if _, ok := completed[i]; ok {
			continue
		}
		if err := applyStep(ctx, c, s, m, i, step); err != nil {
			return err
		}
	}

	if err := s.saveMigration(ctx, m.ID, m.Name, migrationChecksum(m), time.Since(start).Milliseconds()); err != nil {
		return &ErrMigrationFailed{ID: m.ID, Name: m.Name, Err: fmt.Errorf("recording: %w", err)}
	}
	return nil
}

// applyStep applies one step's schema (if any), runs its Up (if any), then
// records the step. Schema applies before Up so a data transform sees the new
// predicates; both must be idempotent (see Step doc).
func applyStep(ctx context.Context, c mg.Client, s store, m Migration, index int, step Step) error {
	start := time.Now()
	if err := applySchemaChange(ctx, c, step.Schema); err != nil {
		return &ErrStepFailed{MigrationID: m.ID, StepName: step.Name, Index: index, Err: fmt.Errorf("schema: %w", err)}
	}
	if step.Up != nil {
		if err := step.Up(ctx, c); err != nil {
			return &ErrStepFailed{MigrationID: m.ID, StepName: step.Name, Index: index, Err: err}
		}
	}
	if err := s.saveStep(ctx, m.ID, step.Name, index, stepChecksum(index, step), time.Since(start).Milliseconds()); err != nil {
		return &ErrStepFailed{MigrationID: m.ID, StepName: step.Name, Index: index, Err: fmt.Errorf("recording: %w", err)}
	}
	return nil
}

func applySchemaChange(ctx context.Context, c mg.Client, sc SchemaChange) error {
	switch {
	case sc.Alter != "":
		return c.AlterSchema(ctx, sc.Alter)
	case len(sc.Ensure) > 0:
		return c.UpdateSchema(ctx, sc.Ensure...)
	}
	return nil
}

// Down rolls back every applied migration with ID > toVersion, in descending
// ID order, running each migration's steps' Down in reverse step order. The
// whole range is validated first: if any step has a nil Down, no work is done.
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

	applied, err := s.loadAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	byID := make(map[int64]Migration, len(migrations))
	for _, m := range migrations {
		byID[m.ID] = m
	}

	var toRollback []migrationRec
	for _, rec := range applied {
		if rec.ID > toVersion {
			toRollback = append(toRollback, rec)
		}
	}
	sort.Slice(toRollback, func(i, j int) bool { return toRollback[i].ID > toRollback[j].ID })

	for _, rec := range toRollback {
		m, ok := byID[rec.ID]
		if !ok {
			return &ErrMissingApplied{ID: rec.ID}
		}
		for _, st := range m.Steps {
			if st.Down == nil {
				return &ErrIrreversible{ID: m.ID, Name: m.Name}
			}
		}
	}

	for _, rec := range toRollback {
		m := byID[rec.ID]
		for i := len(m.Steps) - 1; i >= 0; i-- {
			if err := m.Steps[i].Down(ctx, c); err != nil {
				return &ErrStepFailed{MigrationID: m.ID, StepName: m.Steps[i].Name, Index: i, Err: err}
			}
		}
		if err := s.removeMigration(ctx, m.ID); err != nil {
			return &ErrMigrationFailed{ID: m.ID, Name: m.Name, Err: fmt.Errorf("removing records: %w", err)}
		}
	}
	return nil
}

// Status returns applied, in-progress, and pending migration states.
func Status(ctx context.Context, c mg.Client, migrations []Migration) (StatusResult, error) {
	return status(ctx, newDgraphStore(c), migrations)
}

func status(ctx context.Context, s store, migrations []Migration) (StatusResult, error) {
	if err := s.bootstrap(ctx); err != nil {
		return StatusResult{}, fmt.Errorf("migrate: bootstrap: %w", err)
	}
	applied, err := s.loadAppliedMigrations(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	appliedSet := make(map[int64]migrationRec, len(applied))
	for _, rec := range applied {
		appliedSet[rec.ID] = rec
	}

	sorted := append([]Migration(nil), migrations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	var res StatusResult
	for _, m := range sorted {
		entry := StatusEntry{ID: m.ID, Name: m.Name, StepsTotal: len(m.Steps)}
		if rec, ok := appliedSet[m.ID]; ok {
			computed := migrationChecksum(m)
			entry.Applied = true
			entry.StepsApplied = len(m.Steps)
			entry.Checksum = rec.Checksum
			entry.HasDrift = rec.Checksum != computed
			res.Applied = append(res.Applied, entry)
			continue
		}
		stepRecs, err := s.loadStepRecords(ctx, m.ID)
		if err != nil {
			return StatusResult{}, err
		}
		if len(stepRecs) > 0 {
			entry.StepsApplied = len(stepRecs)
			res.InProgress = append(res.InProgress, entry)
		} else {
			res.Pending = append(res.Pending, entry)
		}
	}
	return res, nil
}

// Version returns the highest fully-applied migration ID, or 0 if none.
func Version(ctx context.Context, c mg.Client) (int64, error) {
	return version(ctx, newDgraphStore(c))
}

func version(ctx context.Context, s store) (int64, error) {
	if err := s.bootstrap(ctx); err != nil {
		return 0, fmt.Errorf("migrate: bootstrap: %w", err)
	}
	applied, err := s.loadAppliedMigrations(ctx)
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
