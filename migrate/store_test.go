package migrate

import (
	"context"
	"testing"
	"time"

	mg "github.com/matthewmcneely/modusgraph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds a file-backed embedded modusgraph client for integration
// tests. No live Dgraph cluster is required.
func newTestClient(t *testing.T) mg.Client {
	t.Helper()
	c, err := mg.NewClient("file://"+t.TempDir(), mg.WithAutoSchema(false))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = c.DropAll(context.Background())
		c.Close()
		mg.Shutdown()
	})
	return c
}

func TestRunIntegration_AppliesAndRecords(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	migrations := []Migration{
		{
			ID:     20260101000000,
			Name:   "add_widget",
			Schema: Schema{DQL: "widgetName: string @index(exact) ."},
		},
	}

	require.NoError(t, Run(ctx, c, migrations))

	v, err := Version(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, int64(20260101000000), v)

	// Second run is idempotent.
	require.NoError(t, Run(ctx, c, migrations))
	v2, _ := Version(ctx, c)
	assert.Equal(t, v, v2)
}

func TestRunIntegration_ChecksumMismatchFails(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	original := Migration{
		ID:     20260101000000,
		Name:   "add_widget",
		Schema: Schema{DQL: "widgetName: string @index(exact) ."},
	}
	require.NoError(t, Run(ctx, c, []Migration{original}))

	// Change the schema definition of an already-applied migration.
	tampered := original
	tampered.Schema.DQL = "widgetName: string @index(hash) ."
	err := Run(ctx, c, []Migration{tampered})
	var mismatch *ErrChecksumMismatch
	require.ErrorAs(t, err, &mismatch)
}

func TestRunIntegration_TypesMigrationProducesSchema(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	type gadget struct {
		UID   string   `json:"uid,omitempty"`
		DType []string `json:"dgraph.type,omitempty" dgraph:"Gadget"`
		Label string   `json:"gadgetLabel,omitempty" dgraph:"predicate=gadget_label index=exact"`
	}

	migrations := []Migration{{
		ID:     20260101000000,
		Name:   "add_gadget",
		Schema: Schema{Types: []any{&gadget{}}},
	}}
	require.NoError(t, Run(ctx, c, migrations))

	schema, err := c.GetSchema(ctx)
	require.NoError(t, err)
	assert.Contains(t, schema, "gadget_label")

	// A Types migration must be idempotent: re-running must not trip the
	// checksum guard (regression for non-deterministic Types checksums).
	require.NoError(t, Run(ctx, c, migrations), "re-running a Types migration must not fail the checksum check")
}

func TestStoreIntegration_LockBlocksWhenHeld(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	s := newDgraphStore(c)
	require.NoError(t, s.bootstrap(ctx))

	require.NoError(t, s.acquireLock(ctx), "first acquire should succeed")
	require.ErrorIs(t, s.acquireLock(ctx), ErrLocked, "second acquire should be blocked")

	require.NoError(t, s.releaseLock(ctx))
	require.NoError(t, s.acquireLock(ctx), "acquire after release should succeed")
}

func TestStoreIntegration_StaleLockStolen(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	s := newDgraphStore(c)
	s.lockTTL = 10 * time.Millisecond
	require.NoError(t, s.bootstrap(ctx))

	require.NoError(t, s.acquireLock(ctx))
	time.Sleep(20 * time.Millisecond) // let the lock go stale
	require.NoError(t, s.acquireLock(ctx), "stale lock should be stealable")
}

func TestDownIntegration_RollsBackAndRemovesRecord(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	m := Migration{
		ID:     20260101000000,
		Name:   "add_widget",
		Schema: Schema{DQL: "widgetName2: string @index(exact) ."},
		Down:   func(_ context.Context, _ mg.Client) error { return nil },
	}

	require.NoError(t, Run(ctx, c, []Migration{m}))
	v, _ := Version(ctx, c)
	require.Equal(t, m.ID, v)

	require.NoError(t, Down(ctx, c, []Migration{m}, 0))
	v2, _ := Version(ctx, c)
	assert.Equal(t, int64(0), v2)
}
