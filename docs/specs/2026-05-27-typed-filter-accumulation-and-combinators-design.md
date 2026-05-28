---
date: 2026-05-27
topic: typed-filter-accumulation-and-combinators
status: draft
---

# Filter Accumulation, `Or`, and `Where<Edge>By`

## Goal

Make the typed query builder compose filters correctly and let callers express
AND-of-OR and edge-target constraints without hand-written DQL:

```go
// AND-of-OR on the root entity
client.Resource.Query(ctx).
    ByArchiveStatus(filter.String{Value: "active"}).
    Or(
        func(r *ResourceQuery) { r.ByName(filter.String{Value: "vehicle"}) },
        func(r *ResourceQuery) { r.ByExtension(filter.String{Value: "glb"}) },
    ).
    Nodes()

// Typed edge-target filter — no DQL string at the call site
client.Resource.Query(ctx).
    WhereRevisionsBy(func(r *RevisionQuery) {
        r.ByExternalIdentifier(filter.String{Value: "X"})
    }).
    Nodes()
```

## Background: the accumulation bug

`typed.Query[T].Filter` delegated straight to dgman's `Query.Filter`, which is
**last-write-wins** (`q.filter = parseQueryWithParams(...)`). Each generated
`By<Field>` calls `Filter` with its own `filter.Builder` expression, so chaining
`ByID(...).ByName(...)` — or any `By<Field>` followed by a trailing `.Filter(...)`
— silently dropped every fragment but the last. The multi-field filter story was
therefore broken: only the final group survived.

## Design

### Filter fragments accumulate (AND)

`typed.Query[T]` now keeps a slice of `(expr, params)` fragments. `Filter`,
`WhereAnyOfText`, and `WhereAllOfText` append a fragment instead of overwriting.
The effective filter is every fragment joined with `AND`, each fragment's `$N`
placeholders shifted against the concatenated params slice (reusing
`shiftPlaceholders`, already present for the `WhereEdge` pre-pass). dgman's
`Filter` is still last-write-wins, so the full combined expression is re-pushed
on each call — `Raw()` continues to reflect the current filter.

```go
// Each call adds a fragment; the fragments AND together.
client.Resource.Query(ctx).
    ByName(filter.String{Value: "vehicle"}).   // (eq(resourceName, $1))
    ByExtension(filter.String{Value: "glb"}).  // AND (eq(extension, $2))
    Filter("ge(size, $1)", 1024).              // AND ge(size, $3)
    Nodes()
// effective @filter: (eq(resourceName, $1)) AND (eq(extension, $2)) AND ge(size, $3)
```

Before this change, dgman's last-write-wins `Filter` kept only the final
fragment — the first two groups were silently dropped.

### Capture primitive

Three small additions turn accumulation into composition:

- `NewDetachedQuery[T]()` — a connection-less `Query[T]` used only to accumulate
  a filter (no terminal path).
- `CombinedFilter() (string, []any)` — reads the AND-combined fragments back.
- `OrGroup(subs ...*Query[T])` — reads each sub's `CombinedFilter`, parenthesizes
  and OR-joins them into one group, which ANDs with the receiver's other filters.

Most callers reach this through the generated `Or` / `Where<Edge>By` (below) and
never touch the primitives. Used directly, they compose an AND-of-OR by hand:

```go
qc := typed.NewClient[Resource](conn)
qc.Query(ctx).
    Filter("eq(archiveStatus, $1)", "active").
    OrGroup(
        typed.NewDetachedQuery[Resource]().Filter("eq(extension, $1)", "glb"),
        typed.NewDetachedQuery[Resource]().Filter("eq(extension, $1)", "obj"),
    ).
    Nodes()
// effective @filter: eq(archiveStatus, $1) AND ((eq(extension, $2)) OR (eq(extension, $3)))
```

`CombinedFilter` reads a detached query's accumulation back without executing it:

```go
sub := typed.NewDetachedQuery[Resource]()
sub.Filter("eq(resourceName, $1)", "vehicle")
sub.Filter("ge(size, $1)", 1024)
expr, params := sub.CombinedFilter()
// expr   → "eq(resourceName, $1) AND ge(size, $2)"
// params → ["vehicle", 1024]
```

### Generated combinators

The wrapper template emits, per entity:

- `Or(builders ...func(*<E>Query))` — runs each builder against a detached
  `<E>Query`, then hands the subs to `typed.OrGroup`. Because each builder
  receives the full `<E>Query`, OR nests arbitrarily (a builder may itself call
  `Or`, or chain `By<Field>` which ANDs within the builder).
- `Where<Edge>By(build func(*<Target>Query))` — per edge, the type-safe form of
  `Where<Edge>`: runs `build` against a detached `<Target>Query`, captures its
  `CombinedFilter`, and feeds it to `WhereEdge`. No DQL string or `$N` at the
  call site.

`Or` and `Where<Edge>By` are the same capture primitive pointed at two sinks: a
parent OR group, and an edge constraint.

## Limitations

- `Or` and `Where<Edge>By` compose **scalar** filters. Nesting a `Where<Edge>`
  inside an `Or` builder or a `Where<Edge>By` closure is not supported — edge
  constraints resolve through the `@cascade` pre-pass, which has no OR form.
- Capture closures must use filter methods only (`By<Field>`, `Filter`, `Or`);
  a detached query has no connection, so terminals/ordering/pagination inside a
  closure are undefined.

## Naming

The edge variant is `Where<Edge>By`. For an edge whose accessor already ends in
"By" (e.g. `createdBy` → `CreatedBy`) this yields `WhereCreatedByBy`. The `By`
suffix is kept for consistency with `By<Field>`; `Where<Edge>Match` is the
alternative if the doubled suffix proves grating.
