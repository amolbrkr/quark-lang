# Quark Tables v0.1 Spec (Draft)

Status: Draft, revision 2 (2026-11-13). The runtime engine and CSV reader of the revision 2 design are implemented; the compiler side is not. See "Revision 2" below, which takes precedence over the original draft where they differ.

Implementation state: only the runtime representation exists: `QTable` in `runtime/include/quark/types/table.hpp` and the `VAL_TABLE` value tag, which `fmt.table` can render. The lexer, parser, analyzer and codegen have no table support: `tbl` is not a keyword, a `tbl { ... }` literal is a parse error (`QK-PARSE-001`), and `tbl` is not accepted as a type annotation.
Last Updated: 2026-04-02
Scope: Core table type, typed table literals, and in-memory vector-backed backend. This spec also defines a backend abstraction for future lazy and database-backed execution.

## Revision 2 (2026-11-13): eager engine and query surface

This section records the decisions of 2026-11-13. It supersedes the rest of this draft where they conflict (relation aliases in §5.6, the backend abstraction in §10 for v0.1, and §15 Q1). The runtime engine and CSV reader are implemented; the compiler side (the `tbl` type, queries, `std/csv` bindings) is not, and will target the lowering/IR pipeline (`architecture.md` §12.8) rather than today's codegen.

### R2.1 Decisions

| # | Decision |
|---|---|
| D1 | **Eager, in-memory, columnar.** Every verb runs to completion and returns a new immutable table; there is no lazy plan in v0.1. Columns use the existing `QVector` buffers (Arrow layout: f64/i64 values, bit-packed bool, int32 offsets + UTF-8 for str, validity bitmaps), so `t.salary` is a `vec` without copying. Only the flat primitive layouts are used; no Arrow library. |
| D2 | **Typed by a struct.** `t: Employee = csv::read_file(path)`: the struct gives column names and types, and the compiler checks every column reference and expression against it. The schema flows through each verb, so the compiler knows every intermediate table's columns. |
| D3 | **Query syntax: one row name per pipe** (`t \| e -> verb(...) \| verb(...)`), re-bound at each verb to that verb's input row. `e` may only appear as `e.col` inside verb arguments. |
| D4 | **Verbs v0.1:** `filter`, `mutate` (add or replace columns), `sort`, `group_by`, `summarize`. No `reduce`: `summarize` without `group_by` aggregates the whole table into one row. `map` over vecs/tables (like `purrr::map`) comes later. |
| D5 | **`summarize` returns an ungrouped table**, so there is no `ungroup()`. `group_by` returns a grouped table that only `summarize` accepts (a compile-time error otherwise). |
| D6 | **Missing values follow `na.rm = TRUE`**: aggregates skip nulls (and NaN in float columns). `sum` of no values is 0; `mean`, `min`, `max` of no values are null; `count()` counts rows. `filter` drops rows whose predicate is null. Sort and group keys put nulls last (NaN after numbers, before nulls). All of this is documented behaviour, not configuration. |
| D7 | **Row indices are int32**: a table holds at most 2,147,483,647 rows (typical healthcare/pharma data is 100K-500K rows). |
| D8 | **CSV/TSV reader written in-house**, parsing straight into typed column buffers; no third-party CSV library. |
| D9 | **Group order is key order** (ascending, nulls last), as in dplyr, so `summarize` output is deterministic. |

### R2.2 Query surface (planned)

```quark
struct Employee:
    id: int
    name: str
    dept: str
    salary: float

staff: Employee = unwrap(csv::read_file('staff.csv'))

stats = staff | e ->
    filter(e.salary >= 100_000)
    | mutate(bonus = e.salary * 0.1)
    | group_by(e.dept)
    | summarize(avg = mean(e.salary), top = max(e.bonus), n = count())
    | sort(desc(e.avg))            // e is now the summarize row: {dept, avg, top, n}
```

- After `|`, an `ID ->` pair starts a query; the chain parses into one query node. Each step must be a verb: anything else, `e` used as a value (`f(e)`), an unknown column, a type mismatch, an aggregate outside `summarize`, or a non-table source is a compile-time error.
- `name = expr` inside `mutate` and `summarize` defines a column (Quark has no keyword arguments elsewhere).
- Verb expressions are arithmetic, comparisons, `and`/`or`/`!`, literals and outer scalar variables in v0.1. Codegen compiles each one into a typed loop over the column buffers (no expression interpreter, no temporary columns).
- Aggregates: `count()`, `count(e.col)` (non-missing values), `sum`, `mean`, `min`, `max`. `sum` of int is int (overflow is a runtime error), of bool counts true values, of float is float; `mean` is float (a bool mean is the share of true values); `min`/`max` keep the column's type and also work on str (byte order).
- `sort(e.a, desc(e.b))`: stable, several keys.
- `100_000` needs digit separators in the lexer (planned with the query work).

### R2.3 Runtime engine (implemented)

`runtime/include/quark/table/engine.hpp` (namespace `quark::tbl`), header-only and not part of `quark.hpp` (generated programs include it only when they use tables):

| Function | Does |
|---|---|
| `make_table(def, cols)` | Builds a table from equal-length columns |
| `take` / `take_table(t, idx, n)` | Gathers rows by index (filter, sort); taking every row in order returns the input |
| `sort_perm` / `sort_table(t, keys, n)` | Stable multi-key sort; a fast path for one dense numeric key |
| `group_rows(t, key_cols, n)` | Hash grouping (open addressing), groups renumbered in key order; `whole_table(n)` for summarize without `group_by` |
| `group_keys(t, g, col)` | The key column of the summarize output |
| `agg_count`, `agg_count_values`, `agg_sum`, `agg_mean`, `agg_min`, `agg_max` | Per-group aggregates (D6) |

Filter predicates and mutate expressions are not engine functions: codegen writes their loops and feeds the selected row indices to `take_table`.

Measured on 500K rows (`-O2`, one core, Linux x86_64, 2026-11-13): `group_by` + `mean` 23 ms, sort by one float key 97 ms, sort by two keys 271 ms.

### R2.4 CSV / TSV (implemented)

`runtime/include/quark/table/csv.hpp` (namespace `quark::csv`):

- `read_table(path, options, def, schema)` / `parse_table(text, ...)`: typed when a schema (column names and types) is given, otherwise every column's type is inferred from all of its values (int, else float, else bool, else str).
- `write_table(t, path, options)` / `format_table(t, sep)`.
- Accepted input: `,` or `\t` (from a `.tsv`/`.tab` path, or set explicitly); quoted fields with separators, line breaks and `""`; `\n`, `\r\n` and `\r` line endings; a UTF-8 BOM; blank lines (skipped); spaces around values and quotes (trimmed). `""` and `NA`, quoted or not, are missing in every column (as in readr); the list is an option. Header columns are matched to the schema by name, so a file may have more columns than the schema needs; without a header, by position.
- Errors name the file, line, column and text, and count the rest: `staff.csv:412: column 'salary': '12k' is not a float (and 3 more problems)`. A row with the wrong number of fields, an unterminated quote, text after a closing quote, a missing or duplicated column, an empty file and an int that overflows are all errors; nothing becomes null silently.
- Writing: a header line, missing values as empty fields, strings quoted only when needed, floats in shortest round-trip form.
- Measured: a 27.6 MB file of 500K rows reads in 219 ms typed and 480 ms inferred.

### R2.5 Not yet

- Compiler: `tbl` type and struct-typed tables, the query node, digit separators, `std/csv` bindings returning `result`, printing.
- Verbs after v0.1: joins, `select`/`rename`, `distinct`, `n_distinct`, `slice`/`head`.
- Column types: `date` (int32 days, Arrow `date32`), then timestamps and categoricals.
- Header names that aren't identifiers (`Employee ID`) need a renaming option on read.

## 1. Purpose

Define a first-class `tbl` type that is:

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

1. Introduce `tbl` as a first-class type.
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

`tbl` is a single first-class type at the source level.

Primary declaration form uses a struct annotation on the variable to supply row shape:

```quark
orders: Order = tbl {
    1, 'c1', 10.0, 2
    2, 'c2',  5.5, 4
}
```

Where `Order` is a struct type.

Semantics:

1. When a variable is annotated with a struct type `R` and the right-hand side is a `tbl { ... }` literal, the value is typed as `tbl` with attached row-shape metadata `R` in analysis/runtime metadata.
2. This keeps source syntax minimal while preserving compile-time guarantees.
3. Function signatures and variable annotations do not use `tbl R` syntax.

Example:

```quark
struct Order:
    id: int
    customer: str
    price: float
    qty: int

orders: Order = tbl {
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

Recommendation for production code: prefer struct-typed table declarations (`x: R = tbl { ... }`) and plain `tbl` in function signatures.

## 5. Syntax

### 5.1 Struct-typed table literal (primary form)

```quark
orders: Order = tbl {
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
orders = tbl {
    id:int, customer:str, price:float, qty:int
    1, 'c1', 10.0, 2
    2, 'c2',  5.5, 4
}
```

### 5.3 Table literal from column sources

```quark
ids = vec [1, 2, 3]
customers = list ['c1', 'c2', 'c3']
prices = vec [10.0, 5.5, 8.0]
qtys = vec [2, 4, 1]

orders = tbl {
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
orders.customer         // column access -> vec[str]
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

TableType         ::= "tbl"

Expression        ::= ...
                  |   TableLiteral
                  |   TableIndexExpr

TableLiteral      ::= "tbl" "{" NEWLINE INDENT
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

1. `tbl { ... }` is a dedicated literal form.
2. Body mode is context-sensitive:
    - When a struct-typed declaration context exists, `RowsOnly` is valid and mapped positionally to struct fields.
    - Otherwise a header-based form is required for reliable schema inference.

## 7. Compile-Time Guarantees

Guarantees depend on how much type information is available.

### 7.1 Struct-typed table (`x: R = tbl { ... }`) guarantees

1. `R` must resolve to a struct type.
2. Each row must have exactly one value per struct field.
3. Row position maps to struct declaration order.
4. Row value types must be compatible with struct field types.
5. Dot column access must refer to struct field names only.

Additional note:

1. These guarantees are attached to the resulting `tbl` value as row-shape metadata, not as a user-visible `tbl R` type annotation.

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

`tbl` runtime values are columnar and vector-backed:

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

1. `mask` must be `vec[bool]`.
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
3. Schemas (future spec) bind constraints over either row-level (`R`) or table-level (`tbl`) checks.

Example:

```quark
fn id_unique(t: tbl) result ->
    if unique_count(t.id) == len(t.id):
        ok null
    else:
        err 'id must be unique'
```

In practice, analyzer uses call-site row-shape metadata to statically validate `t.id` where known.

## 13. Compiler Work Plan (Implementation-Oriented)

1. Lexer: add `tbl` keyword token.
2. AST: add table literal node, table type node, table index/slice nodes.
3. Parser: parse header + body forms and distinguish row vs column-source mode.
4. Analyzer:
    - resolve row type binding from struct-typed declarations (`x: R = tbl { ... }`)
    - infer table schema when omitted
    - validate widths, types, and column names
    - propagate row-shape metadata through table operations
    - type-check table access/index/slice/mask
5. Runtime/codegen:
   - add `QTable` runtime representation referencing vector columns (done: `types/table.hpp`)
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
4. Should `x: R = tbl { ... }` require `R` to be a struct only, or allow future schema-bound aliases directly?
5. For `fn f(t: tbl)`, how strict should analyzer be when row-shape metadata is unknown at call sites: fail compile or defer to runtime checks?
