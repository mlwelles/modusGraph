/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package typed

import (
	"context"
	"fmt"

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

// Execute runs all blocks in a single Dgraph round-trip and returns the
// per-block results. The full implementation lands in the next commit on
// this branch.
func (mq *MultiQuery[T]) Execute(ctx context.Context) (map[string][]T, error) {
	if len(mq.names) == 0 {
		return map[string][]T{}, nil
	}
	return nil, fmt.Errorf("multi_query: Execute not yet implemented")
}
