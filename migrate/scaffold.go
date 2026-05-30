package migrate

import "sort"

// Delta is the classified difference between two canonical schema strings — the
// checked-in desired state and the schema derived from the current structs.
// Added and IndexChanged are additive and safe to emit verbatim into an
// EnsureSchema step. TypeChanged and Removed are destructive or ambiguous: the
// scaffolder only flags them as action-required comments, never as schema.
type Delta struct {
	Added        []string // full predicate declaration lines new since the last state
	IndexChanged []string // full predicate declaration lines whose index/directives changed
	TypeChanged  []string // "predicate: oldType → newType" notes; needs RetypePredicate
	Removed      []string // full predicate declaration lines absent from the current structs
}

// Additive returns the declaration lines safe to apply via EnsureSchema (the
// Added and IndexChanged buckets), sorted for a deterministic schema file.
func (d Delta) Additive() []string {
	out := make([]string, 0, len(d.Added)+len(d.IndexChanged))
	out = append(out, d.Added...)
	out = append(out, d.IndexChanged...)
	sort.Strings(out)
	return out
}

// HasAdditive reports whether the delta contains anything to emit as schema.
func (d Delta) HasAdditive() bool { return len(d.Added)+len(d.IndexChanged) > 0 }

// HasFlagged reports whether the delta contains a destructive/ambiguous change
// that the scaffolder will flag rather than emit.
func (d Delta) HasFlagged() bool { return len(d.TypeChanged)+len(d.Removed) > 0 }

// Empty reports whether the two schemas are identical (no drift).
func (d Delta) Empty() bool { return !d.HasAdditive() && !d.HasFlagged() }
