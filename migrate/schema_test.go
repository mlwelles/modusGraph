package migrate

import (
	"context"
	"testing"

	dg "github.com/dolan-in/dgman/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalSchema_DeterministicAndCanonical(t *testing.T) {
	got := MarshalSchema(&tree{})
	require.NotEmpty(t, got)
	assert.Contains(t, got, "height:")
	// Stable across calls — guards against dgman's map-order non-determinism.
	assert.Equal(t, got, MarshalSchema(&tree{}))
	// Byte-identical to the rendering an equivalent Ensure step is checksummed
	// from, so freezing a live Ensure list captures exactly what it would apply.
	ts := dg.NewTypeSchema()
	ts.Marshal("", &tree{})
	assert.Equal(t, canonicalTypeSchema(ts), got)
}

func TestEnsureSchemaChecksum_DependsOnlyOnString(t *testing.T) {
	frozen := MarshalSchema(&tree{})
	step := Step{Name: "baseline", Schema: SchemaChange{EnsureSchema: frozen}}
	// Deterministic for a fixed string.
	assert.Equal(t, stepChecksum(0, step), stepChecksum(0, step))
	// A frozen EnsureSchema and a live Ensure over the same structs hash in
	// different domains, so they are distinct steps even at capture time.
	ensure := Step{Name: "baseline", Schema: SchemaChange{Ensure: []any{&tree{}}}}
	assert.NotEqual(t, stepChecksum(0, ensure), stepChecksum(0, step))
	// Changing the frozen string changes the checksum.
	other := Step{Name: "baseline", Schema: SchemaChange{EnsureSchema: frozen + "\nextra: string ."}}
	assert.NotEqual(t, stepChecksum(0, step), stepChecksum(0, other))
}

func TestEnsureSchema_AppliesAndIsIdempotent(t *testing.T) {
	c := newEmbeddedClient(t)
	ctx := context.Background()
	m := Migration{
		ID:   20260104000000,
		Name: "frozen_baseline",
		Steps: []Step{
			{Name: "baseline_schema", Schema: SchemaChange{EnsureSchema: MarshalSchema(&tree{})}},
		},
	}
	require.NoError(t, Run(ctx, c, []Migration{m}))
	require.NoError(t, Run(ctx, c, []Migration{m}), "re-running a frozen baseline must be idempotent")

	v, err := Version(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, int64(20260104000000), v)

	sch, err := c.GetSchema(ctx)
	require.NoError(t, err)
	assert.Contains(t, sch, "height", "frozen EnsureSchema must create the predicate")
}
