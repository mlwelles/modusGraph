package migrate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mg "github.com/matthewmcneely/modusgraph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var bg = context.Background()

func makeMigration(id int64, name string) Migration {
	return Migration{
		ID:     id,
		Name:   name,
		Schema: Schema{DQL: fmt.Sprintf("%s: string .", name)},
	}
}

func makeMigrationWithUp(id int64, name string, upErr error) Migration {
	m := makeMigration(id, name)
	m.Up = func(_ context.Context, _ mg.Client) error { return upErr }
	return m
}

func makeMigrationWithDown(id int64, name string) Migration {
	m := makeMigration(id, name)
	m.Down = func(_ context.Context, _ mg.Client) error { return nil }
	return m
}

// --- Run ---

func TestRun_AppliesPending(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	migrations := []Migration{
		makeMigration(20260101000000, "first"),
		makeMigration(20260102000000, "second"),
	}

	require.NoError(t, run(bg, c, s, migrations))

	applied, _ := s.loadApplied(bg)
	require.Len(t, applied, 2)
	assert.Equal(t, int64(20260101000000), applied[0].ID)
	assert.Equal(t, int64(20260102000000), applied[1].ID)
}

func TestRun_SkipsAlreadyApplied(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	m := makeMigration(20260101000000, "first")
	_ = s.save(bg, m.ID, m.Name, checksumSchema(m.Schema), 0)

	require.NoError(t, run(bg, c, s, []Migration{m}))

	applied, _ := s.loadApplied(bg)
	assert.Len(t, applied, 1, "should not double-apply")
}

func TestRun_FailsOnChecksumMismatch(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	m := makeMigration(20260101000000, "first")
	_ = s.save(bg, m.ID, m.Name, "deadbeefdeadbeef", 0) // wrong checksum

	err := run(bg, c, s, []Migration{m})
	var mismatch *ErrChecksumMismatch
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, int64(20260101000000), mismatch.ID)
}

func TestRun_FailsOnMissingApplied(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	_ = s.save(bg, 20260101000000, "deleted", "anychecksum", 0)

	err := run(bg, c, s, []Migration{makeMigration(20260102000000, "other")})
	var missing *ErrMissingApplied
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, int64(20260101000000), missing.ID)
}

func TestRun_StopsOnUpFailure(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	upErr := errors.New("data transform exploded")
	migrations := []Migration{
		makeMigration(20260101000000, "first"),
		makeMigrationWithUp(20260102000000, "second", upErr),
		makeMigration(20260103000000, "third"),
	}

	err := run(bg, c, s, migrations)
	var failed *ErrMigrationFailed
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, int64(20260102000000), failed.ID)

	applied, _ := s.loadApplied(bg)
	assert.Len(t, applied, 1)
	assert.Equal(t, int64(20260101000000), applied[0].ID)
}

func TestRun_ReturnsErrLockedWhenLockHeld(t *testing.T) {
	s := newFakeStore()
	s.lockErr = ErrLocked
	c := &stubClient{}

	err := run(bg, c, s, []Migration{makeMigration(20260101000000, "first")})
	require.ErrorIs(t, err, ErrLocked)
}

func TestRun_AppliesSchemaTypesMigration(t *testing.T) {
	type thing struct {
		UID   string   `json:"uid,omitempty"`
		DType []string `json:"dgraph.type,omitempty"`
		Name  string   `json:"name,omitempty" dgraph:"index=exact"`
	}

	s := newFakeStore()
	c := &stubClient{}
	m := Migration{
		ID:     20260101000001,
		Name:   "add_thing",
		Schema: Schema{Types: []any{&thing{}}},
	}

	require.NoError(t, run(bg, c, s, []Migration{m}))
	assert.Len(t, c.appliedTypes, 1, "UpdateSchema should have been called")
}

func TestRun_AppliesDQLMigration(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	m := makeMigration(20260101000002, "dqlchange")

	require.NoError(t, run(bg, c, s, []Migration{m}))
	require.Len(t, c.alteredDQL, 1, "AlterSchema should have been called")
	assert.Contains(t, c.alteredDQL[0], "dqlchange")
}

// --- Down ---

func TestDown_RollsBackInDescendingOrder(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	m1 := makeMigrationWithDown(20260101000000, "first")
	m2 := makeMigrationWithDown(20260102000000, "second")
	m3 := makeMigrationWithDown(20260103000000, "third")

	for _, m := range []Migration{m1, m2, m3} {
		_ = s.save(bg, m.ID, m.Name, checksumSchema(m.Schema), 0)
	}

	require.NoError(t, down(bg, c, s, []Migration{m1, m2, m3}, m1.ID))

	applied, _ := s.loadApplied(bg)
	require.Len(t, applied, 1)
	assert.Equal(t, m1.ID, applied[0].ID, "only first should remain")
}

func TestDown_RefusesIfAnyMigrationLacksDown(t *testing.T) {
	s := newFakeStore()
	c := &stubClient{}
	m1 := makeMigrationWithDown(20260101000000, "first")
	m2 := makeMigration(20260102000000, "second") // no Down

	for _, m := range []Migration{m1, m2} {
		_ = s.save(bg, m.ID, m.Name, checksumSchema(m.Schema), 0)
	}

	err := down(bg, c, s, []Migration{m1, m2}, 0)
	var irreversible *ErrIrreversible
	require.ErrorAs(t, err, &irreversible)
	assert.Equal(t, m2.ID, irreversible.ID)

	applied, _ := s.loadApplied(bg)
	assert.Len(t, applied, 2, "refused before doing any work")
}

// --- Status ---

func TestStatus_ShowsAppliedAndPending(t *testing.T) {
	s := newFakeStore()
	m1 := makeMigration(20260101000000, "first")
	m2 := makeMigration(20260102000000, "second")
	_ = s.save(bg, m1.ID, m1.Name, checksumSchema(m1.Schema), 0)

	result, err := status(bg, s, []Migration{m1, m2})
	require.NoError(t, err)
	assert.Len(t, result.Applied, 1)
	assert.Len(t, result.Pending, 1)
	assert.Equal(t, m1.ID, result.Applied[0].ID)
	assert.Equal(t, m2.ID, result.Pending[0].ID)
}

func TestStatus_DetectsChecksumDrift(t *testing.T) {
	s := newFakeStore()
	m := makeMigration(20260101000000, "first")
	_ = s.save(bg, m.ID, m.Name, "oldbadchecksum", 0)

	result, err := status(bg, s, []Migration{m})
	require.NoError(t, err)
	require.Len(t, result.Applied, 1)
	assert.True(t, result.Applied[0].HasDrift)
}

func TestVersion_ReturnsHighestApplied(t *testing.T) {
	s := newFakeStore()
	_ = s.save(bg, 20260101000000, "first", "x", 0)
	_ = s.save(bg, 20260103000000, "third", "y", 0)

	v, err := version(bg, s)
	require.NoError(t, err)
	assert.Equal(t, int64(20260103000000), v)
}
