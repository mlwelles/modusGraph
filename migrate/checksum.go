package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	dg "github.com/dolan-in/dgman/v2"
)

// checksumSchema computes a stable SHA-256 hex string over the schema portion
// of a Migration. Only the schema input is hashed — never the Up/Down closures.
// For Schema.DQL the raw string is hashed; for Schema.Types a canonical
// (sorted) rendering of the dgman schema is hashed. A zero-value Schema (no
// schema change) hashes to a well-known constant.
func checksumSchema(s Schema) string {
	h := sha256.New()
	switch {
	case s.DQL != "":
		h.Write([]byte("dql:" + s.DQL))
	case len(s.Types) > 0:
		ts := dg.NewTypeSchema()
		ts.Marshal("", s.Types...)
		h.Write([]byte("types:" + canonicalTypeSchema(ts)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalTypeSchema renders a dgman TypeSchema deterministically. dgman's own
// TypeSchema.String() ranges over Go maps (TypeMap, SchemaMap) in randomized
// order, so its output cannot be hashed stably. This sorts predicates and types
// (and each type's predicate set) into a fixed order.
func canonicalTypeSchema(ts *dg.TypeSchema) string {
	var b strings.Builder

	preds := make([]string, 0, len(ts.Schema))
	for _, s := range ts.Schema {
		preds = append(preds, s.String())
	}
	sort.Strings(preds)
	for _, p := range preds {
		b.WriteString(p)
		b.WriteByte('\n')
	}

	typeNames := make([]string, 0, len(ts.Types))
	for name := range ts.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		b.WriteString("type ")
		b.WriteString(name)
		b.WriteString(" {")
		predNames := make([]string, 0, len(ts.Types[name]))
		for pn := range ts.Types[name] {
			predNames = append(predNames, pn)
		}
		sort.Strings(predNames)
		for _, pn := range predNames {
			b.WriteByte('\n')
			b.WriteString(pn)
		}
		b.WriteString("\n}\n")
	}
	return b.String()
}
