package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	mg "github.com/matthewmcneely/modusgraph"
)

// store is the persistence layer for migration state. It is the only layer
// that knows the MigrationRecord and MigrationLock Dgraph types.
type store interface {
	// bootstrap ensures MigrationRecord and MigrationLock schemas exist in Dgraph.
	bootstrap(ctx context.Context) error
	// loadApplied returns all applied migration records, sorted by ID ascending.
	loadApplied(ctx context.Context) ([]appliedRecord, error)
	// save records a migration as successfully applied.
	save(ctx context.Context, id int64, name, checksum string, durationMs int64) error
	// remove deletes a migration record (used by Down).
	remove(ctx context.Context, id int64) error
	// acquireLock attempts to acquire the singleton migration lock.
	// Returns ErrLocked if a fresh lock (within TTL) is already held.
	acquireLock(ctx context.Context) error
	// releaseLock releases the singleton lock.
	releaseLock(ctx context.Context) error
}

// appliedRecord is the in-memory representation of a MigrationRecord node.
type appliedRecord struct {
	UID        string    `json:"uid,omitempty"`
	ID         int64     `json:"migrationId,omitempty"`
	Name       string    `json:"migrationName,omitempty"`
	AppliedAt  time.Time `json:"migrationAppliedAt,omitempty"`
	Checksum   string    `json:"migrationChecksum,omitempty"`
	DurationMs int64     `json:"migrationDurationMs,omitempty"`
}

// MigrationRecord is the Dgraph node type for a persisted migration record.
// Uses migration_* predicate prefix to avoid collisions with application predicates.
type MigrationRecord struct {
	UID        string    `json:"uid,omitempty"`
	DType      []string  `json:"dgraph.type,omitempty" dgraph:"MigrationRecord"`
	ID         int64     `json:"migrationId,omitempty" dgraph:"predicate=migration_id index=int"`
	Name       string    `json:"migrationName,omitempty" dgraph:"predicate=migration_name"`
	AppliedAt  time.Time `json:"migrationAppliedAt,omitempty" dgraph:"predicate=migration_applied_at index=hour"`
	Checksum   string    `json:"migrationChecksum,omitempty" dgraph:"predicate=migration_checksum"`
	DurationMs int64     `json:"migrationDurationMs,omitempty" dgraph:"predicate=migration_duration_ms"`
}

// MigrationLock is the Dgraph node type for the singleton migration lock.
type MigrationLock struct {
	UID      string    `json:"uid,omitempty"`
	DType    []string  `json:"dgraph.type,omitempty" dgraph:"MigrationLock"`
	LockKey  string    `json:"migrationLockKey,omitempty" dgraph:"predicate=migration_lock_key index=exact"`
	LockedAt time.Time `json:"migrationLockedAt,omitempty" dgraph:"predicate=migration_locked_at"`
}

const lockSingleton = "singleton"

// defaultLockTTL is how long a lock is considered fresh. A lock older than
// this is assumed stale (crashed runner) and may be stolen.
const defaultLockTTL = 5 * time.Minute

// dgraphStore is the production store implementation backed by a Dgraph database.
type dgraphStore struct {
	conn    mg.Client
	lockTTL time.Duration
}

func newDgraphStore(conn mg.Client) *dgraphStore {
	return &dgraphStore{conn: conn, lockTTL: defaultLockTTL}
}

func (s *dgraphStore) bootstrap(ctx context.Context) error {
	return s.conn.UpdateSchema(ctx, &MigrationRecord{}, &MigrationLock{})
}

func (s *dgraphStore) loadApplied(ctx context.Context) ([]appliedRecord, error) {
	raw, err := s.conn.QueryRaw(ctx,
		`{ records(func: type(MigrationRecord), orderasc: migration_id) {
			uid
			migrationId: migration_id
			migrationName: migration_name
			migrationAppliedAt: migration_applied_at
			migrationChecksum: migration_checksum
			migrationDurationMs: migration_duration_ms
		} }`, nil)
	if err != nil {
		return nil, fmt.Errorf("loading applied migrations: %w", err)
	}

	var result struct {
		Records []appliedRecord `json:"records"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing migration records: %w", err)
	}
	return result.Records, nil
}

func (s *dgraphStore) save(ctx context.Context, id int64, name, checksum string, durationMs int64) error {
	rec := &MigrationRecord{
		DType:      []string{"MigrationRecord"},
		ID:         id,
		Name:       name,
		AppliedAt:  time.Now().UTC(),
		Checksum:   checksum,
		DurationMs: durationMs,
	}
	return s.conn.Insert(ctx, rec)
}

func (s *dgraphStore) remove(ctx context.Context, id int64) error {
	raw, err := s.conn.QueryRaw(ctx,
		fmt.Sprintf(`{ r(func: type(MigrationRecord)) @filter(eq(migration_id, %d)) { uid } }`, id), nil)
	if err != nil {
		return fmt.Errorf("finding migration record %d: %w", id, err)
	}

	var result struct {
		R []struct {
			UID string `json:"uid"`
		} `json:"r"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parsing remove query: %w", err)
	}
	if len(result.R) == 0 {
		return nil // already gone
	}
	return s.conn.Delete(ctx, []string{result.R[0].UID})
}

// acquireLock tries to acquire the singleton migration lock. If a fresh lock
// (within TTL) already exists, it returns ErrLocked. A stale lock is stolen.
//
// Note: there is a small TOCTOU window between the check and the write —
// acceptable for a CLI/operator tool. For production, run migrations from a
// single runner (CI/CD step or init container), not concurrently.
func (s *dgraphStore) acquireLock(ctx context.Context) error {
	now := time.Now().UTC()
	threshold := now.Add(-s.lockTTL)

	raw, err := s.conn.QueryRaw(ctx,
		`{ lock(func: type(MigrationLock)) @filter(eq(migration_lock_key, "singleton")) {
			uid
			migrationLockedAt: migration_locked_at
		} }`, nil)
	if err != nil {
		return fmt.Errorf("checking migration lock: %w", err)
	}

	var result struct {
		Lock []struct {
			UID      string    `json:"uid"`
			LockedAt time.Time `json:"migrationLockedAt"`
		} `json:"lock"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parsing lock query: %w", err)
	}

	if len(result.Lock) > 0 && result.Lock[0].LockedAt.After(threshold) {
		return ErrLocked
	}

	if len(result.Lock) > 0 {
		// Steal stale lock: update lockedAt in place.
		return s.conn.Update(ctx, &MigrationLock{
			UID:      result.Lock[0].UID,
			DType:    []string{"MigrationLock"},
			LockKey:  lockSingleton,
			LockedAt: now,
		})
	}

	// No lock exists: create it.
	return s.conn.Insert(ctx, &MigrationLock{
		DType:    []string{"MigrationLock"},
		LockKey:  lockSingleton,
		LockedAt: now,
	})
}

func (s *dgraphStore) releaseLock(ctx context.Context) error {
	raw, err := s.conn.QueryRaw(ctx,
		`{ lock(func: type(MigrationLock)) @filter(eq(migration_lock_key, "singleton")) { uid } }`, nil)
	if err != nil {
		return fmt.Errorf("querying migration lock for release: %w", err)
	}

	var result struct {
		Lock []struct {
			UID string `json:"uid"`
		} `json:"lock"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parsing lock release query: %w", err)
	}
	if len(result.Lock) == 0 {
		return nil
	}
	return s.conn.Delete(ctx, []string{result.Lock[0].UID})
}
