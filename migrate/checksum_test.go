package migrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checksumWidget struct {
	UID   string   `json:"uid,omitempty"`
	DType []string `json:"dgraph.type,omitempty"`
	Name  string   `json:"name,omitempty" dgraph:"index=exact"`
}

func TestChecksumSchema_DQL(t *testing.T) {
	s := Schema{DQL: "name: string @index(exact) ."}
	c1 := checksumSchema(s)
	c2 := checksumSchema(s)
	assert.Equal(t, c1, c2, "same DQL must produce same checksum")
	assert.Len(t, c1, 64, "SHA-256 hex is 64 chars")

	other := Schema{DQL: "age: int ."}
	assert.NotEqual(t, c1, checksumSchema(other), "different DQL must differ")
}

func TestChecksumSchema_Types(t *testing.T) {
	s := Schema{Types: []any{&checksumWidget{}}}
	c1 := checksumSchema(s)
	require.Len(t, c1, 64)
	assert.Equal(t, c1, checksumSchema(s), "same types must produce same checksum")

	// Zero Schema → distinct from a non-zero one.
	assert.NotEqual(t, c1, checksumSchema(Schema{}))
}

type checksumGadget struct {
	UID   string   `json:"uid,omitempty"`
	DType []string `json:"dgraph.type,omitempty"`
	Label string   `json:"gadgetLabel,omitempty" dgraph:"predicate=gadget_label index=exact"`
	Size  int      `json:"gadgetSize,omitempty" dgraph:"predicate=gadget_size index=int"`
}

type checksumSprocket struct {
	UID   string   `json:"uid,omitempty"`
	DType []string `json:"dgraph.type,omitempty"`
	Name  string   `json:"sprocketName,omitempty" dgraph:"predicate=sprocket_name index=term,trigram"`
	Teeth int      `json:"sprocketTeeth,omitempty" dgraph:"predicate=sprocket_teeth index=int"`
}

// TestChecksumSchema_TypesStableAcrossManyCalls guards against the dgman
// TypeSchema.String() map-ordering non-determinism: with several types and
// predicates, an order-sensitive hash would vary between calls.
func TestChecksumSchema_TypesStableAcrossManyCalls(t *testing.T) {
	s := Schema{Types: []any{&checksumWidget{}, &checksumGadget{}, &checksumSprocket{}}}
	want := checksumSchema(s)
	for i := 0; i < 50; i++ {
		assert.Equal(t, want, checksumSchema(s), "checksum must be deterministic across calls (iteration %d)", i)
	}

	// Type order in the slice must not change the checksum.
	reordered := Schema{Types: []any{&checksumSprocket{}, &checksumWidget{}, &checksumGadget{}}}
	assert.Equal(t, want, checksumSchema(reordered), "type order in the slice must not affect the checksum")
}
