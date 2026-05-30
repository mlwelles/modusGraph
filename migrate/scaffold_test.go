package migrate

import (
	"strings"
	"testing"
)

// The two fixture struct sets differ by exactly one change per classification:
//   diff_mime   — present only in current        → Added
//   diff_title  — index term → exact             → IndexChanged
//   diff_size   — type int → string              → TypeChanged
//   diff_legacy — present only in prev           → Removed
type diffPrev struct {
	UID    string   `json:"uid,omitempty"`
	DType  []string `json:"dgraph.type,omitempty" dgraph:"DiffDoc"`
	Title  string   `json:"title,omitempty" dgraph:"predicate=diff_title index=term"`
	Size   int      `json:"size,omitempty" dgraph:"predicate=diff_size"`
	Legacy string   `json:"legacy,omitempty" dgraph:"predicate=diff_legacy"`
}

type diffCurrent struct {
	UID   string   `json:"uid,omitempty"`
	DType []string `json:"dgraph.type,omitempty" dgraph:"DiffDoc"`
	Title string   `json:"title,omitempty" dgraph:"predicate=diff_title index=exact"`
	Size  string   `json:"size,omitempty" dgraph:"predicate=diff_size"`
	Mime  string   `json:"mime,omitempty" dgraph:"predicate=diff_mime index=exact"`
}

func bucketHas(bucket []string, substr string) bool {
	for _, s := range bucket {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func TestDiffSchema_ClassifiesEachChange(t *testing.T) {
	prev := MarshalSchema(&diffPrev{})
	current := MarshalSchema(&diffCurrent{})

	d := diffSchema(prev, current)

	if !bucketHas(d.Added, "diff_mime") {
		t.Errorf("diff_mime should be Added; got %v", d.Added)
	}
	if !bucketHas(d.IndexChanged, "diff_title") {
		t.Errorf("diff_title should be IndexChanged; got %v", d.IndexChanged)
	}
	if !bucketHas(d.TypeChanged, "diff_size") {
		t.Errorf("diff_size should be TypeChanged; got %v", d.TypeChanged)
	}
	if !bucketHas(d.Removed, "diff_legacy") {
		t.Errorf("diff_legacy should be Removed; got %v", d.Removed)
	}

	// Each change lands in exactly one bucket.
	if bucketHas(d.Added, "diff_size") || bucketHas(d.IndexChanged, "diff_size") {
		t.Errorf("type change leaked into an additive bucket: Added=%v Index=%v", d.Added, d.IndexChanged)
	}
	if bucketHas(d.Added, "diff_title") {
		t.Errorf("index change misclassified as Added: %v", d.Added)
	}

	// The type change is destructive and must not be emittable schema.
	if d.HasAdditive() && bucketHas(d.Additive(), "diff_size") {
		t.Errorf("retype must never be emitted as additive schema")
	}
	if !d.HasFlagged() {
		t.Errorf("expected flagged changes (type change + removal)")
	}
}

func TestDiffSchema_IdenticalIsEmpty(t *testing.T) {
	s := MarshalSchema(&diffCurrent{})
	if d := diffSchema(s, s); !d.Empty() {
		t.Errorf("identical schemas should produce an empty delta; got %+v", d)
	}
}

func TestDiffSchema_EmptyPrevIsAllAdded(t *testing.T) {
	current := MarshalSchema(&diffCurrent{})
	d := diffSchema("", current)
	if len(d.Added) == 0 || d.HasFlagged() {
		t.Errorf("empty prior state should make every predicate Added with no flags; got %+v", d)
	}
	if !bucketHas(d.Added, "diff_title") || !bucketHas(d.Added, "diff_mime") {
		t.Errorf("expected all current predicates in Added; got %v", d.Added)
	}
}
