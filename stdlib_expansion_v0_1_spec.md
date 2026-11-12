# Quark Standard Library Expansion v0.1 Spec

Status: Draft (revision 1)
Last Updated: 2026-11-11
Scope: `std/fmt`, `std/sys`, `std/time`, `std/io`, `std/csv`, a new `std/fs`, and smaller additions to `std/random`. JSON, regex and subprocesses are covered briefly in §9.
Related: `stdlib.md` (current contracts), `ffi_v0_1_spec.md` (how modules bind native code), `semantics.md` §10 (modules)

## 1. Summary

The stdlib grew quickly in November 2026 (`sys`, `time`, `random`, `csv`), but each module covers only its first use case. This spec fills the gaps a developer hits in ordinary scripts: printing several values without `to_str` everywhere, getting the working directory, formatting dates and durations, reading a file line by line, and loading a CSV file with a header.

The goal is not Python or R parity. Each module should cover the common 80% of its domain, so that a newcomer's first script doesn't need a workaround.

Measured problems in the current modules (2026-11-11, Linux x86_64):

- `std/io` writes are unbuffered: writing 1M short lines takes 2.8s, against 0.4s in Python.
- `std/io` never closes a file the program forgot to close. With a 256-descriptor limit (the macOS default), a loop that opens files without `io.close` fails after about 250 files with "Too many open files".
- Reading a whole file needs a guessed size: `io.read(f, 100000000)`.
- There is no way to print to stderr, read a line from a file, list a directory, or get the working directory.

## 2. Design Principles

1. **Modules, not prelude.** New functionality goes in `std/` modules. Prelude builtin names can't be used as variable names, so every new builtin takes a name away from users. The one exception proposed is multi-argument `print` (§3.2, decision 1).
2. **Results for things that can fail because of the environment** (missing file, bad date string). Programming errors (a negative count, an unknown format code) are runtime errors.
3. **Convenience first, then control.** Each module gets one-call helpers for the common case (`io.read_text(path)`) next to the handle-based API (`io.open`, `io.read_line`).
4. **Dicts, not structs, for compound returns.** A struct can't be defined inside a `module` block yet, so stdlib functions return dicts (`time.parts(ts)` gives `{year: ..., month: ...}`).
5. **Compile cost counts.** Glue headers are compiled into every program that imports the module; `extern source` (FFI P2), which would cache them, isn't implemented. A module should add well under a second to a build.
6. **No new syntax.** No string interpolation, no varargs in Quark functions, no keyword arguments; the language-scope decisions of 2026-11-10 stand.

## 3. `std/fmt`: Printing Without `to_str`

### 3.1 Problem

Without interpolation, building a line of output means chaining `+` and `to_str`:

```quark
print('Processed ' + to_str(n) + ' rows in ' + to_str(elapsed) + 's')
```

There is no number formatting either: a float prints all its digits (`0.30000000000000004`), and there is no padding, rounding or thousands separator except `print`'s width argument.

### 3.2 Proposal

**(a) Multi-argument `print` (prelude).** `print(a, b, c, ...)` prints its arguments separated by one space, like Python's `print`:

```quark
print('Processed', n, 'rows in', elapsed, 's')    // Processed 1000 rows in 1.25 s
```

This removes most `to_str` calls in everyday output. It's a prelude change because Quark functions can't take a variable number of arguments, while builtins can (the catalog has `MinArgs`/`MaxArgs`). `print` keeps its current signature (value, end, width, align, pad): its extra positional arguments already mean something, so it can't become variadic without breaking calls.

**(b) `fmt.format(template, args)`**, a subset of Python's format mini-language:

```quark
incl 'std/fmt' as fmt
print(fmt.format('{} scored {:.1f} ({:>5}%)', list [name, score, pct]))
print(fmt.format('{0} vs {1}, {0} wins', list ['A', 'B']))
```

Arguments are a list, since Quark functions have no varargs (decision 2 covers the alternative).

| Spec | Meaning | Example |
|---|---|---|
| `{}`, `{0}` | next or numbered argument, rendered like `print` | `{}` with `[1, 2]` → `[1, 2]` |
| `{:.Nf}` | fixed point, N decimals | `{:.2f}` with `3.14159` → `3.14` |
| `{:.Ne}` | scientific | `{:.1e}` with `12345.0` → `1.2e+04` |
| `{:,}` / `{:,.2f}` | thousands separator | `1234567` → `1,234,567` |
| `{:.N%}` | percentage | `{:.1%}` with `0.256` → `25.6%` |
| `{:>W}`, `{:<W}`, `{:^W}` | right, left, center in width W | `{:>6}` with `42` → `    42` |
| `{:0W}` | zero-pad numbers | `{:05}` with `42` → `00042` |
| `{:x}`, `{:b}` | hex, binary (ints) | `{:x}` with `255` → `ff` |
| `{{`, `}}` | literal braces | |

An unknown spec, a spec that doesn't fit the value (`{:.2f}` on a string), or a missing argument is a runtime error naming the placeholder.

**(c) Small helpers** for the most common single-value cases:

| Function | Result |
|---|---|
| `fmt.fixed(x: float, digits: int = 2) str` | `fmt.fixed(3.14159)` → `3.14` |
| `fmt.num(x, digits: int = -1) str` | thousands separator: `fmt.num(1234567.891, 2)` → `1,234,567.89`; `digits = -1` keeps the shortest form |
| `fmt.percent(x: float, digits: int = 1) str` | `fmt.percent(0.256)` → `25.6%` |
| `fmt.pad(x, width: int, align: str = 'left', fill: str = ' ') str` | like `print`'s width, returning the string |
| `fmt.bytes(n: int) str` | `fmt.bytes(1536)` → `1.5 KiB` |

The existing `show_list`, `show_vec`, `show_dict`, `table`, `head`, `tail` and `full` stay.

## 4. `std/sys`: Process and Environment

### 4.1 Current

`args()`, `env(name)`, `exit(code)`.

### 4.2 Proposal

| Function | Notes |
|---|---|
| `args() list` | existing |
| `program() str` | path the program was started as (`argv[0]`) |
| `env(name: str)` | existing; value or `null` |
| `env_or(name: str, default: str) str` | value, or `default` when unset |
| `set_env(name: str, value: str)` | for child processes (§9.3) |
| `unset_env(name: str)` | |
| `environ() dict` | all variables |
| `exit(code: int = 0)` | existing |
| `cwd() str` | working directory (`pwd`) |
| `chdir(path: str) result` | `err` if it doesn't exist |
| `platform() str` | `'linux'`, `'darwin'` or `'windows'` |
| `arch() str` | `'x86_64'` or `'arm64'` |
| `pid() int` | process id |
| `cpu_count() int` | logical CPUs |
| `eprint(x)`, `eprintln(x)` | write to stderr, rendered like `print` (multi-argument if decision 1 is accepted) |

Filesystem operations (listing directories, making them, removing files) go in `std/fs` (§7), not `sys`: Python split these between `os`, `os.path`, `shutil` and `pathlib`, and one place for them is easier to find. `cwd` and `chdir` stay in `sys` because the working directory is process state; `fs` functions accept relative paths against it.

## 5. `std/time`: Clocks, Dates and Durations

### 5.1 Current

`now()` (epoch seconds), `clock()` (monotonic seconds), `sleep(seconds)`. There is no way to show a timestamp as a date, read a date, or present a duration.

### 5.2 Representation

Timestamps stay floats: seconds since the Unix epoch, UTC. That keeps arithmetic trivial (`ts + time.days(3)`), sorts, and fits vectors. Durations are float seconds too. A dedicated date type would need structs in modules or a new value kind; neither is worth it for v0.1.

### 5.3 Proposal

Durations:

| Function | Result |
|---|---|
| `since(start: float) float` | seconds elapsed since a `clock()` reading |
| `seconds(n)`, `minutes(n)`, `hours(n)`, `days(n)`, `weeks(n)` | seconds: `time.minutes(5)` → `300.0` |
| `duration(seconds: float) str` | human form: `0.25` → `250ms`, `75.5` → `1m 15.5s`, `93784` → `1d 2h 3m 4s` |
| `stopwatch(label: str) float` / `lap(label: str)` | see decision 5 |

Dates (UTC by default; `local: bool = false` switches to the machine's time zone):

| Function | Result |
|---|---|
| `format(ts: float, pattern: str = '%Y-%m-%d %H:%M:%S', local: bool = false) str` | `strftime` patterns |
| `iso(ts: float) str` | ISO 8601 UTC: `2026-11-11T19:24:06Z` (fractional seconds when present) |
| `parse(text: str, pattern: str = '%Y-%m-%d', local: bool = false) result` | `ok(ts)` or `err('cannot parse ...')`; via `std::get_time` (portable; Windows has no `strptime`) |
| `parse_iso(text: str) result` | ISO 8601 with `Z` or `±hh:mm` offsets |
| `parts(ts: float, local: bool = false) dict` | `{year, month, day, hour, minute, second, weekday, yearday}`; weekday 0 = Monday |
| `make(year: int, month: int, day: int, hour: int = 0, minute: int = 0, second: float = 0.0, local: bool = false) float` | the inverse of `parts`; `timegm`/`mktime` |
| `today(local: bool = false) float` | midnight of the current day |

Only UTC and the machine's local zone are supported. Named zones (`Europe/Berlin`) need a time-zone database: C++20's `<chrono>` has one, but Quark compiles as C++17. Howard Hinnant's `date/tz` library (MIT) is the option if this becomes necessary.

## 6. `std/io`: Files

### 6.1 Problems a new developer would hit

Compared with Python's `open()`:

1. **Unclosed files leak.** The `FINALIZABLE` flag on file resources isn't backed by a GC finalizer, so a forgotten `io.close` keeps the descriptor until exit. Python closes files when they're collected and has `with`.
2. **Writes are unbuffered**, one system call per `io.write`: 7× slower than Python for many small writes (§1).
3. **No whole-file or line reading.** `io.read(f, n)` needs a size; there is no `readline`, no line iteration, and no read-everything.
4. **Binary data is truncated at the first NUL byte**, because strings are NUL-terminated. `binary: true` only changes Windows newline handling, so the parameter promises more than it delivers.
5. **No stdout/stderr/stdin handles** (no `print(..., file=sys.stderr)`, no reading all of stdin).
6. **Every call returns a result**, so a three-line read is three `unwrap`s. Results are right for `open`, but reads and writes on an open file rarely fail.
7. **No `flush`, no file size or position query** (`seek(f, 0, 1)` works but reads oddly), **no `exclusive` create mode** (`'x'`) and no read-write mode (`'r+'`).

### 6.2 Proposal

**One-call helpers** (most scripts need only these):

| Function | Result |
|---|---|
| `read_text(path: str) result` | `ok(contents)` |
| `read_lines(path: str) result` | `ok(list of lines)` without line endings (`\n` or `\r\n`) |
| `write_text(path: str, text: str) result` | create or replace |
| `append_text(path: str, text: str) result` | create or append |
| `write_lines(path: str, lines: list) result` | each element rendered like `print`, one per line |

**Handles** (for large files and streaming):

| Function | Notes |
|---|---|
| `open(path, mode = 'r') result` | modes `r`, `w`, `a`, plus `x` (create, fail if it exists) and `r+` (read and write) |
| `read_line(f)` | next line without its ending, or `null` at end of file. (Note, 2026-11-12: `input()` now returns `ok(line)` / `err('end of input')` instead of `null`; decide whether `read_line` should match before implementing it) |
| `read(f, n: int = -1)` | up to `n` bytes; `-1` reads to the end |
| `write(f, text)` | buffered; returns the number of bytes |
| `flush(f)`, `close(f)` | `close` flushes; closing twice stays an `err` |
| `seek(f, offset, whence = 0)`, `tell(f) int` | |
| `stdin()`, `stdout()`, `stderr()` | handles for the standard streams |

Reads and writes on an open handle return plain values and raise a runtime error on failure (a bad handle, a disk error), instead of results: decision 6.

**Runtime changes:**

- A 64 KiB write buffer and read buffer per handle (`read_line` needs the read buffer anyway). The buffer lives in GC-scanned memory without pointers (`q_malloc_atomic`).
- Register a Boehm finalizer for file resources that closes the descriptor if the handle becomes unreachable while still open, and flushes first. This is FFI P3's planned "finalizers as a safety net", applied to files first. Explicit `close` stays the documented API; output may only be flushed by `close`, `flush` or exit.
- Flush all open handles at exit (`atexit`), so a program that never closes its output file doesn't lose the buffered tail.
- Binary data: `binary: true` and NUL bytes need a bytes type or length-carrying strings. That is out of scope here; the docs should say that `std/io` is for text.

## 7. New `std/fs`: Files, Directories and Paths

The things Python has in `os`, `os.path`, `shutil` and `pathlib`, implemented with C++17 `<filesystem>` (portable across Linux, macOS and Windows). Every function that touches the disk returns a result.

| Function | Notes |
|---|---|
| `exists(path)`, `is_file(path)`, `is_dir(path)` | `bool`; `io.exists` stays as an alias |
| `list_dir(path = '.') result` | sorted names |
| `walk(path) result` | every file below `path`, relative paths, sorted |
| `glob(pattern) result` | `*`, `?`, `**`; sorted |
| `mkdir(path, parents: bool = true) result` | no error if it already exists |
| `remove(path) result` | file or empty directory |
| `remove_tree(path) result` | recursive; refuses `'/'` and the home directory |
| `rename(from, to) result`, `copy(from, to) result` | |
| `size(path) result` | bytes |
| `modified(path) result` | epoch seconds, for `std/time` |
| `temp_dir() str`, `temp_file(prefix = 'quark') result` | |
| `join(a, b, ...)` | path joining; see decision 3 for arity |
| `basename(p)`, `dirname(p)`, `stem(p)`, `ext(p)` | `ext('data.csv')` → `'.csv'` |
| `absolute(p) str`, `normalize(p) str` | |

## 8. `std/csv`

### 8.1 Current

`parse(text, sep)` and `read(path, sep)` return rows, each a list of strings. The parser follows RFC 4180 (quotes, doubled quotes, embedded separators and newlines, CRLF), skips blank lines, and on a 1M-row file is slightly faster than Python's `csv` module (1.8s vs 2.1s). Missing:

- **Headers:** the first row comes back like any other.
- **Columns:** data work wants columns (for vectors), not rows.
- **Types:** every field is a string; numbers need `to_float` per cell, and empty cells aren't `null`.
- **Writing:** there's no CSV writer, so output with commas or quotes in fields gets corrupted.
- **Large files:** the whole file is loaded as rows of strings.
- **Dialects:** no quote-character option, no delimiter detection, no comment lines.

### 8.2 Should we wrap a C++ library?

Measured on 2026-11-11 (clang 22, `-O2`; compile time of a file that only includes the header, baseline 0.03s):

| Library | License | Size | Added compile time | Fit |
|---|---|---|---|---|
| [vincentlaucsb/csv-parser](https://github.com/vincentlaucsb/csv-parser) 5.7.2 | MIT | single header, 17,956 lines (680 KB) | 4.0s, 5.3s with one `CSVReader` loop | Fast (mmap, worker thread), delimiter guessing, type detection, writer. The compile cost lands on every program that imports `std/csv` until `extern source` (FFI P2) can compile and cache it once. |
| [d99kris/rapidcsv](https://github.com/d99kris/rapidcsv) | BSD-3-Clause | 2,166 lines | 0.95s | Simple document model (columns by name, typed `GetColumn<T>`, save). Loads the file into `std::vector<std::vector<std::string>>`, then we'd copy everything into Quark values again. |
| [ben-strasser/fast-cpp-csv-parser](https://github.com/ben-strasser/fast-cpp-csv-parser) | BSD-3-Clause | 1,199 lines | — | Column count fixed at compile time; can't read arbitrary files. |
| DuckDB `read_csv` | MIT | `libduckdb` (tens of MB, linked) | link only | The best reader available (dialect and type detection, parallel), and returns columns. It's FFI Phase 3. |

**Recommendation: don't wrap a library for v0.1; extend the in-house parser.** It is 129 lines, correct for RFC 4180, already faster than Python, and adds nothing to compile time beyond the runtime. What's missing (headers, columns, type inference, writing, streaming) is mostly Quark-side work that any wrapper would also need: turning fields into Quark lists, dicts and vectors. csv-parser's speed advantage doesn't pay for 4 to 5 seconds on every build. For large files and analytics, the long-term reader is DuckDB's `read_csv` once `std/duckdb` exists. `std/csv` stays the small, dependency-free option, and its column output should match whatever DuckDB's `query` returns (a dict of vectors, see FFI §9.2 decision 5), so code can move between them.

### 8.3 Proposal

| Function | Result |
|---|---|
| `parse(text, sep = ',') list` | existing: rows of strings |
| `read(path, sep = ',') result` | existing: `ok(rows)` |
| `read_records(path, sep = ',') result` | `ok(list of dicts)` keyed by the header row |
| `read_columns(path, sep = ',', types: bool = true) result` | `ok(dict)` from header name to column. With `types`, a column whose non-empty cells are all integers becomes `vec[i64]`, all numbers `vec[f64]`, all `true`/`false` `vec[bool]`, otherwise `vec[str]`; empty cells become `null`. `fmt.table` already renders this shape. |
| `each(path, f, sep = ',') result` | calls `f(row)` for every row without loading the file; `ok(row count)`. A callback rather than an iterator, since Quark has no iterator protocol. |
| `format(rows, sep = ',') str` | quotes fields that contain `sep`, quotes or newlines |
| `write(path, rows, sep = ',') result` | `rows` is a list of lists, or a list of dicts (header from the first dict's sorted keys) |
| options | `quote: str = '"'` on every reader and writer; `comment: str = ''` skips lines starting with it |

Duplicate or empty header names are an `err` for `read_records` and `read_columns`. Type inference reads the whole column before choosing a type, so one stray string makes the column `vec[str]` (decision 7).

## 9. Other Gaps

### 9.1 `std/random`

Add `uniform(lo, hi)` alongside `uniform()`, `normal(mean = 0.0, sd = 1.0)`, `sample(xs, k)` (without replacement), `weighted(xs, weights)`, and vector generators for data work: `uniform_vec(n)`, `normal_vec(n, mean, sd)`, `randint_vec(n, lo, hi)`.

### 9.2 JSON

The most common format after CSV. Candidates: [nlohmann/json](https://github.com/nlohmann/json) (MIT, single header, but heavy to compile: check before choosing, like §8.2), or an in-house parser of a few hundred lines mapping to dict/list/str/int/float/bool/null. Proposed API: `json.parse(text) result`, `json.read(path) result`, `json.format(x, indent = 0) str`, `json.write(path, x) result`. Dict keys are strings already, so the mapping is direct.

### 9.3 Subprocesses

`sys.run(command: str) result` returning `{code: int, stdout: str, stderr: str}`, via `popen`/`_popen` or `posix_spawn`. A command string goes through the shell, so the docs must warn about quoting untrusted input; `run_args(program, args: list)` avoids the shell.

### 9.4 Regex

`std::regex` is slow and adds noticeable compile time; RE2 (BSD) or PCRE2 (BSD) are fast but are libraries to link. Defer until a concrete need appears; the string methods (`split`, `replace`, `contains`, `startswith`) cover simple cases.

## 10. Dependencies on FFI Phase 2

Measured 2026-11-11 (`quark build`, PCH warm, Linux x86_64): a one-line program builds in 1.1s; the same program importing all six current stdlib modules builds in 3.6s, even if it calls none of them. Nearly all of that is `std/fmt`: its 437-line glue header costs 1.9s, because every import recompiles the glue at `-O3` inside the program's translation unit. This proposal makes the glue much larger (format mini-language, buffered I/O, CSV columns and writer), and some of it needs heavy standard headers on their own: `<filesystem>` adds 0.7s, `<regex>` 0.7s, `<iomanip>` 0.4s; nlohmann/json (48,158 lines) 5.1s.

- **P2 (separately compiled glue sources)** should come first. With stdlib glue compiled once and cached (`extern source`), an import costs only its declarations and a link, so build time stays near 1.1s however much the stdlib grows. Stdlib glue includes only runtime headers, which are already part of the PCH cache key, so the stdlib is the simple first case for P2's dependency tracking. P2 also makes the libraries ruled out in §8.2 and §9.2 for compile cost (csv-parser, nlohmann/json) viable again.
- **P3 (extension handles), at least its finalizers**, should come before the `std/io` rework (§6.2). Closing forgotten files is P3 item 3; doing it for files alone first means writing it twice. P3's dynamic kinds also give `stdin()`/`stdout()`/`stderr()` and later SQLite and DuckDB handles one mechanism. Parent references aren't needed by the stdlib.
- **P4 (result helpers) and `QK-FFI-004`** are small and help every new function that returns a result or checks a handle; do them alongside.

Not dependent on Phase 2: multi-argument `print` (prelude), `std/sys` and `std/time` additions (small headers), `std/random`, and the in-house CSV logic. These can proceed first or in parallel.

FFI Phase 2 as specified ends with `std/sqlite`; the stdlib needs only P2, P3 and P4 from it, so the order can be P2, P3, P4, then stdlib phase 1, then SQLite.

## 11. Rollout

| Phase | Contents | Done when |
|---|---|---|
| 1. Daily friction | multi-argument `print` (if decision 1 is accepted), `sys.eprintln`, `fmt.format` and helpers, `io` whole-file and line helpers, buffering, file finalizers and flush at exit | the CSV report and a log-writing script need no `to_str` chains, guessed sizes or manual flushing; `io` write benchmark within 2× of Python |
| 2. Scripting | `sys` additions, `std/fs`, `std/time` dates and durations | a script can find files, check modification times and print readable dates and durations |
| 3. Data in and out | `csv` records, columns with types, writer, streaming; `random` additions | a CSV file with a header loads into a dict of typed vectors and writes back unchanged |
| 4. Formats | JSON, subprocesses | — |

Every function gets a smoke test, an entry in `stdlib.md`, and editor data (`go test ./internal/editordata -update`).

## 12. Decisions

### 12.1 Open

1. **Multi-argument `print`.** Accept the prelude change (§3.2a), or keep `print` single-argument and rely on `fmt.format`? Recommended: accept. It's the change that removes the most `to_str` calls, and it adds no new name.
2. **`fmt.format` arguments.** A list (`fmt.format(t, list [a, b])`, proposed), or a prelude builtin `format(t, a, b, ...)` that can be variadic like `print`? The builtin reads better but takes the name `format` from users.
3. **`fs.join` arity.** Quark functions have no varargs: `fs.join(a, b)` with two arguments (nest for more), or `fs.join(list [a, b, c])`?
4. **`std/fs` vs. more in `std/io`.** A separate module (proposed), or put directory and path functions in `std/io`?
5. **Timing helpers.** Is `time.since(start)` plus `time.duration(s)` enough, or do you want a named stopwatch (`stopwatch('load')` … `lap('load')` printing `load: 1.25s`)? The named form needs module-level state.
6. **Results on open handles.** Make `read`, `read_line`, `write` and `flush` on an open handle return plain values with runtime errors on failure (proposed, like Python's exceptions for rare I/O errors), and keep results for `open` and the path-based helpers? This changes the current `io.read`/`io.write` signatures.
7. **CSV type inference.** Whole-column inference (proposed), or only the first N rows, as DuckDB samples? Whole-column is exact; sampling is faster on huge files but can fail late.
8. **CSV library.** Agree to stay in-house for v0.1 and revisit after DuckDB (§8.2)?
