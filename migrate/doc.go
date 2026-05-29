// Package migrate runs versioned, run-once schema-and-data migrations against a
// modusgraph.Client.
//
// # Model
//
// A Migration is an ordered list of named Steps, identified by a UTC timestamp
// ID (YYYYMMDDHHMMSS). Each Step carries an optional SchemaChange (Ensure for
// additive struct-derived schema, or Alter for raw DQL: rename/drop/retype) and
// an optional Up data transform. On up, the runner applies a step's schema, runs
// its Up, then records a checkpoint; on a later run it skips already-recorded
// steps and resumes at the next one.
//
// # The one rule that matters
//
// Every Step must be idempotent. A Dgraph Alter auto-commits and cannot share a
// transaction with the step's data work or its checkpoint write, so a crash can
// leave a step's effect applied but unrecorded. On resume the step runs again —
// it must converge to the same end state. Dgraph's add-predicate, add-index,
// set-type, and drop-predicate are naturally idempotent; author your Up funcs
// (upsert by UID, not blind insert) to be the same.
//
// # Reversal
//
// Down is optional per step. Down(toVersion) rolls back applied migrations with
// ID > toVersion in descending order, running each migration's steps' Down in
// reverse step order. If any step in the range has a nil Down, the whole range
// is refused with ErrIrreversible before any work.
//
// Migrations are immutable once applied: editing a shipped migration's schema
// portion trips ErrChecksumMismatch. Correct mistakes by writing a new migration.
package migrate
