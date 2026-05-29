package migrate_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	mg "github.com/matthewmcneely/modusgraph"
	"github.com/matthewmcneely/modusgraph/migrate"
)

type Tree struct {
	UID    string   `json:"uid,omitempty"`
	DType  []string `json:"dgraph.type,omitempty" dgraph:"Tree"`
	Height int      `json:"height,omitempty" dgraph:"predicate=height index=int"`
}

// Example_additive shows the simplest migration: one step that ensures a type's
// schema exists. Ensure is additive and idempotent.
func Example_additive() {
	m := migrate.Migration{
		ID:   20260101000000,
		Name: "create_tree",
		Steps: []migrate.Step{
			{Name: "ensure_tree", Schema: migrate.SchemaChange{Ensure: []any{&Tree{}}}},
		},
	}
	fmt.Println(m.Name, len(m.Steps))
	// Output: create_tree 1
}

// Example_frozenSchema shows the immutable counterpart to Ensure. MarshalSchema
// renders the live structs to a Dgraph schema string once; storing that string
// in EnsureSchema freezes it, so the migration stays reproducible even after the
// Tree struct evolves. Capture the string at authoring time (e.g. paste the
// MarshalSchema output into a const or an embedded file) rather than calling
// MarshalSchema from the migration, which would re-derive it from live structs.
func Example_frozenSchema() {
	frozen := migrate.MarshalSchema(&Tree{})
	m := migrate.Migration{
		ID:   20260104000000,
		Name: "baseline",
		Steps: []migrate.Step{
			{Name: "baseline_schema", Schema: migrate.SchemaChange{EnsureSchema: frozen}},
		},
	}
	fmt.Println(strings.Contains(frozen, "height:"), len(m.Steps))
	// Output: true 1
}

// Example_backfill shows a hybrid migration: a schema step followed by a data
// step. The data step is keyed by UID so re-running it converges.
func Example_backfill() {
	m := migrate.Migration{
		ID:   20260102000000,
		Name: "seed_tree",
		Steps: []migrate.Step{
			{Name: "ensure_tree", Schema: migrate.SchemaChange{Ensure: []any{&Tree{}}}},
			{Name: "seed_one", Up: func(ctx context.Context, c mg.Client) error {
				return c.Insert(ctx, &Tree{DType: []string{"Tree"}, Height: 5730})
			}},
		},
	}
	fmt.Println(len(m.Steps))
	// Output: 2
}

// Example_retypePredicate shows how RetypePredicate expands into five staged,
// checkpointed, idempotent steps. Provide a Convert func that maps each
// existing string-rendered value to its new typed value. The op is irreversible:
// every step's Down is nil.
func Example_retypePredicate() {
	spec := migrate.RetypeSpec{
		Predicate: "height",
		To:        migrate.Int,
		Index:     "int",
		// metersToMM converts a decimal-meters string to integer millimeters.
		Convert: func(old string) (any, error) {
			v, err := strconv.ParseFloat(old, 64)
			if err != nil {
				return nil, err
			}
			return int64(v * 1000), nil
		},
	}
	steps := migrate.RetypePredicate(spec)
	fmt.Println(len(steps))
	// Output: 5
}

// Example_irreversibleDrop shows a destructive step. It omits Down, so any down
// range that includes it is refused with ErrIrreversible.
func Example_irreversibleDrop() {
	m := migrate.Migration{
		ID:   20260103000000,
		Name: "drop_legacy",
		Steps: []migrate.Step{
			{Name: "drop_old", Schema: migrate.SchemaChange{Alter: "drop_predicate_via_alter_here ."}},
		},
	}
	fmt.Println(m.Steps[0].Down == nil)
	// Output: true
}
