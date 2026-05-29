package migrate

import (
	"errors"
	"fmt"
)

// ErrLocked is returned when another runner holds the migration lock.
var ErrLocked = errors.New("migrate: another migration runner holds the lock")

// ErrChecksumMismatch is returned when an applied migration's (or in-flight
// step's) schema portion has been edited since it ran. Migrations are immutable
// once applied; correct mistakes with a new migration.
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

// ErrMissingApplied is returned when a migration ID recorded in the database is
// absent from the registered list (a shipped migration was deleted).
type ErrMissingApplied struct{ ID int64 }

func (e *ErrMissingApplied) Error() string {
	return fmt.Sprintf("migrate: applied migration %d is missing from the registered list — never delete a shipped migration", e.ID)
}

// ErrStepRemoved is returned when a recorded step's ordinal no longer exists
// in the registered migration (a shipped step was deleted).
type ErrStepRemoved struct {
	MigrationID int64
	Name        string
	Index       int
}

func (e *ErrStepRemoved) Error() string {
	return fmt.Sprintf("migrate: migration %d step %d (%s) was recorded but no longer exists — never delete a shipped step", e.MigrationID, e.Index, e.Name)
}

// ErrIrreversible is returned when a requested down range includes a migration
// with any step that has a nil Down.
type ErrIrreversible struct {
	ID   int64
	Name string
}

func (e *ErrIrreversible) Error() string {
	return fmt.Sprintf("migrate: migration %d (%s) has a step with no Down — cannot reverse", e.ID, e.Name)
}

// ErrMigrationFailed wraps a migration-level failure (e.g. recording the
// migration after all steps ran) with the offending ID.
type ErrMigrationFailed struct {
	ID   int64
	Name string
	Err  error
}

func (e *ErrMigrationFailed) Error() string {
	return fmt.Sprintf("migrate: migration %d (%s) failed: %v", e.ID, e.Name, e.Err)
}

func (e *ErrMigrationFailed) Unwrap() error { return e.Err }

// ErrStepFailed wraps a step-level failure with the migration ID and the step's
// name and ordinal, so the operator knows exactly where a run stopped.
type ErrStepFailed struct {
	MigrationID int64
	StepName    string
	Index       int
	Err         error
}

func (e *ErrStepFailed) Error() string {
	return fmt.Sprintf("migrate: migration %d step %d (%s) failed: %v", e.MigrationID, e.Index, e.StepName, e.Err)
}

func (e *ErrStepFailed) Unwrap() error { return e.Err }

func short(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}
