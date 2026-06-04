# Changelog

## 2026-06-04 - OpenTelemetry instrumentation extracted

- refactor(typed)!: the typed client no longer hard-depends on OpenTelemetry. The
  inline `startDBSpan`/`endDBSpan` calls are replaced with a no-op-by-default `Tracer`
  seam (`typed.Span`, `typed.Tracer`, `typed.SetTracer`). The OpenTelemetry
  implementation moved to
  [`modusgraph-telemetry`](https://github.com/mlwelles/modusgraph-telemetry); install it
  with `typed.SetTracer(telemetry.New())`.

## 2026-06-04 - Migration engine extracted

- chore: the schema versioning and migration engine (`migrate`, `migrate/migratecli`)
  moved to a standalone project,
  [`modusgraph-migrate`](https://github.com/mlwelles/modusgraph-migrate).

## 2026-06-04 - Generator and wrapper-entity runtime extracted

- chore: the code generator (`cmd/modusgraph-gen`) and the wrapper-entity runtime
  (`typed/wrapper.go`) moved to a standalone project,
  [`modusgraph-gen`](https://github.com/mlwelles/modusgraph-gen). The `typed` package
  retains only the generic struct client and query builder.

## 2025-10-20 - Version 0.3.1

- chore: update to Dgraph v25.0.0 and dgo v250.0.0

## 2025-10-15 - Version 0.3.0

- feat: add new InsertRaw function
- chore: add throughput tests

## 2025-07-22 - Version 0.2.0

- feat: introduce new API that works with local mode and remote clusters
- chore: remove deprecated API

## 2025-05-21 - Version 0.1.0

Baseline for the changelog.

See git commit history for changes for this version and prior.
