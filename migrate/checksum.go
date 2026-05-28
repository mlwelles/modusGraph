package migrate

import (
	"crypto/sha256"
	"encoding/hex"

	dg "github.com/dolan-in/dgman/v2"
)

// checksumSchema computes a stable SHA-256 hex string over the schema portion
// of a Migration. Only the schema input is hashed — never the Up/Down closures.
// For Schema.DQL the raw string is hashed; for Schema.Types the dgman canonical
// schema string is hashed. A zero-value Schema (no schema change) hashes to a
// well-known constant.
func checksumSchema(s Schema) string {
	h := sha256.New()
	switch {
	case s.DQL != "":
		h.Write([]byte("dql:" + s.DQL))
	case len(s.Types) > 0:
		ts := dg.NewTypeSchema()
		ts.Marshal("", s.Types...)
		h.Write([]byte("types:" + ts.String()))
	}
	return hex.EncodeToString(h.Sum(nil))
}
