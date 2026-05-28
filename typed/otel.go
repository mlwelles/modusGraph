/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package typed

import (
	"context"
	"reflect"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerName is the instrumentation scope for typed-layer DB spans. With no SDK
// installed in the host process the tracer is a no-op: span creation costs a
// few interface dispatches and one context node, and nothing is recorded or
// exported. The library never installs an SDK; that is the application's job.
const tracerName = "github.com/matthewmcneely/modusgraph/typed"

// entityName returns the unqualified Go type name of T (for example "Resource"),
// used as the db.collection attribute.
func entityName[T any]() string {
	return reflect.TypeFor[T]().Name()
}

// startDBSpan opens a span named "modusgraph.<op>" with Dgraph semantic
// attributes. Callers must defer endDBSpan(span, err).
func startDBSpan(ctx context.Context, op, collection string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "modusgraph."+op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "dgraph"),
			attribute.String("db.operation", op),
			attribute.String("db.collection", collection),
		),
	)
}

// endDBSpan records err (if any) on span and ends it. Safe to defer.
func endDBSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
