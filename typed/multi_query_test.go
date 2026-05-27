/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package typed_test

import (
	"context"
	"testing"

	"github.com/matthewmcneely/modusgraph/typed"
)

func TestMultiQueryAddAccumulatesBlocks(t *testing.T) {
	conn := newConn(t)
	mq := typed.NewMultiQuery[widget](conn)
	q1 := typed.NewClient[widget](conn).Query(context.Background())
	q2 := typed.NewClient[widget](conn).Query(context.Background())
	mq.Add("byName", q1)
	mq.Add("byQty", q2)
	got := mq.BlockNames()
	if len(got) != 2 || got[0] != "byName" || got[1] != "byQty" {
		t.Fatalf("BlockNames = %v, want [byName, byQty]", got)
	}
}

func TestMultiQueryAddRejectsDuplicateName(t *testing.T) {
	conn := newConn(t)
	mq := typed.NewMultiQuery[widget](conn)
	q := typed.NewClient[widget](conn).Query(context.Background())
	mq.Add("byName", q)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate block name")
		}
	}()
	mq.Add("byName", q)
}
