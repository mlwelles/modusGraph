/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package typed_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	noop "go.opentelemetry.io/otel/trace/noop"

	"github.com/matthewmcneely/modusgraph/typed"
)

func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })
	return sr
}

func spanNames(sr *tracetest.SpanRecorder) []string {
	var names []string
	for _, s := range sr.Ended() {
		names = append(names, s.Name())
	}
	return names
}

func TestClient_CRUD_EmitsSpans(t *testing.T) {
	sr := recordSpans(t)
	ctx := context.Background()
	c := typed.NewClient[widget](newConn(t))

	w := &widget{Name: "sprocket", Qty: 1}
	if err := c.Add(ctx, w); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := c.Get(ctx, w.UID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	got := spanNames(sr)
	wantAdd, wantGet := false, false
	for _, n := range got {
		if n == "modusgraph.add" {
			wantAdd = true
		}
		if n == "modusgraph.get" {
			wantGet = true
		}
	}
	if !wantAdd || !wantGet {
		t.Fatalf("missing CRUD spans; got %v", got)
	}

	for _, s := range sr.Ended() {
		if s.Name() != "modusgraph.get" {
			continue
		}
		attrs := map[string]string{}
		for _, kv := range s.Attributes() {
			attrs[string(kv.Key)] = kv.Value.AsString()
		}
		if attrs["db.system"] != "dgraph" {
			t.Errorf("db.system = %q, want dgraph", attrs["db.system"])
		}
		if attrs["db.operation"] != "get" {
			t.Errorf("db.operation = %q, want get", attrs["db.operation"])
		}
		if attrs["db.collection"] != "widget" {
			t.Errorf("db.collection = %q, want widget", attrs["db.collection"])
		}
	}
}

func TestQuery_Terminals_EmitSpans(t *testing.T) {
	sr := recordSpans(t)
	ctx := context.Background()
	c := typed.NewClient[widget](newConn(t))
	if err := c.Add(ctx, &widget{Name: "a", Qty: 1}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := c.Query(ctx).Nodes(); err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if _, err := c.Query(ctx).First(); err != nil {
		t.Fatalf("First: %v", err)
	}

	var querySpans int
	for _, s := range sr.Ended() {
		if s.Name() == "modusgraph.query" {
			querySpans++
		}
	}
	if querySpans < 2 {
		t.Fatalf("want >=2 modusgraph.query spans, got %d (%v)", querySpans, spanNames(sr))
	}
}
