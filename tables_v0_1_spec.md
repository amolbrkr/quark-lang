# Quark Tables v0.1 Spec (Draft)

Status: Draft
Last Updated: 2026-04-02
Scope: Core table type, typed table literals, and in-memory vector-backed backend. This spec also defines a backend abstraction for future lazy and database-backed execution.

## 1. Purpose

Define a first-class `table` type that is:

1. Columnar and vector-backed for performance.
2. Strongly typed for compile-time guarantees.
3. Backend-agnostic at the language boundary so future `lazy_table` and `db_table` variants can share the same user-facing semantics.

## 2. Design Constraints

### 2.1 Product constraints

Tables must:

1. Feel native to Quark pipe-style workflows.
2. Provide stronger safety than ad hoc `dict` and `list` data wrangling.
3. Scale from local eager execution to remote/lazy backends without syntax churn.

### 2.2 Engineering constraints

1. Reuse existing vector runtime storage rather than adding a second column store.
2. Keep v0.1 parser and analyzer changes incremental.
3. Keep failure behavior explicit and fail-loud.

## 3. Goals and Non-Goals

### 3.1 Goals

1. Introduce `table` as a first-class type.
2. Support typed table construction with deterministic column order.
3. Enforce column shape and type constraints at compile time whenever possible.
4. Provide baseline table access operations (column, row, slice, mask).
5. Define backend abstraction points for future lazy/db execution.

### 3.2 Non-goals (v0.1)

1. No separate `query { ... }` syntax.
2. No optimizer-heavy query planning in v0.1.
3. No remote pushdown execution in v0.1.
4. No mutable row object semantics.
5. No table-valued generic type parameters.

## 4. Type Model

### 4.1 Table type form

`table` is a single first-class type at the source level.

Primary declaration form uses a struct annotation on the variable to supply row shape:

```quark
orders: Order = table {
    1, 'c1', 10.0, 2
    2, 'c2',  5.5, 4
}
```

Where `Order` is a struct type.

Semantics:

1. When a variable is annotated with a struct type `R` and the right-hand side is a `table { ... }` literal, the value is typed as `table` with attached row-shape metadata `R` in analysis/runtime metadata.
2. This keeps source syntax minimal while preserving compile-time guarantees.
3. Function signatures and variable annotations do not use `table R` syntax.

Example:

```quark
struct Order:
    id: int
    customer: str
    price: float
    qty: int

orders: Order = table {
    1, 'c1', 10.0, 2
    2, 'c2',  5.5, 4
}
```

### 4.2 Row type relationship

1. Every struct-typed table declaration has a row shape `R` (a struct).
2. Column names map 1:1 to row fields.
3. Column types map 1:1 to row field types.

### 4.3 Untyped or inferred tables

v0.1 supports inference when a struct annotation is absent, but with reduced guarantees.

Inference sources:

1. Typed column header declarations.
2. Homogeneous vector/list column sources.
3. Homogeneous row literals.

Recommendation for production code: prefer struct-typed table declarations (`x: R = table { ... }`) and plain `table` in function signatures.

## 5. Syntax

### 5.1 Struct-typed table literal (primary form)

```quark
orders: Order = table {
    1, 'c1', 10.0, 2
    2, 'c2',  5.5, 4
}
```

Rules:

1. Left-hand type annotation must resolve to a struct type.
2. No header row is required; field names and types come from the struct.
3. Each row arity must equal struct field count.
4. Positional mapping follows struct declaration order.
5. Row values must be type-compatible with struct fields.

### 5.2 Header-typed table literal (inference form)

Optional header form for inferred tables:

```quark
orders = table {
    id:int, customer:str, price:float, qty:int
    1, 'c1', 10.0, 2
    2, 'c2',  5.5, 4
}
```

### 5.3 Table literal from column sources

```quark
ids = vector [1, 2, 3]
customers = list ['c1', 'c2', 'c3']
prices = vector [10.0, 5.5, 8.0]
qtys = vector [2, 4, 1]

orders = table {
    id:int, customer:str, price:float, qty:int
    ids, customers, prices, qtys
}
```

Rules:

1. Source count must match declared column count.
2. All source columns must have identical length.
3. Each source must be list/vector compatible with declared column type.

### 5.4 Column names

v0.1 column names are identifiers. Quoted identifiers for spaces/punctuation are deferred unless lexer support is added.

### 5.5 Access and selection

```quark
orders.customer         // column access -> vector[str]
orders[0]              // row access -> Order
orders[0:10]           // row slice -> table
orders[orders.qty > 1] // mask filter -> table
```

### 5.6 Relation alias syntax (planned)

Quark reserves table aliasing in pipe workflows using `as`:

```quark
summary =
    orders as o
    | where(o.qty > 1)
    | mutate(total = o.price * o.qty)
```

Notes:

1. This is documented now for consistency with the language direction.
2. Full alias-aware pipeline semantics are specified in the pipe/column semantics spec.
3. Alias syntax may be implemented after core table literal/type support lands.

## 6. Grammar Additions (EBNF)

```ebnf
Type              ::= ...
                  |   TableType

TableType         ::= "table"

Expression        ::= ...
                  |   TableLiteral
                  |   TableIndexExpr

TableLiteral      ::= "table" "{" NEWLINE INDENT
                      TableBody
                      DEDENT "}"

TableBody         ::= HeaderThenRows
                  |   HeaderThenColSources
                  |   RowsOnly

HeaderThenRows    ::= TableHeader NEWLINE RowData { NEWLINE RowData }
HeaderThenColSources ::= TableHeader NEWLINE ColSourceLine
RowsOnly          ::= RowData { NEWLINE RowData }

TableHeader       ::= ColDecl { "," ColDecl }
ColDecl           ::= ID [ ":" Type ]

RowData           ::= Expression { "," Expression }
ColSourceLine     ::= Expression { "," Expression }

TableIndexExpr    ::= Expression "[" Expression "]"
                  |   Expression "[" Expression ":" Expression "]"
```

Parser note:

1. `table { ... }` is a dedicated literal form.
2. Body mode is context-sensitive:
    - When a struct-typed declaration context exists, `RowsOnly` is valid and mapped positionally to struct fields.
    - Otherwise a header-based form is required for reliable schema inference.

## 7. Compile-Time Guarantees

Guarantees depend on how much type information is available.

### 7.1 Struct-typed table (`x: R = table { ... }`) guarantees

1. `R` must resolve to a struct type.
2. Each row must have exactly one value per struct field.
3. Row position maps to struct declaration order.
4. Row value types must be compatible with struct field types.
5. Dot column access must refer to struct field names only.

Additional note:

1. These guarantees are attached to the resulting `table` value as row-shape metadata, not as a user-visible `table R` type annotation.

### 7.2 Header-typed inferred table guarantees

1. Column names and types are known from header.
2. Row/column-source values are checked against header types.
3. Dot column access is statically checked.

### 7.3 Untyped inferred table guarantees

1. Width consistency is enforced (all rows same arity).
2. Per-column type inference uses all rows/sources; mixed incompatible types are compile-time errors where analyzable.
3. Unknown-column diagnostics may degrade to runtime errors if schema is not statically known.

## 8. Runtime Semantics

### 8.1 Storage model (v0.1)

`table` runtime values are columnar and vector-backed:

1. Each column is a typed vector storage object.
2. Table stores column metadata (name, logical type, ordinal).
3. Table stores row count and column count.
4. No row-major storage exists.

### 8.2 Row materialization

`orders[i]` materializes a struct row value on demand from column vectors.

Properties:

1. Materialized row is a value, not a live mutable view into table storage.
2. Struct field types follow the table row type contract.

### 8.3 Null behavior

1. Table null behavior is inherited from underlying vector null masks.
2. Row materialization maps null-mask entries to `null` values at corresponding fields.

### 8.4 Mutability model

v0.1 table operations are value-oriented:

1. Filtering/slicing/projection produce new table values.
2. No direct mutable cell assignment syntax in v0.1.

## 9. Indexing and Access Semantics

### 9.1 Column access

`tbl.col`:

1. Returns the typed vector for `col`.
2. Unknown `col` is compile-time error for statically known tables, runtime fatal otherwise.

### 9.2 Row indexing

`tbl[i]`:

1. Returns row struct value.
2. Negative indexing support is allowed and should match vector/list conventions.
3. Out-of-bounds is a runtime fatal error.

### 9.3 Row slicing

`tbl[start:end]`:

1. Returns table with the same row-shape metadata.
2. Half-open interval `[start, end)`.
3. Bounds are clamped after negative index normalization.

### 9.4 Boolean mask filtering

`tbl[mask]`:

1. `mask` must be `vector[bool]`.
2. Mask length must equal table row count.
3. Null mask entries count as not selected.
4. Result preserves column order and row-shape metadata.

## 10. Backend Abstraction for Future Variants

### 10.1 Backend kinds

Planned backend kinds:

1. `mem` (v0.1): eager in-memory vector-backed table.
2. `lazy` (future): deferred execution over logical plans.
3. `db` (future): remote execution with pushdown.

### 10.2 Internal table contract

A table value should carry:

1. Row type id / schema metadata.
2. Backend kind.
3. Backend payload handle.
4. Optional logical plan handle (for lazy/db backends).

### 10.3 Capability model

Backends advertise supported operations (for example filter, join, group, summarize, sort). Analyzer/planner can reject unsupported pipelines early when backend is statically known.

### 10.4 Language-level compatibility rule

User-facing semantics for core operations must remain stable across backends. Differences should be performance/execution-site related, not syntax-related.

## 11. Diagnostics

Tables should reuse current diagnostic families:

1. Parse issues: `QK-PARSE-001`.
2. Type/semantic issues: `QK-CHECK-001`.
3. Runtime failures: `QK-RUNTIME-001`.

Recommended message shapes:

1. `table literal does not match struct Order fields`.
2. `column count mismatch: expected 4, got 3`.
3. `column 'price' expects float, got str`.
4. `table mask length mismatch: rows=100, mask=95`.
5. `row index 120 out of bounds for table with 100 rows`.

## 12. Interaction with Structs and Schemas

1. Structs define row shape.
2. Tables carry row collections for that shape.
3. Schemas (future spec) bind constraints over either row-level (`R`) or table-level (`table`) checks.

Example:

```quark
fn id_unique(t: table) result ->
    if unique_count(t.id) == len(t.id):
        ok null
    else:
        err 'id must be unique'
```

In practice, analyzer uses call-site row-shape metadata to statically validate `t.id` where known.

## 13. Compiler Work Plan (Implementation-Oriented)

1. Lexer: add `table` keyword token.
2. AST: add table literal node, table type node, table index/slice nodes.
3. Parser: parse header + body forms and distinguish row vs column-source mode.
4. Analyzer:
    - resolve row type binding from struct-typed declarations (`x: R = table { ... }`)
    - infer table schema when omitted
    - validate widths, types, and column names
    - propagate row-shape metadata through table operations
    - type-check table access/index/slice/mask
5. Runtime/codegen:
   - add `QTable` runtime representation referencing vector columns
   - emit literal construction and access code paths
   - ensure column vectors keep contiguous typed storage
6. Tests:
   - parser tests for literal forms
   - analyzer tests for type and shape errors
   - smoke tests for access/slice/filter behavior

## 14. Acceptance Criteria for v0.1

1. Typed and inferred table literals parse and type-check.
2. Table literals enforce width/type consistency.
3. Column-source construction enforces equal lengths and type compatibility.
4. Dot column access returns typed vectors.
5. Row index/slice/mask operations work with documented semantics.
6. Table values are vector-backed and column order is deterministic.
7. Existing list/vector behavior remains unchanged.
8. Struct-typed headerless table literals work as the primary declaration path.
9. Relation alias syntax is documented as planned and reserved for pipeline semantics.

## 15. Open Questions (Need Decision)

1. For untyped table inference, should int+float mixed columns promote to float, or fail unless explicit type is given?
2. Should `tbl[i]` out-of-bounds be fatal (current spec) or return `null` like list `.get()`?
3. Should quoted column names (for example `Customer Name`) be in v0.1, or deferred with identifier-only names?
4. Should `x: R = table { ... }` require `R` to be a struct only, or allow future schema-bound aliases directly?
5. For `fn f(t: table)`, how strict should analyzer be when row-shape metadata is unknown at call sites: fail compile or defer to runtime checks?
