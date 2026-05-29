package migrate

import (
	dg "github.com/dolan-in/dgman/v2"

	mg "github.com/matthewmcneely/modusgraph"
)

// MarshalSchema renders struct templates to a canonical Dgraph Schema Definition
// Language string — the frozen counterpart to a SchemaChange.Ensure list.
//
// Capture the result once at authoring time and store it in
// SchemaChange.EnsureSchema. Because the string is then applied and checksummed
// verbatim, the migration's schema never drifts as the live struct definitions
// evolve — unlike Ensure, which re-derives both at run time. This is what makes
// a baseline (or any additive schema in a shipped migration) reproducible and
// immutable.
//
// The output is deterministic: predicates and types are sorted into a fixed
// order so it is stable across runs regardless of Go map iteration order, and it
// is byte-identical to the rendering the runner checksums for an equivalent
// Ensure step at the same moment. Models are unwrapped (mg.UnwrapSchema) to match
// the schema client.UpdateSchema derives, so a frozen EnsureSchema and the live
// Ensure it was captured from apply the same predicates.
func MarshalSchema(models ...any) string {
	ts := dg.NewTypeSchema()
	unwrapped := make([]any, len(models))
	for i, m := range models {
		unwrapped[i] = mg.UnwrapSchema(m)
	}
	ts.Marshal("", unwrapped...)
	return canonicalTypeSchema(ts)
}
