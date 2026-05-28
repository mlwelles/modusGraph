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
