# Quark Dates v0.1 Spec

Status: Implemented (2026-11-15). Open questions in §14 are decided as written there. `time::time(...)` is `time::timestamp(...)`: a member cannot share its module's name. Vec literals of dates fail at compile time (`vec literals can't hold date yet`). Decisions below supersede the float-as-date approach in `specs/stdlib_expansion_v0_1_spec.md` §5.
Last Updated: 2026-11-15
Related: `stdlib.md` (`std/time` today), `semantics.md` §9.5 (tables), `specs/tables_v0_1_spec.md`, `architecture.md` §12 (soundness), Arrow `date32`.

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [The `date` type](#3-the-date-type)
4. [The `period` type](#4-the-period-type)
5. [Arithmetic](#5-arithmetic)
6. [`std/time` (rewritten)](#6-stdtime-rewritten)
7. [Tables and vectors](#7-tables-and-vectors)
8. [CSV](#8-csv)
9. [Printing, conversion, equality](#9-printing-conversion-equality)
10. [What stays float](#10-what-stays-float)
11. [Implementation sketch](#11-implementation-sketch)
12. [Tests](#12-tests)
13. [Out of scope](#13-out-of-scope)
14. [Open questions](#14-open-questions)

## 1. Purpose

Calendar dates show up in almost every data script: filter rows after a cutoff, group by month, parse a CSV column of `YYYY-MM-DD`. Quark needs:

- a **first-class `date` type** that is cheap to store, compare and sort (Arrow `date32`: int32 days since 1970-01-01);
- **R / lubridate-shaped arithmetic** so `cutoff + time::months(3)` is calendar math, not "add N×86400 seconds";
- the same type in **scripts and table columns**, so `filter(e.created_at > cutoff)` is one language, not two;
- a **rewritten `std/time`** that builds and parses dates (and still handles clocks / float timestamps).

Instants (timestamps) stay **float seconds since the Unix epoch** in v0.1. A dedicated `timestamp` column type can come later.

## 2. Decisions

| # | Decision |
|---|---|
| D1 | **`date` is a language type**, like `int` and `str`: annotations, struct fields, parameters, returns, and table columns. It is not a heap object and not a `std/time` struct. |
| D2 | **Storage is Arrow `date32`**: a signed 32-bit count of days since 1970-01-01 (Unix epoch), UTC calendar, no time of day and no time zone. Same as R's `Date` and Arrow `date32`. Range is the int32 range of day counts (roughly years −5.8M .. +5.8M). |
| D3 | **`period` is a language type** for calendar offsets. It is *not* a column type. `time::days`, `weeks`, `months`, `quarters` and `years` return `period`. Without `period`, `d + time::months(3)` cannot type-check: Quark has no user-defined operators. |
| D4 | **`date + period` uses calendar arithmetic with rollback** (lubridate's `%m+%` / `add_with_rollback`, not plain `+ months` which yields `NA` on impossible days). January 31 + 1 month → February 28 (or 29 in a leap year), never March 3 and never a missing value. Day and week components are exact day counts after the month/year step. |
| D5 | **`date + int` and `date - int` add or subtract whole days** (as in R). `time::days(n)` is the period form of the same idea; `d + 3` and `d + time::days(3)` are equal. |
| D6 | **`date - date` is an `int`**: the number of days from the right operand to the left (`d2 - d1`). Not a float and not a period. |
| D7 | **Break and rewrite `std/time`.** Today's helpers that treat "dates" as float midnights, and `days` / `weeks` that return float seconds, go away. Clocks (`now`, `clock`, `sleep`, `since`) and float-second duration helpers (`seconds`, `minutes`, `hours`, `duration`) stay; parsing / formatting / construction are redone around `date` (and float timestamps where they still apply). |
| D8 | **Constructors and parsers live in `std/time`**, not the prelude: `time::date`, `time::ymd`, `time::today`, period helpers, and parse/format. The type name `date` is in the language; building one requires the module (or a conversion). |
| D9 | **A `date` column is a `QVector` of `date32`** (new vector dtype). `t.col` on a date field is `vec[date]`. Compare, sort, `min` / `max`, and group keys use raw int32 order. `sum` / `mean` of a date column are compile-time errors. |
| D10 | **CSV**: a typed `date` field accepts ISO calendar dates (see §8). Untyped inference may promote a column to `date` when every non-missing cell is such a date (after the int/float/bool rules fail). Writing a date cell is `YYYY-MM-DD`. |
| D11 | **Invalid calendar days are errors at construction and parse** (`time::date(2025, 2, 29)`, a bad parse string). Arithmetic never produces an invalid date: it rolls back (D4). |
| D12 | **No date literal syntax in v0.1** (`d'2022-01-30'` or similar). Use `time::date`, `time::ymd`, or `time::parse`. |
| D13 | **`null` is not a `date` value** (same rule as `int`). Missing entries exist only inside columns / vecs; taking one out is a runtime error (tables D12). |

## 3. The `date` type

### 3.1 Meaning

A `date` is a civil calendar day: year, month, day-of-month in the proleptic Gregorian calendar, with no time and no zone. Internally it is one `int32` day index; the civil fields are computed when needed (print, parse, month arithmetic).

### 3.2 Where it appears

```quark
incl 'std/time' as time

struct Event:
    id: int
    created_at: date

cutoff: date = time::ymd('2022-01-30')
fn after(d: date, cut: date) bool -> d > cut
```

- Annotation keyword: `date` (lexer / type grammar, next to `int`, `str`, …).
- Allowed as a struct field type that makes a table column (`IsColumnType`).
- Allowed as a list element type once settled (`list` of dates); vec form is `vec[date]` from a column or a future literal.

### 3.3 Epoch and range

| Day index | Calendar |
|---|---|
| `0` | 1970-01-01 |
| `1` | 1970-01-02 |
| `-1` | 1969-12-31 |

Out-of-range day indexes that cannot be represented as `int32` are a runtime error at construction. Civil constructors reject impossible Y-M-D (D11) before indexing.

## 4. The `period` type

A `period` is a calendar offset with three integer components: **years**, **months**, and **days**. Weeks and quarters are sugar that fill those components.

| Helper | Resulting period |
|---|---|
| `time::days(n)` | `{years: 0, months: 0, days: n}` |
| `time::weeks(n)` | `{years: 0, months: 0, days: 7·n}` |
| `time::months(n)` | `{years: 0, months: n, days: 0}` |
| `time::quarters(n)` | `{years: 0, months: 3·n, days: 0}` |
| `time::years(n)` | `{years: n, months: 0, days: 0}` |

`n` is an `int` (may be negative or zero). A non-int is a compile-time error when the type is known.

Periods combine with `+` / `-` (component-wise) and with `*` by an int:

```quark
time::months(1) + time::days(2)    // 1 month and 2 days
3 * time::months(1)                // same as time::months(3)
time::years(1) - time::months(1)   // 1 year minus 1 month
```

A `period` is **not** a column type, not accepted by `tbl` row fields, and not inferred from CSV. It may be stored in variables, fields of ordinary structs, list elements, and parameters.

Printing: a short form such as `3 months`, `1 year 2 days`, `0 days` (exact format fixed at implementation; stable enough for tests).

## 5. Arithmetic

### 5.1 Allowed operations

| Expression | Result | Rule |
|---|---|---|
| `d + p`, `p + d` | `date` | Apply period to date (§5.2) |
| `d - p` | `date` | Apply negated period |
| `d + n`, `n + d` | `date` | Add `n` days (`n: int`) |
| `d - n` | `date` | Subtract `n` days |
| `d2 - d1` | `int` | Day difference |
| `p1 + p2`, `p1 - p2` | `period` | Component-wise |
| `n * p`, `p * n` | `period` | Scale components by `n: int` |
| `d1 == d2`, `!=`, `<`, `<=`, `>`, `>=` | `bool` | int32 order |
| `p1 == p2`, `!=` | `bool` | Component equality |

Everything else involving `date` or `period` and `+` / `-` / `*` is a **compile-time error** when both types are known (for example `d + 1.5`, `d * 2`, `d + d`, `p + 1`, `d / p`). An `any` operand is checked at runtime.

`date` and `period` are never truthy operands of `and` / `or` / `!` / `if` conditions outside queries (same as other non-bool types). In a query, comparisons already produce bools.

### 5.2 Applying a period to a date (rollback)

Order of application (fixed, documented, tested):

1. Add `years` to the year.
2. Add `months` to the month; normalize year/month (month 13 → next year month 1, and so on for any overflow).
3. If the original day-of-month does not exist in the new month, **clamp to the last day of that month** (rollback).
4. Add `days` as a whole-day offset on the day index (can move into another month; that step is exact, not rolled).

Examples:

```quark
time::ymd('2022-01-30') + time::months(3)   // 2022-04-30
time::ymd('2022-01-31') + time::months(1)   // 2022-02-28
time::ymd('2024-01-31') + time::months(1)   // 2024-02-29 (leap)
time::ymd('2022-01-31') + time::months(1) + time::days(1)  // 2022-03-01
time::ymd('2022-03-31') - time::months(1)   // 2022-02-28
```

This matches lubridate `%m+%`, not lubridate's plain `+ months(...)` (which returns `NA` for impossible days). Quark does not use missing values as arithmetic results for scalars; rollback keeps a real `date` and stays reversible enough for data work (`d + months(1) - months(1)` can differ when rollback happened — same caveat as R's `%m+%`; document it).

### 5.3 Why not float-day arithmetic for months?

`date` is a day index, so `+ days` is one integer add. Months and years are civil; doing them as `n * 30` would be silently wrong (Robustness Bar: no silent wrong results). Periods exist so the operator stays readable *and* the meaning stays calendar-correct.

## 6. `std/time` (rewritten)

Import: `incl 'std/time' as time`.

### 6.1 Dates

| Function | Signature | Description |
|---|---|---|
| `date` | `year: int, month: int, day: int -> date` | Civil constructor. Impossible Y-M-D is a runtime error. |
| `ymd` | `text: str -> date` | Parse a year-month-day string (lubridate-shaped). Accepts `-`, `/`, or `.` as separators, and 1- or 2-digit month/day (`2022-1-30`, `2022/01/30`). Surrounding whitespace allowed; other trailing text is not. On failure: runtime error (or `result` — see §14 Q1). |
| `today` | `local: bool = false -> date` | Current calendar day. UTC unless `local` is true (machine local zone only, as today). |
| `parts` | `d: date -> time::DateParts` | `year`, `month`, `day`, `weekday` (0 = Monday), `yearday` (1 = 1 January). |
| `year` / `month` / `day` / `weekday` | `d: date -> int` | Field accessors (optional sugar over `parts`; include at least these four). |
| `from_days` | `n: int -> date` | Day index → `date` (Arrow / R interop). Out of int32 range: runtime error. |
| `to_days` | `d: date -> int` | Inverse of `from_days`. |
| `format` | `d: date, pattern: str = '%Y-%m-%d' -> str` | `strftime`-style on the civil day (time fields behave as midnight UTC). Empty pattern: runtime error. |
| `parse` | `text: str, pattern: str = '%Y-%m-%d' -> result` | `ok(date)` or `err(str)`. Whole string must match; bad calendar day → `err`. |

### 6.2 Periods

| Function | Signature | Description |
|---|---|---|
| `days` | `n: int -> period` | |
| `weeks` | `n: int -> period` | |
| `months` | `n: int -> period` | |
| `quarters` | `n: int -> period` | |
| `years` | `n: int -> period` | |

Singular names (`day`, `month`, …) are **not** provided in v0.1 (avoids clashing with field accessors like `time::month(d)` if we add those as functions on dates). Period helpers stay plural.

### 6.3 Timestamps (float seconds) and clocks

Kept, still float-based:

| Function | Signature | Notes |
|---|---|---|
| `now` | `-> float` | Epoch seconds |
| `clock` | `-> float` | Monotonic |
| `sleep` | `seconds: float` | |
| `since` | `start: float -> float` | |
| `seconds` / `minutes` / `hours` | `n: float -> float` | Float second lengths only |
| `duration` | `seconds: float -> str` | Human duration string |
| `timestamp` | `year, month, day, hour = 0, minute = 0, second = 0.0, local = false -> float` | Replaces `make`: a timestamp from civil fields. Not named `time`, because that clashes with the module |
| `iso` | `ts: float -> str` | ISO 8601 UTC instant |
| `format_time` | `ts: float, pattern: str = '%Y-%m-%d %H:%M:%S', local: bool = false -> str` | Instant formatting (name distinguishes it from `format` on `date`) |
| `parse_time` | `text: str, pattern: str = '%Y-%m-%d %H:%M:%S', local: bool = false -> result` | `ok(float)` |
| `parse_iso` | `text: str -> result` | Instant ISO, as today |
| `parts_time` | `ts: float, local: bool = false -> time::Parts` | Full civil+time parts (today's `Parts`) |
| `date_from_ts` | `ts: float, local: bool = false -> date` | Calendar day of an instant |
| `ts_from_date` | `d: date, local: bool = false -> float` | Midnight of that day as a timestamp |

`time::Parts` remains the timestamp parts struct. `time::DateParts` is the date-only struct (no hour/minute/second), unless `parts(d)` reuses `Parts` with time fields zero — prefer a small `DateParts` so a date never pretends to have a time of day.

### 6.4 Breaking changes vs current `std/time`

| Today | After |
|---|---|
| `today()` → `float` midnight | `today()` → `date` |
| `make(...)` → `float` | `timestamp(...)` → `float`; `date(y,m,d)` → `date` |
| `parse` / `parse_iso` → `ok(float)` | `parse` → `ok(date)`; instant parsing is `parse_time` / `parse_iso` |
| `format(ts, ...)` on floats | `format(d, ...)` on dates; `format_time` for floats |
| `days` / `weeks` → float seconds | → `period` |
| `parts(ts)` | `parts(d)` on dates; `parts_time(ts)` on floats |

Document these under Breaking in `changelog.md` when implementing.

### 6.5 Example

```quark
incl 'std/time' as time

cutoff: date = time::ymd('2022-01-30')
print(cutoff + time::months(3))    // 2022-04-30
print(cutoff + time::days(7))      // 2022-02-06
print(time::date(2026, 1, 1) - cutoff)

events: tbl[Event] = unwrap(csv::read_file('events.csv'))
recent = events | e -> filter(e.created_at > cutoff)
by_month = events | e ->
    mutate(month_start = e.created_at - time::days(time::day(e.created_at) - 1))
    | group_by(e.month_start)
    | summarize(n = count())
```

## 7. Tables and vectors

### 7.1 Column type

`IsColumnType` gains `date`. A row struct may declare `created_at: date`. Physical column: `QVector::Type::DATE` (int32 values, Arrow `date32` / `tdD`), with the usual validity bitmap.

### 7.2 Column access

`t.created_at` is `vec[date]`. Element reads follow tables D12 (missing → runtime error; `get` → `null` / `any`).

### 7.3 Queries

Verb expressions already allow calls and arithmetic (tables D21). After this work:

```quark
t | e -> filter(e.created_at > cutoff)
t | e -> filter(e.created_at >= time::ymd('2024-01-01'))
t | e -> mutate(later = e.created_at + time::months(1))
t | e -> sort(e.created_at)
t | e -> group_by(e.created_at) | summarize(n = count())
t | e -> summarize(lo = min(e.created_at), hi = max(e.created_at))
```

- Comparisons and `+` / `-` with periods and ints lower to raw int32 (and a small civil helper only for month/year periods).
- `min` / `max` keep type `date`.
- `sum` / `mean` / `n_distinct` on dates: `n_distinct` is fine; `sum` / `mean` are compile-time errors (same spirit as for `str`).

### 7.4 Performance

- Sort, filter compare, group key, `min` / `max`: **raw int32**, no string conversion.
- `date + days` / `date + int`: one integer add (overflow → runtime error if outside int32).
- `date + months/years/quarters`: civil decode → add → rollback → encode; acceptable per row; still far cheaper than string dates.

## 8. CSV

### 8.1 Typed read (`tbl[R]` with a `date` field)

Accepted cell text (after NA / empty → missing):

- Strict ISO form preferred: `YYYY-MM-DD` (zero-padded month and day).
- Also accept unpadded `YYYY-M-D` with `-` separators when reading (writer always pads).

Anything else for that cell is a parse error naming line and column (`events.csv:12: column 'created_at' is not a date (got '01/30/2022')`). Do **not** silently invent a format from locale.

### 8.2 Untyped inference

Current order: int → float → bool → str. Extend to:

**int → float → bool → date → str**

A column becomes `date` only when every non-missing value matches the typed date patterns above. Values like `20240101` stay **int** (they already matched int). Values like `2024-01-01` are not ints or floats, so they can become `date` instead of `str`.

### 8.3 Write

Date cells write as `YYYY-MM-DD`. Missing stays empty / `NA` per existing options.

### 8.4 `std/csv` surface

No new Quark functions. The reader/writer and `field_dtypes` gain a date dtype code next to int/float/bool/str. Docs in `stdlib.md` mention date columns with a short example.

## 9. Printing, conversion, equality

| Operation | Behaviour |
|---|---|
| `print(d)` / `to_str(d)` | `YYYY-MM-DD` |
| `type(d)` | `'date'` |
| `==` / `!=` / ordering | Day-index order |
| `to_int(d)` | Same as `time::to_days(d)` (days since epoch), or reject and require `to_days` — prefer **explicit `to_days`** so `to_int` on a date is a compile-time error and nobody confuses dates with ordinary ints (Q2). |
| Widening | None. `date` does not widen to `int` or `float`. Passing a date where an int is expected is an error. |

## 10. What stays float

- `now()`, measurements, `sleep`, duration formatting.
- Instant parse/format (`parse_iso`, `format_time`, `time(...)`).
- Float `+` / `-` of seconds. There is **no** `float + period` in v0.1 (compile-time error). To shift an instant by calendar months, convert with `date_from_ts`, add the period, then `ts_from_date` (loses time-of-day) or wait for a later `timestamp` type.

This keeps the first cut small and matches "keep instants as floats if it makes things easier."

## 11. Implementation sketch

Rough order (each step shippable with tests):

1. **Runtime + type system**: `QValue::VAL_DATE`, `VAL_PERIOD`; `TypeDate` / `TypePeriod`; lexer type keyword `date` / `period`; `qv_date` / `q_as_date` / `q_param_date`; period constructors in the runtime.
2. **Operators**: analyzer + lowering for §5.1; civil add with rollback in one runtime helper (`q_date_add_period`).
3. **`std/time` rewrite**: new `.qrk` / `.hpp` (or `extern source`); remove old float-day API; smoke + negative tests.
4. **Vectors / tables**: `QVector::DATE`; `IsColumnType`; query lowering paths for date compare and period add; `min`/`max`.
5. **CSV**: parse/write/inference; typed schema dtype; integration tests with a `date` column.
6. **Docs**: `semantics.md`, `stdlib.md`, `grammar.md` (keywords), `error_codes.md` as needed; point tables §9 and stdlib expansion §5 here; changelog Breaking entry.

Compiler-internal names and display: user-facing diagnostics say `date` and `period`, never raw tags.

## 12. Tests

### 12.1 Positive

- Construct, print, compare, `from_days` / `to_days` round trip (including day 0 and negative indexes).
- `ymd` forms: `2022-01-30`, `2022-1-30`, `2022/1/30`.
- Period arithmetic examples in §5.2 (including leap rollback).
- `d + 3` vs `d + time::days(3)`.
- `d2 - d1` across month and year boundaries.
- Table: filter / mutate / sort / group_by / min / max on a date column.
- CSV typed read/write and untyped inference of an all-ISO date column.
- Query with outer `cutoff: date` captured in `filter(e.created_at > cutoff)`.

### 12.2 Negative (Robustness Bar)

- `time::date(2025, 2, 29)` → runtime error.
- Bad `ymd` / `parse` input.
- `d + 1.5`, `d + d`, `sum` of a date column → compile-time errors.
- `null` to a `date` parameter / field.
- `any` carrying a non-date into a `date` parameter → runtime error with location.
- Period used as a table field type → compile-time error.
- CSV typed date cell with `01/30/2022` → parse error naming the column.

## 13. Out of scope

- Time zones beyond UTC vs machine local.
- `timestamp` / `time-of-day` column types (Arrow `timestamp`, `time64`).
- Date literals in source.
- Regex / locale-specific date parsing.
- `float + period`.
- lubridate's full parse family (`mdy`, `dmy`, …) — add later if needed; `ymd` + `parse` with a pattern cover the common cases.
- Interval types (start/end), business-day calendars.

## 14. Open questions

Decided 2026-11-15:

1. **`ymd` failure mode.** `ymd` and `date(...)` are runtime errors. `parse` returns `result`.
2. **`to_int(date)`.** Forbidden. Use `time::to_days`.
3. **Period names are plural** (`months`, `days`, …). `month(d)` stays free as a field accessor.
4. **`format` is for dates.** Instants use `format_time`.
5. **Vec literals of dates fail at compile time** until a later change. A date column is still `vec[date]`.
6. **Untyped CSV** infers `date` after bool and before str, when every present cell is a calendar day.

## Appendix: Mapping to other systems

| Quark | R | Python | Arrow |
|---|---|---|---|
| `date` | `Date` | `datetime.date` | `date32` |
| day index | `as.numeric(Date)` | `date.toordinal()` (different epoch) | int32 days since epoch |
| `d + months(1)` with rollback | `d %m+% months(1)` | `date + relativedelta(months=1)` (similar clamp) | compute kernels |
| instant | `POSIXct` (seconds) | `datetime` / `datetime64` | `timestamp` (later) |
| `period` | lubridate `Period` | `relativedelta` / `DateOffset` | (not a storage type) |
