// Package migrate provides versioned, run-once schema and data migrations
// for modusGraph databases. Each Migration carries a schema change and an
// optional data-transform function, identified by a UTC timestamp ID.
package migrate

import (
	"errors"
	"fmt"
)

// ErrLocked is returned when another runner holds the migration lock.
var ErrLocked = errors.New("migrate: another migration runner holds the lock")

// ErrChecksumMismatch is returned when an already-applied migration's schema
// portion has been edited since it ran.
type ErrChecksumMismatch struct {
	ID       int64
	Name     string
	Stored   string
	Computed string
}

func (e *ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("migrate: checksum mismatch for migration %d (%s): stored=%s computed=%s — migrations are immutable once applied",
		e.ID, e.Name, short(e.Stored), short(e.Computed))
}

// short returns the first 8 chars of a checksum for compact error messages,
// or the whole string if shorter.
func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}

// ErrMissingApplied is returned when a migration ID recorded in the database
// is absent from the registered migration list (i.e. a shipped migration was deleted).
type ErrMissingApplied struct {
	ID int64
}

func (e *ErrMissingApplied) Error() string {
	return fmt.Sprintf("migrate: applied migration %d is missing from the registered list — never delete a shipped migration", e.ID)
}

// ErrIrreversible is returned when a requested down migration includes a
// migration with a nil Down function.
type ErrIrreversible struct {
	ID   int64
	Name string
}

func (e *ErrIrreversible) Error() string {
	return fmt.Sprintf("migrate: migration %d (%s) has no Down function — cannot reverse", e.ID, e.Name)
}

// ErrMigrationFailed wraps a migration execution error with the offending ID.
type ErrMigrationFailed struct {
	ID   int64
	Name string
	Err  error
}

func (e *ErrMigrationFailed) Error() string {
	return fmt.Sprintf("migrate: migration %d (%s) failed: %v", e.ID, e.Name, e.Err)
}

func (e *ErrMigrationFailed) Unwrap() error { return e.Err }
