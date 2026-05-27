/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package typed

import (
	"context"
	"encoding/json"
	"fmt"

	dg "github.com/dolan-in/dgman/v2"
	"github.com/matthewmcneely/modusgraph"
)

// MultiQuery batches N homogeneous-type Query[T] blocks into a single
// Dgraph multi-block request. All blocks return rows of the same T; the
// per-block result is keyed by the block name supplied at Add.
//
// Dgraph executes the blocks concurrently on the server side; the entire
// batch costs one gRPC round-trip.
type MultiQuery[T any] struct {
	conn   modusgraph.Client
	names  []string
	blocks map[string]*Query[T]
}

// NewMultiQuery constructs a MultiQuery bound to conn.
func NewMultiQuery[T any](conn modusgraph.Client) *MultiQuery[T] {
	return &MultiQuery[T]{
		conn:   conn,
		blocks: make(map[string]*Query[T]),
	}
}

// Add registers a named block. Names must be unique within one MultiQuery.
// Panics on duplicate name — the call site is a programming error, not a
// runtime condition.
func (mq *MultiQuery[T]) Add(name string, q *Query[T]) *MultiQuery[T] {
	if _, exists := mq.blocks[name]; exists {
		panic(fmt.Sprintf("multi_query: duplicate block name %q", name))
	}
	mq.names = append(mq.names, name)
	mq.blocks[name] = q
	return mq
}

// BlockNames returns the registered block names in insertion order.
func (mq *MultiQuery[T]) BlockNames() []string {
	out := make([]string, len(mq.names))
	copy(out, mq.names)
	return out
}

// Execute runs every registered block in a single Dgraph round-trip and
// returns the per-block results, keyed by the block name supplied at Add.
// A block that matched no rows appears as an empty (non-nil) slice in the
// result map; the key is always present.
//
// Execute rejects blocks that carry WhereEdge constraints — those require a
// runtime pre-pass that cannot be folded into the multi-block batch. Run such
// queries individually with Query.Nodes.
func (mq *MultiQuery[T]) Execute(ctx context.Context) (map[string][]T, error) {
	if len(mq.names) == 0 {
		return map[string][]T{}, nil
	}

	rawBlocks := make([]*dg.Query, 0, len(mq.names))
	for _, name := range mq.names {
		block := mq.blocks[name]
		if len(block.edges) != 0 {
			return nil, fmt.Errorf("multi_query: block %q carries WhereEdge constraints; MultiQuery cannot batch edge-filtered blocks", name)
		}
		// Name the underlying dgman query so blocks do not collide on the
		// default "data" name and so the response JSON keys are predictable.
		block.q.Name(name)
		rawBlocks = append(rawBlocks, block.q)
	}

	dql := dg.NewQueryBlock(rawBlocks...).String()
	raw, err := mq.conn.QueryRaw(ctx, dql, nil)
	if err != nil {
		return nil, fmt.Errorf("multi_query: dgraph: %w", err)
	}

	var perBlockRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw, &perBlockRaw); err != nil {
		return nil, fmt.Errorf("multi_query: decoding response: %w", err)
	}

	out := make(map[string][]T, len(mq.names))
	for _, name := range mq.names {
		body, ok := perBlockRaw[name]
		if !ok {
			out[name] = []T{}
			continue
		}
		var rows []T
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, fmt.Errorf("multi_query: decoding block %q: %w", name, err)
		}
		if rows == nil {
			rows = []T{}
		}
		out[name] = rows
	}
	return out, nil
}
