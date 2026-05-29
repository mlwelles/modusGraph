package migrate_test

import (
	"context"
	"fmt"

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
