# Changelog

Notable changes to the Quark language, compiler, runtime, stdlib and tooling.

Quark has no tagged releases yet, so entries are grouped by date and by piece of work, newest first. Each entry lists the commits it covers. Entries were written from the diffs, tests and smoke programs, not from the commit messages, which are often vague (`huge chunk of changes`, `minor QOL improvments`) or misleading (`fix returns inside if` fixes scalar assignment codegen; Quark has no `return`).

Sections follow [Keep a Changelog](https://keepachangelog.com/): **Breaking**, **Added**, **Changed**, **Fixed**, **Removed**, **Internal**. "Breaking" covers anything that can make a program that worked before fail to compile, fail at runtime, or print something different.

---

## Known issues (as of `4294dab` plus uncommitted changes)

These were checked against the current compiler. They fail loudly, but with a C++ error, a wrong location or an unhelpful message, or by rejecting a valid program.

- **A file included without an alias by two other files is rejected.** If `left.qrk` and `right.qrk` both do `incl './base.qrk'` and the main file includes both, the program fails with `symbol 'base__twice' from module 'base' conflicts with existing definition`. Including it with an alias (`incl './base.qrk' as b`) works.
- A `struct` defined inside a `module` block fails with `internal: struct 'L' not pre-declared`. A qualified literal (`shapes.Sq { side: 1 }`) fails with `no prefix parse function for LBRACE`. Both are deferred to modules v0.2 (`modules_v0_2_spec.md` §2 and §7).
- A runtime error inside a `for` loop body is reported at the `for` line, not at the statement that failed. For example, `print(r % 0)` on line 4 of a loop starting on line 2 reports `modulo by zero at file.qrk:2:1`.

---

## 2026-11-12: Function values, `fn` type, call-shape checking, keyword renames, struct field checks, name clashes, `input()` result, fixed `fn` declarations

Commits: `c0ac8cd`, `ac274c0`, `6dc11a0` (keyword renames), `2e62209` (struct fields, dict keys), `4294dab` (name clashes, shadowing, `input()`); fixed `fn` declarations (modules v0.2 step 1) not yet committed

### Breaking
- **`input()` returns a `result`.** It returns `ok(line)` (`ok('')` for an empty line), or `err('end of input')` once stdin is exhausted. It used to return a `str`, or `null` at end of input. Using the value as a string is now a compile error (`len(input())`, `input().upper()`, a `str` parameter), so code must unwrap it:
  ```
  name = unwrap(input('Name: '))       // was: name = input('Name: ')

  while true:                          // was: line = input(); if line == null: break
      r = input()
      if is_err(r):
          break
      line = unwrap(r)
      print(line)
  ```
- **A `result` is never `null`.** `null` can no longer be passed, stored, returned or used as a default where a `result` is expected. This used to be allowed, as for `list` and `dict`. It is a compile error when the compiler can see it (`fn f(r: result)` called with `f(null)`, `r: result = null`, `r = ok(1)` then `r = null`, a `result` struct field, `fn f() result -> null`, `r: result = null` as a default). Otherwise it is a runtime error (`argument 'r' of 'f' expects result, got null`). Comparing a result with `null` is a compile error: `a result is never null, so this comparison is always false; use is_ok(r), is_err(r) or when to check it`. This catches the old `line == null` loop, which would otherwise never stop.
- **A top-level name defined in two files is a compile error.** It used to resolve silently to one of them. The error names both places:
  ```
  incl 'std/fmt' as fmt
  fn fmt_full(x) -> 'mine'
  // 'fmt_full' is already defined as an extern fn (std/fmt.qrk:7); every top-level
  // name of an included file is visible to the importer, so rename this one
  ```
  This covers `fn`s, variables, `struct`s and `extern fn`s, between the main file and an included file, between two included files, and in either order. An `extern fn` and another definition with the same name in one file is also an error. Before, an importer's `fn helper` hijacked a library's internal calls to its own `helper`, and an importer's `fn fmt_full` was ignored. A file included twice (directly and transitively) is spliced once and doesn't clash with itself.
- **Redefining or assigning a builtin is a compile error**: `fn len(x) -> 42`, `len = fn(x) -> 42`, `fn print(x) -> 1`, `sum = 0`, and `max = 1` inside a function. All report `'len' is a builtin function and can't be redefined or assigned; choose another name`. The `fn` forms used to be silently ignored, so `len('ab')` still called the builtin. The value forms failed with a confusing type error.
- A default of `null` for an annotated parameter it can't satisfy (`x: int = null`, `r: result = null`) is a compile error. It used to fail at runtime on every call that omitted the argument.

- **`fn` declarations are fixed** (modules v0.2 spec, rules 17-21). Each of these used to compile and is now an error:
  - Declaring a name twice in one scope. The second definition used to win silently: `fn f(x) -> x`, then `fn f(x) -> x + 1`, then `print(f(1))` printed `2`. Now: `'f' is already defined on line 1`.
  - Assigning to a declared name, as a function or any other value (`f = fn(x) -> 0`, `f = 3`, `f: int = 2`, also from inside another function): `'f' is a function declaration (line 1) and can't be reassigned; use a variable: g = fn ...`.
  - A nested `fn` reusing a name that is visible there: an outer function, variable or parameter (`'f' is already a function in an enclosing scope (line 1); choose another name`), or a builtin (`fn len` inside a function used to shadow `len`; it is now `'len' is a builtin function and can't be redefined or assigned`). Parameters, loop variables and `when` bindings may still shadow.

  To swap a function at runtime, hold it in a variable:
  ```
  g = fn(x) -> x + 1
  g = fn(x) -> 0        // fine: g is a variable
  ```
- **A function body can't name a `g = fn(...)` variable assigned below it.** `fn a() -> g(1)`, then `g = fn(x) -> x + 1`, used to work because such assignments were predeclared. It is now `undefined identifier 'g'`, as for any variable. Declare it with `fn g(x) -> x + 1` instead, which is hoisted.

- **Renamed keywords and builtins.** The old spellings no longer compile:

  | Old | New |
  |---|---|
  | `use 'std/io' as io`, `use mathx as m` | `incl 'std/io' as io`, `incl mathx as m` |
  | `vector [1, 2, 3]`, `x: vector`, `fn f() vector` | `vec [1, 2, 3]`, `x: vec`, `fn f() vec` |
  | `xs.to_vector()` | `xs.to_vec()` |
  | `println(x)` | `print(x)` |
  | `fmt_vector(v, n, true)` (the `std/fmt` extern binding) | `fmt_vec(v, n, true)`, or `fmt.show_vec(v)` |

  `incl` was chosen because importing a file doesn't "use" everything in it.
- `incl` and `vec` are reserved words now. A variable, parameter or function named `vec` or `incl` is a parse error. `use` and `vector` are ordinary names again.
- **One print function.** `println` is gone. `print(x)` prints `x` followed by `end`, which defaults to `'\n'`, so each call prints its own line (this is what `print(x)` already did). Pass `end` positionally to change it: `print(x, '')` prints no newline, `print(x, ' | ')` a separator. The optional `width`, `align` and `pad` arguments are unchanged.
- **Output says `vec`.** `type(vec [1.5])` returns `'vec[f64]'` (was `'vector[f64]'`), and code that compares `type()` strings must be updated. `fmt.show_vec` ends with `[vec[i64]: 2 elements]`. Compile-time and runtime messages name the type `vec`, for example `vec cannot be used as a condition` and `argument 'x' of 'f' expects vec, got list`.

- New compile errors. Programs that pass a callback with the wrong arity or argument types, or pass a non-function where the body calls the parameter, are now rejected (see *Call shapes* below). They used to compile and then misbehave at runtime.
- Native/extension C++ API:
  - `QClosure` has a new `const QFnInfo* info` field.
  - `q_alloc_closure(fn, info, n)` and `qv_func(fn, info)` take a new `QFnInfo*` argument.
  - `q_require_callable` was replaced by `q_require_closure`.
  - Hand-written glue that builds closures must be updated.
- **`d.key` on a missing dict key is a runtime error** instead of returning `null`: `dict has no key 'nmae'; use .get('nmae') if the key may be missing`. Code that relied on `d.key` reading `null` must use `d.get('key')`.
  ```
  rows = list [dict {name: 'b', amount: 2}]
  for r in rows:
      print(r.amonut)      // used to print null; now stops with an error
  ```
- **Struct field values of unknown type are checked when the literal is built.** `fn mk(a) -> P { x: a }` with `x: int` used to accept `mk('hello')`; it now stops with `field 'x' of P expects int, got str`.
- A fully annotated (native) function now checks its result at runtime. For example, `fn nat(f: fn) int -> f(1)` fails with `expected int, got str` when the callback returns a string; it used to read garbage.

### Added
- **`fn` annotation type.** Means "any function value", with no signature. It works on parameters, return types, variables and struct fields, and accepts `null`.
  ```
  fn apply(g: fn, x: int) -> g(x)
  fn pick(up: bool) fn -> ...
  cb: fn = mx.inc
  struct Handler:
      run: fn
  ```
- **Module members as values**, for example `f = io.exists`, `list [mx.inc, io.exists]`, `apply(mx.inc, 2)`. An unknown member is a compile error: `module 'io' has no symbol 'nope'`.
- **`extern fn` callbacks.** An `fn` parameter or return maps to `QClosure*`. Glue code calls it with `qext::call` and can return it with the new `qext::box(QClosure*)`. Example: `extern fn apply_n(f: fn, n: int, x: any) any`.
- **Call shapes: compile-time checks on callbacks.** The analyzer records how a function body calls its parameters, and checks every function passed in against those calls:
  - **Arity:** `fn apply(k) -> k(1)`, then `apply(h)` where `h` takes 2 → `argument 1 of 'apply' is called with 1 argument (line 2), but 'h' takes 2 arguments`.
  - **Argument types:** `apply(fn(x: int) -> x, 'str')` → `... is called with str as argument 1 ..., but the lambda passed expects int`.
  - **Pass-through:** a parameter handed on to another function inherits that function's calls, at any depth (`fn wrap(k) -> apply(k)` is checked at `wrap(h)`).
  - **Non-functions:** `apply(5)` → `... is called as a function (line 1), but gets int`. `null` is still allowed.
  - **Lists of functions:** `fs[0](1, 2, 3)` → `none of the functions this value can be accepts 3 arguments`.
  - This covers direct, module-member, pipe and lambda arguments. It doesn't track a parameter the body reassigns.
- **Runtime arity checks and default filling** for calls the compiler can't resolve (values from `any`, lists, struct fields, native code):
  - Errors look like `function 'h' expects 2 arguments but got 1` or `function 'g' expects 1-2 arguments but got 0`.
  - Omitted default arguments are built fresh on every call.
  - `qext::call` from native code gets the same checks.
- A list of functions with different signatures can be indexed and called (`fs[0](9)`). This used to fail with "expression is not callable".
- **`fn` declarations are hoisted** (rule 22). Every `fn` of a scope (the file, a module body or a block) exists before the scope's first statement runs, so calls work in any order and mutual recursion needs no particular order. A statement calling a function defined further down used to be a compile error (`'f' is called before its definition on line N`); through another function it failed at runtime.
  ```
  print(is_even(10))   // true
  fn is_even(n) -> ...is_odd(n - 1)...
  fn is_odd(n) -> ...is_even(n - 1)...
  ```
- **Use-before-assignment checks for hoisted functions** (rule 23). Variables are not hoisted: a body only sees variables assigned above it. A call that would run a function before a variable it uses is assigned is rejected when the order is visible, including through other declared functions and pipes:
  ```
  report()             // error: 'report' uses 'total', which is assigned on line 2, after this call
  total = 5
  fn report() -> print(total)
  ```
  Through a function value (`g = report`, then `g()`), it is a runtime error: `'total' is used before it is assigned`, never `null`.
- **Struct literals and struct return types work in an importing file.** With `struct Point` defined at the top level of `lib/geo.qrk`, the importer can write `Point { x: 1 }` and `fn mk(a: int) Point -> ...`. Both used to be parse errors (`no prefix parse function for LBRACE`). A misspelled struct name is now a check error: `unknown struct type 'Pnt'` or `unknown type 'Pnt'`.

### Changed
- **A declared function that captures no variables is a plain C++ function.** Calls to it are direct (`quark_f(nullptr, ...)`), with no binding, closure allocation or indirect call. Before, every named function lived in a closure and was called through it, unless it was fully annotated. Whether a function is native (fully annotated) is now decided after the whole program is analyzed, so a fully annotated function that calls another non-capturing function is native too. A declared function that captures a variable stays a closure, held in a binding that never changes.
- A fully annotated lambda assigned to a variable (`g = fn(x: int) int -> x`) is now a closure like any other variable. Only `fn` declarations compile to native C++ functions.
- Tables spec (`tables_v0_1_spec.md`, still not implemented): the literal is `tbl { ... }` and the type annotation is `tbl` (`fn f(t: tbl)`). The runtime type name for a table is `tbl`. `fmt.table(...)` keeps its name.

### Fixed
- **Calling a fully annotated function declared below its caller failed in clang.** `fn a(n) -> b(n) + 1`, then `fn b(n: int) int -> n * 2`, stopped with `assigning to 'QCell *' from incompatible type 'int64_t (QClosure *, int64_t)'`. It now prints `11` for `a(5)`.
- **Reassigning a fully annotated function failed in clang** (`redefinition of 'quark_f'`). It is now the Quark error above.
- **Two functions that each declared a fully annotated helper of the same name failed in clang** (`redefinition of 'quark_helper'`). Each nested declaration now gets its own C++ name.
- **A module function that read a top-level variable failed in clang.** `counter = 7`, then `module cnt:` with `fn get() -> counter`, then `print(c.get())` after `incl cnt as c`, stopped with `use of undeclared identifier 'quark_counter'`. It now prints `7`, and sees later assignments to `counter`.
- **A parameter, loop variable, lambda parameter or `when` binding named like a builtin now shadows it.** (A nested `fn` named like a builtin is an error since the fixed-declaration change above.) Calls used to go to the builtin:
  - `fn f(len) -> len('abc')` called the builtin `len`.
  - `fn g(len) -> 'x' | len()` did the same through a pipe.
  - `fn adder(max) -> (fn(x) -> x + max)` failed in clang (`use of undeclared identifier 'quark_max'`).

  They now use the local binding. `fn clamp(x, min, max)` still works.
- **A captured parameter named like a fully annotated (native) function was called as that function.** With `fn f(x: int) int -> x + 1` defined, `fn g(f) -> (fn(z) -> f(z))` called the top-level `f`, so `g(fn(y) -> y * 100)(10)` printed `11` instead of `1000`.
- **An int stored in a `float` struct field read back as a garbage float.** `P { x: 1 }` with `x: float` stored the int unconverted, and the field read took the float bits: `print(p.x)` printed `5e-324`. Ints are now widened to float in struct literals and in field defaults (`weight: float = 2`), as for parameters.
- **A value of unknown type stored in a typed struct field wasn't checked**, so reading it reinterpreted the wrong type (`p.x + 1` on a string printed a large integer). It is now checked when the struct is built (see Breaking).
- Reading a field of a `null` struct value says so: `cannot read a field of null (expected a struct value)` (was `expected struct, got different type`).
- A function value called with too few or too many arguments, from Quark or through `qext::call`, used to read missing arguments silently.
- `unboxToNative` read the union fields of a native function body's result without checking the tag, which gave garbage values. The read is now checked.
- Fully annotated (native) functions can be called from closures and nested functions.
- `QClosure*` native arguments and returns are boxed with `qv_closure_ptr`. They used to be wrapped in a second closure.

### Internal
- Renames: `token.USE`→`token.INCL`, `token.VECTOR`→`token.VEC`, `ast.UseNode`→`ast.InclNode` (`quark parse` prints `Incl`), `parseUse`→`parseIncl`. The catalog key `vector_any` is now `vec_any`.
- Runtime: `q_println` removed; `q_to_vector`→`q_to_vec`, `q_fmt_vector`→`q_fmt_vec`. C++ type and tag names (`QVector`, `VAL_VECTOR`, `qv_vector*`, `qext::as_vector`) are unchanged.
- Editor extension: `incl`/`vec` highlighting and completion, snippets `incl`, `inclf` and `vec`; `data/quark.json` regenerated.
- `TestNeg_RenamedSymbolsRejected` checks that the old spellings are rejected.
- Struct field checks: codegen `checkedFieldValue` wraps each field value whose type isn't proven in a new `q_field_int/float/bool/check/struct` helper (`builtins/params.hpp`); proven values are stored unchanged.
- Parser: the struct-name pre-pass (`collectStructNames`, `Parser.structNames`) is removed. `Name {` always parses as a struct literal and any name before `->` as a return type; the analyzer checks the name.
- `q_member_get` (`ops/member.hpp`) reports a missing dict key instead of returning `null`.
- New `smoke_struct_fields.qrk`, `TestStructsAcrossFiles`, and 12 `TestSemanticsSafety_Rejected` cases for struct fields and dict keys.
- Loader `checkTopLevelClashes` (`loader/loader.go`) runs on the fully spliced tree, before module namespacing. It groups top-level definitions by name and source file (`ast.TreeNode.SourceFile`) and reports clashes as `QK-LOAD-001`. The location is in the main file when one side is there, otherwise in the included file.
- Analyzer `refersToBuiltin` / `refersToNative` compare the symbol a name resolves to with the one the builtin, extern fn or native function was declared with (`Analyzer.builtinSyms`, `nativeSyms`). The direct-call, pipe and capture paths use the builtin or native dispatch only when the name isn't shadowed.
- `q_input` returns `qv_ok(str)` / `qv_err(str)`. The catalog return type is `result`, and `inferBuiltinReturnType` types it `result[str, str]`, so `unwrap(input())` is a `str`. `CanAssign(result, null)` is false, and codegen's `paramTagChecks` no longer allows null for `result`. `validateDefaultValues` checks a `null` default as `null`.
- New `smoke_shadowing.qrk`, `TestTopLevelNameClashes`, and 23 `TestSemanticsSafety_Rejected` cases (builtin and extern redefinition, `input()` misuse, null results). `TestInput_EOFAndLongLines` uses the new read loop. Editor `data/quark.json` regenerated (`input` signature).
- `QFnInfo {name, min_arity, max_arity, default_arg}` (`types/closure.hpp`). Codegen `emitFnInfos` emits `_qfi_<name>`, and `emitDefaultArgFns` emits `quark_<name>__default_arg`.
- `types/function.hpp` rewritten:
  - `q_call0`…`q_call12` go through `q_call_values`, which calls directly when the argument count equals the parameter count and uses `q_call_adapt` otherwise.
  - New limit `Q_MAX_CALL_ARGS = 12`.
- `types/analyzer_callshapes.go`: `FunctionType.ParamCalls`, `CallShape`, `recordParamCall`, `checkCallbackArgs`, `markParamReassigned`.
- `types.TypeFn`, `IsCallableType`, `Analysis.ModuleMemberRefs`.
- `CLAUDE.md` gains the *Language Goals*, *Catch It at Compile Time* and *Robustness Bar* sections.
- New `smoke_function_values.qrk`, about 24 new `TestSemanticsSafety_Rejected` cases, and `TestNativeCallback_Arity`.
- **Fixed `fn` declarations** (`types/analyzer_fixed.go`):
  - The parser still produces `name = fn(...)` for `fn name(...)`, with the new `ast.TreeNode.FnDecl` set.
  - `declareFixedFns` declares a scope's declarations as non-mutable symbols before its statements (replacing `predeclareFunctions` for them; plain `g = fn` assignments are no longer predeclared) and reports rules 18 and 20; rule 19 is checked in `analyzeOperator` and `analyzeVarDecl`. `checkCallsBeforeDefinition` is removed.
  - Call plans to declarations are recorded with `deferFixedDispatch` and resolved by `finalizeFixedFns` once the program is analyzed. It computes which declarations need a closure (least fixpoint over captured symbols, now recorded per lambda in `captureSyms`), assigns C++ names (`FixedFn.CName`: the name for a non-capturing top-level or module declaration, `name__N` otherwise), registers native functions (`nativeFns` is now keyed by `CName`), drops captures of declarations that need no closure, and sets each plan to `DispatchNative`, `DispatchDirect` or `DispatchClosure`. `refersToNative` and `nativeSyms` are removed.
  - `checkDefiniteAssignment` implements rule 23 per scope and records `HoistScope`s: the cells codegen creates unassigned and the closures it creates at the scope's start. The top level and module bodies are one hoisting unit, since they all run in `main`.
  - New `Analysis` fields: `FixedFns`, `FnRefs`, `HoistScopes`, `CheckedCaptures`.
  - Codegen: `hoistScopes` (replaces `predeclareMainFunctionBindings`) runs at the start of `main`, of every block and of the native-function fast path; `fixedFnValue`/`directFnValue` for declarations used as values; `generateIdentifier` reads `FnRefs` and uses `q_cell_get` for checked captures. The first assignment of a captured variable declares its cell before building the value, so `fib = fn(n) -> fib(n - 1)` captures it.
  - Runtime (`core/cell.hpp`): `q_new_unassigned_cell` and `q_cell_get`. `qv_null()` now zeroes its payload so no real null matches the unassigned marker.
  - Tests: new `smoke_fn_declarations.qrk`; 28 `TestSemanticsSafety_Rejected` cases replace the two `call_before_definition` ones; `smoke_shadowing.qrk` drops its nested `fn len`; four codegen tests now expect direct calls.

---

## 2026-11-11: Value printing, float formatting, `**` precedence, parameter enforcement

Commit: `c33d362`

### Breaking
- **`**` binds tighter than unary minus and `!`.** `-2 ** 2` is now `-4` (it was `4`), and `!x ** 2` is `!(x ** 2)`. `2 ** -1` now parses. Existing code changes result with no warning.
- **Float output** uses the shortest round-trip form, like Python, instead of `%g`.
  - Whole floats keep `.0`: `sqrt(16)` → `4.0`, `10 / 2` → `5.0`, `sum(vector [1,2,3,4])` → `10.0`.
  - Other examples: `0.30000000000000004`, `1e+16`, `1.5e-05`, `inf`, `nan`, `-0.0`.
  - This applies to `println`, `to_str`, `std/fmt`, and error messages.
- **Container output.** `println`/`to_str` print contents instead of summaries such as `[list len=N]`, `[dict len=N]` or `Name{...}`. Tables print `[table<T> rows=N]`.
- **Parameter annotations are enforced at runtime** when the argument type isn't known at compile time, for example `fn f(x: int)` called with an `unwrap(...)` that holds a string. Such calls used to be accepted.

### Added
- **Printing by contents** (`builtins/repr.hpp`, `q_repr`):
  - Lists and vectors: `[1, 'a', null]`. Strings inside collections are quoted and escaped.
  - Dicts: `{age: 36, 'first name': 'A', name: 'Ada'}`, keys sorted and bare when they look like names.
  - Structs: `Point {x: 1, y: 2}`. Results: `ok 5` / `err 'boom'`.
  - A collection with more than 100 items shows the first and last 5 items plus a count: `[0, 1, 2, 3, 4, ..., 995, 996, 997, 998, 999] (1000 items)`.
  - Self-references print as `[...]` / `{...}`.
- `fmt.full(x)` in `std/fmt`: the `println` rendering with nothing elided.
- **Runtime parameter checks** (`builtins/params.hpp`):
  - They apply to every annotated parameter, including in partly annotated functions and in calls through function values, pipes and defaults.
  - An int passed to a `float` parameter widens: `fn scale(x: float, k) -> x * k`, then `scale(1, 2)` → `2.0`.
  - Reference annotations (`list`, `dict`, `vector`, `result`, `resource`, struct names) accept `null`.
  - The error reads `argument 'x' of 'f' expects int, got str`.

### Fixed
- A fully annotated function that captures an outer variable is no longer compiled as `DispatchNative`. That path called it without its closure.

### Docs
- New draft `stdlib_expansion_v0_1_spec.md`, not implemented: multi-argument `println`, `fmt.format`, `std/fs`, and sys/time/io/csv additions.
- Float exponent literals documented. README gains a *Known limitations* section.

### Internal
- `core/format.hpp` (`q_format_float`). Codegen `emitParamBindings` uses `q_param_int/float/bool/check/struct` and replaces two duplicated binding loops.

---

## 2026-11-11: Core cleanup; file I/O moved to `std/io`

Commit: `a5afa58`

### Breaking: removed builtins
| Removed | Use instead |
|---|---|
| `_file_open`, `_file_read`, `_file_write`, `_file_close`, `_file_seek`, `_file_exists` | `use 'std/io' as io`, then `io.open/read/write/close/seek/exists` |
| `enumerate(xs)` | `xs.enumerate()` |
| `vfrom_list(xs)` | `xs.to_vector()` |
| `str.concat(s)` | `'a' + 'b'` (`list.concat` is kept) |
| `file_handle` annotation type | `resource` |

### Breaking: other
- For an open file, `type(f)` returns `'resource'` (was `'file_handle'`), and `println(f)` shows `<resource file>`.
- `std/io` error messages changed, for example `io.read() expects an open file`.

### Added
- `vector.enumerate()`.
- GC regression tests (`integration_gc_test.go`, `src/testfiles/gc/`). They compare the GC heap and peak RSS across repeated rounds.

### Changed
- `std/io` is implemented with `extern fn` bindings to `src/core/stdlib/io.hpp` (moved from `runtime/.../builtins/fileio.hpp`). Its parameters are typed `resource`.

### Fixed
- **GC: an open file could be collected while still in use.** The resource registry was stored in `malloc`'d memory, which the GC doesn't scan. It now uses `q_allocator`.
- **GC: dict keys longer than 15 bytes leaked memory** (about 6 MiB over 8 rounds in the test). `QDictKey` now uses `q_allocator`.

### Removed
- `native_modules_design.md`. `ffi_v0_1_spec.md` is now the canonical spec for native interop.

---

## 2026-11-10: Module namespacing, dynamic methods, sorting, new stdlib modules

Commits: `5bc40d9`, `1a96b6d` (`1a96b6d` holds files `5bc40d9` needs to build, so treat them as one change)

### Breaking
- `input()` returns `null` at end of input instead of `''`. It also strips `\r\n` and reads lines of any length; before, long lines came back in roughly 4 KiB pieces.
- **Module namespacing.** Module members are renamed to `module__member`, so aliased modules can share names (`io.read` and `csv.read`), and user code can no longer shadow a module member. With an un-aliased `use`, clashing names are now `QK-LOAD-001` errors:
  - `symbol 'x' from module 'b' conflicts with symbol 'x' from module 'a'; import one of them with an alias`
  - `symbol 'x' from module 'a' (imported on line N) conflicts with this definition; ...`

  Before, whichever module was spliced last silently won.
- New compile errors for programs that used to be accepted:
  - Calling a function above its definition in the same block: `'later' is called before its definition on line N`.
  - `when` patterns other than literals, `_` or `ok`/`err` bindings: `when patterns must be literal values ...`.
  - Combining `ok`/`err` patterns with `or`.
  - `\0` in string literals. It used to cut the string short silently.

### Added
- **New stdlib modules**, each bound with `extern fn`:
  - `std/sys`: `args()`, `env(name)` (returns `null` if unset), `exit(code = 0)`.
  - `std/csv`: `parse(text, sep = ',')` and `read(path, sep = ',') result`. Follows RFC 4180 (quotes, `""` escapes, embedded newlines, CRLF). Every field is a `str`.
  - `std/time`: `now()`, `clock()` (monotonic), `sleep(seconds)`.
  - `std/random`: `seed`, `uniform`, `randint(lo, hi)` (both ends included), `choice`, `shuffle`. Uses mt19937_64, so results are reproducible after `seed`.
- `list.sort()` and `list.sort_by(key)`: in place, stable, ascending. Keys must be all numbers, all strings or all bools; mixed keys or `null` are a runtime error.
- String ordering: `<`, `<=`, `>`, `>=` on two strings compare by bytes.
- **Dynamic method dispatch** on values whose type is unknown at compile time (for example `unwrap(io.read(f, n)).split('\n')`):
  - The call switches on the runtime type tag. A receiver of the wrong type is an error: `type 'int' has no method 'upper'`.
  - At compile time, `no type has a method 'frobnicate'` is an error, and arity and argument types are still checked where possible.
- Float exponent literals: `1e3`, `2.5E-4`, `6e+2`.
- `quark run file.qrk -- arg1 arg2` passes arguments to the program.
- `io.open` mode `'a'` (append).

### Changed
- **Runtime error locations.**
  - After a call returns, an error is reported at the caller's statement.
  - An error inside an imported user module names that module's file.
  - An error inside `std/` points at the user's call.
  - Struct and table runtime errors now use the standard `error[QK-RUNTIME-001] ... at file:line:col` format.
- If generated C++ fails to build, `quark build` and `quark run` print a hint instead of dumping the C++. Rerun with `--debug` to keep it.

### Fixed
- Lowered `for i in range(...)` loops read their arguments without a type check (a float or string gave garbage), and a step of `0` looped forever or was skipped. They now behave like `range()`.
- Module functions can be called from inside user functions and closures, directly or through pipes.

### Removed
- `ops/logical.hpp` (unused) and `runtime/CMakeLists.txt` (a Catch2 setup for runtime tests that don't exist).

### Internal
- `loader/namespace.go`, `ir.DispatchDynamicMethod` with `MethodCandidates`, invariant `INV-DYNAMIC-METHOD`, `QSourceLocGuard` in every generated function, `builtins/sort.hpp`, `core/process.hpp`.

---

## 2026-11-06: Vendored Boehm GC, VS Code extension 0.1.0

Commits: `3d817ad`, `798d76d`, `6a7c994` (the prebuilt GC from `3d817ad` only works with the toolchain change in `798d76d`)

### Changed
- **Prerequisites.** Prebuilt static Boehm GC libraries are committed for linux-amd64, linux-arm64, darwin-arm64 and windows-amd64, so CMake is only needed on other platforms. This was also the first time anything under `deps/` was tracked; before, a fresh clone could not build.
- `toolchain.EnsureGC` looks for a GC library in this order: `deps/bdwgc/lib/<os>-<arch>/`, then a local `build-static/`, then a CMake build (only when the GC source is present).

### Added
- **VS Code / Positron extension 0.1.0.** See `src/devtools/vscode-extension/quark/CHANGELOG.md`. Highlights: a grammar for the current language, snippets checked against the parser, completion and hover from generated compiler data, and Python/R habits highlighted as errors.
- `go run ./cmd/quark-vsix` builds the `.vsix` without Node or npm.
- `internal/editordata` generates `data/quark.json` from the catalog, lexer keywords and stdlib, and fails when the extension falls out of sync.
- The FFI test C sources (`src/testfiles/ffi/*.c`). They were missing from the previous commit because `.gitignore` excluded `*.c`.

---

## 2026-11-04: FFI phase 1 and `std/blas`

Commits: `195a8d2`, `306b764`

### Breaking
- Text after `extern '<header>.hpp'` on the same line is a parse error.
- `quark parse` output names the header node `ExternHeader` (was `ExternSource`).

### Added
- **Native library directives**, allowed only at the top level of a file:
  ```
  extern include_path 'include'   // -I, relative to this file
  extern link 'openblas'          // -lopenblas
  ```
  - They apply to the whole program, including imported modules, with duplicates removed.
  - Link order follows the import graph.
- CLI flags `--lib-path`, `--include-path` and `--link-arg` (all repeatable), and the environment variables `QUARK_LIB_PATH` and `QUARK_INCLUDE_PATH`.
- Libraries are resolved before linking. `--debug` prints where each one was found.
- **`std/blas`** (OpenBLAS/CBLAS over f64 vectors): `dot`, `axpy`, `scal`, `nrm2`, `asum`, `gemv`. Inputs are never modified. Requires OpenBLAS.
- New error codes:
  - `QK-FFI-001`: bad `include_path`.
  - `QK-FFI-002`: native library not found. Reported at the `.qrk` line that requested it, with notes on the paths searched.
  - `QK-FFI-003`: C++ error in a glue header, reported at the header.
- Diagnostics can carry `note:` lines.

### Fixed
- An `int` argument to a `float` parameter (Quark or `extern fn`) passed `quark check` but aborted at runtime. It now widens.
- A scalar-typed (`int`/`float`/`bool`) assignment used as the value of an `if` branch generated C++ that failed to compile (`306b764`).

---

## 2026-10-07: Compilation pipeline rewrite

Commit: `c349da8`

### Breaking
- Unknown CLI flags and extra positional arguments are errors. They used to be ignored.

### Added
- **Precompiled runtime header**, cached under `$XDG_CACHE_HOME/quark/pch/` and on by default. It roughly halves compile time for small programs. Disable it with `--no-pch`.
- `quark build --debug` keeps `<output>.cpp` and prints the clang commands.
- `--lto` uses `lld` when `ld.lld` is on PATH.

### Changed
- Flags can come before or after the file, and `-flag` and `--flag` both work.
- Each build uses its own temporary directory, so concurrent runs no longer collide on `$TMP/quark_temp.cpp`.
- Compilation and linking are separate clang steps.

### Internal
- New `driver/` package (`CompileFile`/`CompileSource` by stage, returning a `Unit`) and `toolchain/` package (clang/GC discovery, compile, link, PCH). `main.go` shrank from about 860 lines to about 380.
- `types.Analysis` with a single `Generator.SetAnalysis` replaces seven `codegen.Set*` setters.
- Deleted the dead `runtime/.../builtins/fmt.hpp`.
- `ffi_v0_1_spec.md` rewritten, design only: `extern` directives instead of `ffi lib`.

---

## 2026-10-06: Arrow-compatible vectors

Commit: `f43d0f5`

### Breaking
- **`v.fillna(x)` returns a new vector and leaves `v` unchanged.** Vectors are now immutable. Code that called `v.fillna(0)` and discarded the result now silently keeps its nulls.
- Native/FFI authors:
  - Removed from `qext`: `as_bool_vec`, `as_bool_vec_mut`, `null_mask`. The old null mask used 0 = valid.
  - Added: `bool_at`, `set_bool`, `str_at`, `set_null`, `validity_bitmap` (Arrow polarity, set bit = valid), `vec_null_count`.
  - Glue that touched `QVector::storage` or `nulls` no longer compiles.

### Changed
- `QVector` uses the Apache Arrow columnar layout:
  - Validity is a bitmap.
  - Bools are bit-packed.
  - Strings use `int32` offsets plus UTF-8 data.
  - Buffers are 64-byte aligned.
- Comparing `vector[i64]` with floats or with a `vector[f64]` promotes to float. It used to be "operator not supported".
- Comparing vectors of different lengths fails with `vector size mismatch in comparison: N vs M`.
- A `vector[str]` holds at most 2 GiB of string data.

### Fixed
- `astype('i64')` on NaN or an out-of-range float is a runtime error. It used to be undefined behavior.
- `vector[i64]` arithmetic skips null slots, so a garbage value in a null slot can't raise a false overflow.

---

## 2026-10-06: Structural equality, overflow checks, stricter conditions, `all`/`any`

Commits: `26d21f9`, `f76348a` (merged through PR #1, `d899f2c`)

### Breaking
- **Equality.** `==`/`!=` on lists, dicts and results compare by value. Before, they were always `false`, even for `a == a`. Function values compare by identity.
- **Integer overflow is fatal.** Int `+ - *`, unary `-` and `**` stop with `integer overflow in '<op>'` instead of wrapping (which was undefined behavior in C++). This covers boxed values, scalar-lowered code and `vector[i64]`.
  - `2 ** 63` used to fall back to a float; it is now an error.
- **Results and vectors can't be used as conditions** in `if`, `while`, the ternary, `!`, `and` or `or`. The compile error suggests `is_ok(r)`/`when`, or `all(v)`/`any(v)`/`len(v) > 0`. Dynamic values fail at runtime.
- Assigning a non-exhaustive `when` (no `_` arm) to an annotated variable is a compile error.

### Added
- `all(v)` and `any(v)` for `vector[bool]`, skipping nulls.
- `when` as an expression on the right of an assignment:
  ```
  size = when n:
      0 -> 'none'
      _ -> 'many'
  ```

### Changed
- The Boehm GC is always built and linked statically, so the binaries Quark produces don't need libgc at runtime.

### Fixed
- `fmt.table` accepts a dict of columns again. This broke on 2026-04-03.
- Scalar-lowered `%` by zero and float `/` by zero are fatal, matching the boxed operators.
- Closure capture analysis no longer captures a `for` variable or an `ok v`/`err e` binding that shadows an outer variable.

### Docs
- README rewritten: quickstart, language tour, stdlib and CLI reference, roadmap.

---

## 2026-04-03: Table runtime scaffolding; struct fixes

Commits: `fd14038`, `df813dc`, `7b150a1`

### Added
- `tables_v0_1_spec.md`, a draft design for a struct-shaped columnar `table` type.
- Runtime support for tables (`types/table.hpp`, `VAL_TABLE`). **`table { ... }` literals don't parse**, so user code can't create a table yet.
- An empty `dict {}` is allowed as a default parameter value.

### Changed
- `fmt.show_dict` lists keys in sorted order.
- A parameter or return annotated with a struct name gets the struct type. It used to fall back to `any`.
- `null` can be assigned to struct- and vector-typed variables.

### Fixed
- Struct field types can refer to structs declared later, to each other, and to themselves.
- The type check on a struct field's default value could check the wrong field.

---

## 2026-04-02: Structs v0.1

Commits: `7fd7670`, `0474262`, `22c8a71`, `97a43ee`

### Breaking
- `struct` is a reserved keyword.

### Added
- Immutable, nominal struct types:
  ```
  struct Customer:
      id: int
      name: str
      age: int = 0            // constant defaults only
  c = Customer { id: 1, name: 'Alice' }
  fn get_name(c: Customer) str -> c.name
  ```
  - Field access is checked at compile time.
  - `type(c)` returns `'Customer'`.
  - Structs are always truthy.
- Compile errors:
  - unknown or duplicate fields;
  - missing required fields;
  - field type mismatches;
  - assignment to a field (structs are immutable);
  - `==`/`!=` on structs (compare fields explicitly).
- `structs_v0_1_spec.md`.

---

## 2026-03-28: Vector soundness, `std/fmt` on `extern`, build output

Commits: `09ac4c0`, `882108e`, `7b3f8e9`, `ab9a14d`, `eb13d87`, `a7120e1`, `1381b04`

### Breaking
- Removed the prelude builtins `_fmt_list`, `_fmt_vector`, `_fmt_dict` and `_fmt_table`. `std/fmt` now binds them as `extern fn`.
- `fmt.show_vec` requires a `vector`.

### Added
- Bool vectors: `vector [true, false]`, and `list [true, null].to_vector()`.

### Changed
- `sum`, `min` and `max` on vectors skip nulls. Each returns `null` when every value is null.
- `quark build` and `quark run` no longer print clang's loop-vectorizer remarks.
- Generated C++ no longer triggers `-Wunused-value` warnings.

### Fixed
- **Mixed-dtype vector arithmetic silently returned `null`** (for example i64 vector + f64 vector). Mixed numeric operands now promote to f64, non-numeric operands are a runtime error, and nulls propagate element by element.
- **`extern fn` arguments from untyped values were reinterpreted without a check.** For example, `extern fn add1(x: int)` given a string read garbage. `q_as_*` now fails with `expected int, got str`.
- A `vector` annotation was treated as `vector[float]`, so `for x in v` over an int vector read garbage values.
- `v.astype('i64')` now gets its result type from the dtype string, not from the receiver.
- Piping into an `extern fn` (`4 | add1()`) uses extern dispatch and checks argument types.
- A variable annotated `any` and initialized with an int can be reassigned to another type.
