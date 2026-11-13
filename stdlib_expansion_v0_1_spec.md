# Quark Standard Library Expansion v0.1 Spec

Status: Draft proposal, revision 2 (2026-11-13). Nothing here is implemented. CSV is superseded by `tables_v0_1_spec.md` (D8, D15, D17, tables §8), and decisions 7 and 8 are answered there.
Scope: `std/fmt`, `std/sys`, `std/time`, `std/io`, a new `std/fs`, and additions to `std/random`. JSON, regex and subprocesses are covered briefly in §9.
Related: `stdlib.md` (current API), `ffi_v0_1_spec.md` (how modules bind native code), `modules_v0_2_spec.md` (modules), `tables_v0_1_spec.md` (CSV and tables)

## Contents

- [1. Summary](#1-summary)
- [2. Design Principles](#2-design-principles)
- [3. `std/fmt`: Printing Without `to_str`](#3-stdfmt-printing-without-to_str)
- [4. `std/sys`: Process and Environment](#4-stdsys-process-and-environment)
- [5. `std/time`: Clocks, Dates and Durations](#5-stdtime-clocks-dates-and-durations)
- [6. `std/io`: Files](#6-stdio-files)
- [7. New `std/fs`: Files, Directories and Paths](#7-new-stdfs-files-directories-and-paths)
- [8. `std/csv`](#8-stdcsv)
- [9. Other Gaps](#9-other-gaps)
- [10. Dependencies on FFI Phase 2](#10-dependencies-on-ffi-phase-2)
- [11. Rollout](#11-rollout)
- [12. Decisions](#12-decisions)

## 1. Summary

Each stdlib module (`io`, `fmt`, `sys`, `time`, `random`) covers only its first use case. This spec fills the gaps a developer hits in ordinary scripts: printing several values without `to_str` everywhere, getting the working directory, formatting dates and durations, and reading a file whole or line by line.

The goal is not Python or R parity. Each module should cover the common 80% of its domain, so that a newcomer's first script doesn't need a workaround.

Measured problems (2026-11-11, Linux x86_64):

- `std/io` writes are unbuffered: writing 1M short lines takes 2.8s, against 0.4s in Python.
- `std/io` never closes a file the program forgot to close. With a 256-descriptor limit (the macOS default), a loop that opens files without `io::close` fails after about 250 files with "Too many open files".
- Reading a whole file needs a guessed size: `io::read(f, 100000000)`.
- There is no way to print to stderr, read a line from a file, list a directory, or get the working directory.

## 2. Design Principles

1. **Modules, not prelude.** New functionality goes in `std/` modules. Prelude builtin names can't be redefined or assigned, so every new builtin takes a name away from users. The one prelude change considered is multi-argument `print` (§3.2, decision 1).
2. **Results for failures caused by the environment** (missing file, bad date string). Programming errors (a negative count, an unknown format code) are runtime errors.
3. **Convenience first, then control.** Each module gets one-call helpers for the common case (`io::read_text(path)`) next to the handle-based API (`io::open`, `io::read_line`).
4. **Structs for compound returns.** A module can define structs (`time::Parts`), which give every field its own type. A dict keeps one value type, so a dict mixing ints and strings would have to hold `any` and every read of it would be unchecked (decision 9).
5. **Compile cost counts.** Glue headers are compiled into every program that imports the module; `extern source` (FFI P2), which would cache them, isn't implemented. A module should add well under a second to a build.
6. **No new syntax.** No string interpolation, no varargs in Quark functions, no keyword arguments (language-scope decisions of 2026-11-10).

## 3. `std/fmt`: Printing Without `to_str`

### 3.1 Problem

Without interpolation, a line of output is a chain of `+` and `to_str`:

```quark
print('Processed ' + to_str(n) + ' rows in ' + to_str(elapsed) + 's')
```

There is no number formatting either: no fixed decimals, rounding or thousands separator; only `print`'s width, align and pad arguments.

### 3.2 Proposal

**(a) Multi-argument `print` (prelude).** `print(a, b, c, ...)` would print its arguments separated by one space, like Python's `print`:

```quark
print('Processed', n, 'rows in', elapsed, 's')    // Processed 1000 rows in 1.25 s
```

This removes most `to_str` calls in everyday output, and only a builtin can do it, since Quark functions have no varargs (the catalog has `MinArgs`/`MaxArgs`). But `print`'s positional arguments 2-5 already mean `end`, `width`, `align` and `pad` (`print(x, '')` prints without a newline), so a variadic `print` is a breaking change: those options would need another form, such as a separate builtin for padded output. Decision 1.

**(b) `fmt::format(template, args)`**, a subset of Python's format mini-language:

```quark
incl 'std/fmt' as fmt
print(fmt::format('{} scored {:.1f} ({:>5}%)', list [name, score, pct]))
print(fmt::format('{0} vs {1}, {0} wins', list ['A', 'B']))
```

Arguments are a list, since Quark functions have no varargs (decision 2 covers the alternative). A list literal with values of several types holds all of them, so `list [name, score, pct]` works as written.

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

An unknown spec, a spec that doesn't fit the value (`{:.2f}` on a string), or a missing argument is a runtime error naming the placeholder. When the template is a string literal and the argument list's types are known, the compiler can check all three at compile time.

**(c) Small helpers** for the most common single-value cases:

| Function | Result |
|---|---|
| `fmt::fixed(x: float, digits: int = 2) str` | `fmt::fixed(3.14159)` → `3.14` |
| `fmt::num(x: float, digits: int = -1) str` | thousands separator: `fmt::num(1234567.891, 2)` → `1,234,567.89`; `digits = -1` keeps the shortest form |
| `fmt::percent(x: float, digits: int = 1) str` | `fmt::percent(0.256)` → `25.6%` |
| `fmt::pad(x, width: int, align: str = 'left', fill: str = ' ') str` | like `print`'s width, returning the string; an `align` other than `left`, `right` or `center` is an error |
| `fmt::bytes(n: int) str` | `fmt::bytes(1536)` → `1.5 KiB` |

The existing `show_list`, `show_vec`, `show_dict`, `table`, `head`, `tail` and `full` stay.

## 4. `std/sys`: Process and Environment

| Function | Notes |
|---|---|
| `args() list` | exists |
| `program() str` | path the program was started as (`argv[0]`) |
| `env(name: str)` | exists; the value or `null` |
| `env_or(name: str, default: str) str` | the value, or `default` when unset; never `null`, so the result is a `str` |
| `set_env(name: str, value: str)` | for child processes (§9.3) |
| `unset_env(name: str)` | |
| `environ() dict` | all variables (a `dict[str, str]`) |
| `exit(code: int = 0)` | exists; a code outside 0-255 should be an error (today only its low 8 bits reach the OS) |
| `cwd() str` | working directory (`pwd`) |
| `chdir(path: str) result` | `err` if it doesn't exist |
| `platform() str` | `'linux'`, `'darwin'` or `'windows'` |
| `arch() str` | `'x86_64'` or `'arm64'` |
| `pid() int` | process id |
| `cpu_count() int` | logical CPUs |
| `eprint(x, end: str = '\n')` | write to stderr, rendered like `print` (multi-argument if decision 1 is accepted) |

Filesystem operations (listing, creating and removing directories and files) go in `std/fs` (§7), not `sys`: Python split these between `os`, `os.path`, `shutil` and `pathlib`, and one place is easier to find. `cwd` and `chdir` stay in `sys` because the working directory is process state; `fs` functions resolve relative paths against it.

## 5. `std/time`: Clocks, Dates and Durations

### 5.1 Representation

`now()` (epoch seconds), `clock()` (monotonic seconds) and `sleep(seconds)` exist. There is no way to show a timestamp as a date, read a date, or present a duration.

Timestamps stay floats: seconds since the Unix epoch, UTC. That keeps arithmetic trivial (`ts + time::days(3)`), sorts, and fits vectors. Durations are float seconds too. A dedicated date type isn't worth it for v0.1; the tables work plans a `date` column type (`tables_v0_1_spec.md` §9), and this module should convert to and from it when it exists.

### 5.2 Proposal

Durations:

| Function | Result |
|---|---|
| `since(start: float) float` | seconds elapsed since a `clock()` reading |
| `seconds(n)`, `minutes(n)`, `hours(n)`, `days(n)`, `weeks(n)` | seconds: `time::minutes(5)` → `300.0` |
| `duration(seconds: float) str` | human form: `0.25` → `250ms`, `75.5` → `1m 15.5s`, `93784` → `1d 2h 3m 4s` |
| `stopwatch(label: str) float` / `lap(label: str)` | see decision 5 |

Dates (UTC by default; `local: bool = false` switches to the machine's time zone):

| Function | Result |
|---|---|
| `format(ts: float, pattern: str = '%Y-%m-%d %H:%M:%S', local: bool = false) str` | `strftime` patterns |
| `iso(ts: float) str` | ISO 8601 UTC: `2026-11-11T19:24:06Z` (fractional seconds when present) |
| `parse(text: str, pattern: str = '%Y-%m-%d', local: bool = false) result` | `ok(ts)` or `err('cannot parse ...')`; via `std::get_time` (portable; Windows has no `strptime`) |
| `parse_iso(text: str) result` | ISO 8601 with `Z` or `±hh:mm` offsets |
| `parts(ts: float, local: bool = false) time::Parts` | a struct with `year`, `month`, `day`, `hour`, `minute`, `weekday`, `yearday` (ints) and `second` (float); weekday 0 = Monday |
| `make(year: int, month: int, day: int, hour: int = 0, minute: int = 0, second: float = 0.0, local: bool = false) float` | the inverse of `parts`; `timegm`/`mktime` |
| `today(local: bool = false) float` | midnight of the current day |

Only UTC and the machine's local zone are supported. Named zones (`Europe/Berlin`) need a time-zone database: C++20's `<chrono>` has one, but Quark compiles as C++17. Howard Hinnant's `date/tz` library (MIT) is the option if this becomes necessary.

## 6. `std/io`: Files

### 6.1 Problems a new developer would hit

Compared with Python's `open()`:

1. **Unclosed files leak.** File resources carry a `FINALIZABLE` flag, but no GC finalizer is registered, so a forgotten `io::close` keeps the descriptor until exit. Python closes files when they're collected and has `with`.
2. **Writes are unbuffered**, one system call per `io::write`: 7× slower than Python for many small writes (§1).
3. **No whole-file or line reading.** `io::read(f, n)` needs a size; there is no `read_line`, no line iteration, and no read-everything.
4. **Binary data is cut at the first NUL byte**, because strings are NUL-terminated. `binary: true` only changes Windows newline handling, so the parameter promises more than it delivers.
5. **No stdout/stderr/stdin handles** (no `print(..., file=sys.stderr)`, no reading all of stdin).
6. **Every call returns a result**, so a three-line read is three `unwrap`s. Results are right for `open`, but reads and writes on an open file rarely fail.
7. **No `flush`, no position query** (`io::seek(f, 0, 1)` works but reads oddly), **no exclusive create mode** (`'x'`) and no read-write mode (`'r+'`).

### 6.2 Proposal

**One-call helpers** (most scripts need only these):

| Function | Result |
|---|---|
| `read_text(path: str) result` | `ok(contents)` |
| `read_lines(path: str) result` | `ok(list[str])`, lines without their endings (`\n` or `\r\n`) |
| `write_text(path: str, text: str) result` | create or replace |
| `append_text(path: str, text: str) result` | create or append |
| `write_lines(path: str, lines: list) result` | each element rendered like `print`, one per line |

**Handles** (for large files and streaming):

| Function | Notes |
|---|---|
| `open(path, mode = 'r') result` | modes `r`, `w`, `a`, plus `x` (create, fail if it exists) and `r+` (read and write) |
| `read_line(f)` | the next line without its ending; end of file: decision 10 |
| `read(f, n: int = -1)` | up to `n` bytes; `-1` reads to the end |
| `write(f, text)` | buffered; returns the number of bytes |
| `flush(f)`, `close(f)` | `close` flushes; closing twice stays an `err` |
| `seek(f, offset, whence = 0)`, `tell(f) int` | |
| `stdin()`, `stdout()`, `stderr()` | handles for the standard streams |

Reads and writes on an open handle return plain values and raise a runtime error on failure (a bad handle, a disk error), instead of results: decision 6.

**Runtime changes:**

- A 64 KiB write buffer and read buffer per handle (`read_line` needs the read buffer anyway). The buffer lives in GC memory without pointers (`q_malloc_atomic`).
- A Boehm finalizer for file resources that flushes and closes the descriptor if the handle becomes unreachable while still open. This is FFI P3's "finalizers as a safety net", applied to files first. Explicit `close` stays the documented API; output may only be flushed by `close`, `flush` or exit.
- Flush all open handles at exit (`atexit`), so a program that never closes its output file doesn't lose the buffered tail.
- Binary data: `binary: true` and NUL bytes need a bytes type or length-carrying strings. That is out of scope; `std/io` is for text.

## 7. New `std/fs`: Files, Directories and Paths

The things Python has in `os`, `os.path`, `shutil` and `pathlib`, implemented with C++17 `<filesystem>` (portable across Linux, macOS and Windows). Every function that touches the disk returns a result.

| Function | Notes |
|---|---|
| `exists(path)`, `is_file(path)`, `is_dir(path)` | `bool`; `io::exists` stays as an alias |
| `list_dir(path = '.') result` | sorted names (`list[str]`) |
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

Superseded by `tables_v0_1_spec.md`. The reader is written in-house and parses straight into typed columns (D8, tables §8); `std/csv` is rebuilt on it as `csv::read_file`, `csv::write_file` and `csv::parse_text`, returning tables, and `csv::parse` / `csv::read` are removed (D15); columns are matched to struct fields by name, with a literal `dict` to rename (D17).

## 9. Other Gaps

### 9.1 `std/random`

Add `uniform(lo, hi)` next to `uniform()`, `normal(mean = 0.0, sd = 1.0)`, `sample(xs, k)` (without replacement), `weighted(xs, weights)`, and vector generators for data work: `uniform_vec(n)`, `normal_vec(n, mean, sd)`, `randint_vec(n, lo, hi)`. Two functions can't share a name in one module, so `uniform(lo, hi)` would be default parameters on one `uniform(lo: float = 0.0, hi: float = 1.0)`.

### 9.2 JSON

The most common format after CSV. Candidates: [nlohmann/json](https://github.com/nlohmann/json) (MIT, single header, but 5.1s of compile time per importing program until FFI P2, §10), or an in-house parser of a few hundred lines mapping to dict/list/str/int/float/bool/null. Proposed API: `json::parse(text) result`, `json::read(path) result`, `json::format(x, indent = 0) str`, `json::write(path, x) result`. Dict keys are strings already, so objects map directly. JSON values are mixed, so parsed lists and dicts hold any value (`list`, `dict` without an element type); reading them into typed values goes through typed declarations or struct literals, which check at runtime.

### 9.3 Subprocesses

`sys::run(command: str) result` returning `ok(sys::Completed)`, a struct with `code: int`, `stdout: str` and `stderr: str`, via `popen`/`_popen` or `posix_spawn`. A command string goes through the shell, so the docs must warn about quoting untrusted input; `run_args(program, args: list)` avoids the shell.

### 9.4 Regex

`std::regex` is slow and adds noticeable compile time; RE2 (BSD) or PCRE2 (BSD) are fast but are libraries to link. Defer until a concrete need appears; the string methods (`split`, `replace`, `contains`, `startswith`) cover simple cases.

## 10. Dependencies on FFI Phase 2

Measured 2026-11-11 (`quark build`, PCH warm, Linux x86_64): a one-line program builds in 1.1s; the same program importing `io`, `fmt`, `sys`, `csv`, `time` and `random` builds in 3.6s, even if it calls none of them. Nearly all of that is `std/fmt`: its 437-line glue header costs 1.9s, because every import recompiles the glue at `-O3` inside the program's translation unit. This proposal makes the glue much larger (format mini-language, buffered I/O), and some of it needs heavy standard headers on their own: `<filesystem>` adds 0.7s, `<regex>` 0.7s, `<iomanip>` 0.4s; nlohmann/json (48,158 lines) 5.1s.

- **P2 (separately compiled glue sources)** should come first. With stdlib glue compiled once and cached (`extern source`), an import costs only its declarations and a link, so build time stays near 1.1s however much the stdlib grows. Stdlib glue includes only runtime headers, which are already part of the PCH cache key, so the stdlib is the simple first case for P2's dependency tracking. P2 also makes nlohmann/json (§9.2) affordable.
- **P3 (extension handles), at least its finalizers**, should come before the `std/io` rework (§6.2). Closing forgotten files is part of P3; doing it for files alone first means writing it twice. P3's dynamic handle kinds also give `stdin()`/`stdout()`/`stderr()` and later SQLite and DuckDB handles one mechanism. Parent references aren't needed by the stdlib.
- **P4 (result helpers) and `QK-FFI-004`** are small and help every new function that returns a result or checks a handle; do them alongside.

Not dependent on P2: multi-argument `print` (prelude), the `std/sys` and `std/time` additions (small headers), and `std/random`. These can go first or in parallel.

FFI Phase 2 as specified ends with `std/sqlite`; the stdlib needs only P2, P3 and P4 from it, so the order can be P2, P3, P4, then stdlib phase 1, then SQLite.

## 11. Rollout

| Phase | Contents | Done when |
|---|---|---|
| 1. Daily friction | multi-argument `print` (if decision 1 is accepted), `sys::eprint`, `fmt::format` and helpers, `io` whole-file and line helpers, buffering, file finalizers and flush at exit | a log-writing script needs no `to_str` chains, guessed sizes or manual flushing; the `io` write benchmark is within 2× of Python |
| 2. Scripting | `sys` additions, `std/fs`, `std/time` dates and durations | a script can find files, check modification times and print readable dates and durations |
| 3. Data generators | `random` additions | `normal_vec(1000000, 0.0, 1.0)` builds a `vec[f64]` without a list in between |
| 4. Formats | JSON, subprocesses | — |

Every function gets a smoke test, negative tests for its misuse paths (wrong type, `null`, an `any` value), an entry in `stdlib.md`, and editor data (`go test ./internal/editordata -update`).

## 12. Decisions

### 12.1 Open

1. **Multi-argument `print`.** Make `print` variadic (§3.2a), a breaking change that moves `end`/`width`/`align`/`pad` elsewhere, or keep `print` as it is and rely on `fmt::format`? Variadic `print` removes the most `to_str` calls and adds no new name.
2. **`fmt::format` arguments.** A list (`fmt::format(t, list [a, b])`, proposed), or a prelude builtin `format(t, a, b, ...)` that can be variadic like `print`? The builtin reads better but takes the name `format` from users.
3. **`fs::join` arity.** Quark functions have no varargs: `fs::join(a, b)` with two arguments (nest for more), or `fs::join(list [a, b, c])`?
4. **`std/fs` vs. more in `std/io`.** A separate module (proposed), or directory and path functions in `std/io`?
5. **Timing helpers.** Is `time::since(start)` plus `time::duration(s)` enough, or do you want a named stopwatch (`stopwatch('load')` … `lap('load')` printing `load: 1.25s`)? The named form needs state kept between calls; an imported file can only hold definitions and constants, so the state would live in the glue header.
6. **Results on open handles.** Make `read`, `read_line`, `write` and `flush` on an open handle return plain values with runtime errors on failure (proposed, like Python's exceptions for rare I/O errors), and keep results for `open` and the path-based helpers? This changes the current `io::read`/`io::write` signatures.
9. **Structs for compound returns** (§2 principle 4). Return module structs (`time::Parts`, `sys::Completed`) instead of dicts?
10. **`read_line` at end of file.** Return a result like `input()` (`ok(line)`, then `err('end of input')`), or `null` at end of file? `null` keeps the plain-value style of decision 6, but a function that can return `null` can't be annotated `str`, so its result is untyped.

### 12.2 Answered by the tables spec

7. **CSV type inference**: from every value of the column (`tables_v0_1_spec.md` D8, tables §8), and from the struct when the table is typed (D2).
8. **CSV library**: in-house, no third-party library (D8).
