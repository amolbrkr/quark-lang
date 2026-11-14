# Quark Architecture & Implementation

How the compiler and runtime work: the pipeline, the runtime value system, code generation, memory management, the soundness contract between the analyzer and the back end, and the back-end roadmap. Language behaviour is in `semantics.md`, syntax in `grammar.md`, builtins in `stdlib.md`.

## Contents

1. [Compilation pipeline](#1-compilation-pipeline)
   - [1.1 Lexer](#11-lexer--token-stream) · [1.2 Parser](#12-parser--ast) · [1.3 Loader](#13-loader--one-tree-and-a-name-table) · [1.4 Analyzer](#14-analyzer--typesanalysis) · [1.5 Invariants](#15-invariants) · [1.6 Lower, verify, emit](#16-lower--ir--verify--emit) · [1.7 C++ compiler](#17-c-compiler--binary)
2. [Runtime value system](#2-runtime-value-system)
3. [Memory management](#3-memory-management)
4. [Naming conventions](#4-naming-conventions)
5. [CallPlan IR](#5-callplan-ir)
6. [Builtin catalog](#6-builtin-catalog)
7. [Extern function system](#7-extern-function-system)
8. [Resource system](#8-resource-system)
9. [Lowering details](#9-lowering-details)
10. [GC bootstrap](#10-gc-bootstrap)
11. [Testing](#11-testing)
12. [Soundness contract](#12-soundness-contract)
13. [Roadmap: back end](#13-roadmap-back-end)
14. [Structural debt](#14-structural-debt)
15. [Performance findings](#15-performance-findings)

---

## 1) Compilation Pipeline

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Lower → IR → Verify → Emit → C++ → clang++ → binary
```

The compiler (lexer through emit) runs in `driver/`: `driver.CompileFile(path, stage)` runs every stage up to a cutoff (`StageParse`, `StageCheck`, `StageEmit`) and returns a `Unit` with the AST, the analyzer's `types.Analysis`, the C++ and all diagnostics. The `parse`, `check`, `emit`, `build` and `run` commands go through it; `quark lex` calls the lexer directly. `internal/testutil` runs in-memory test sources through `driver.CompileSource` (only its `Lex` and `Parse` helpers stop before the loader), so tests see the same stage order, early stops and diagnostics as the CLI. The native back end (clang++, runtime headers, GC) lives in `toolchain/`.

### 1.1 Lexer → Token Stream

Three passes:
1. **Raw tokenization**, driven by operator tables. Single- and double-quoted strings with escapes (`\n`, `\t`, `\r`, `\\`, `\'`, `\"`; `\0` is an error because strings are NUL-terminated). Line comments with `//`. `::` is one token (`COLONCOLON`).
2. **Line-start tracking**: marks which tokens begin a new line.
3. **Indentation injection**: converts leading whitespace after `:` or `->` + newline into `INDENT`/`DEDENT` tokens, with an indent stack; a mismatched dedent is an error.

Inside brackets (`()`, `[]`, `{}`) indentation processing is suppressed: newlines are ignored and no `INDENT`/`DEDENT` is emitted.

### 1.2 Parser → AST

Recursive descent producing `ast.TreeNode`s, with Pratt parsing for expressions (12 precedence levels, `grammar.md` §10). Each construct has its own node kind: `AssignNode` (target: identifier, `MemberNode` or `IndexNode`), `MemberNode` (`object.name`), `UnaryNode`, `BinaryNode`, `PairNode` (a dict literal entry), `QualifiedNameNode` (`a::b::c`), and `FnDeclNode` (`fn name(...)`: `[name, LambdaNode]`).

Key decisions:
- `fn name(...)` at statement level → `FnDeclNode`, a fixed, hoisted declaration (`ast.IsFnDecl`); `fn(...)` in an expression → lambda.
- `x | f(a)` → the call `f(x, a)` marked `Piped` (`x | obj.m(a)` → `obj.m(x, a)`), built in `parsePipe`. No later stage has a pipe path; `Piped` only adds "(including piped input)" to arity messages. A pipe target that isn't a call is a parse error.
- `list [...]` → list literal (the keyword avoids ambiguity with indexing).
- Dict keys in literals are bare identifiers, interpreted as string keys.
- `Name {` always parses as a struct literal and any name before `->` as a return type; the analyzer checks the name.

### 1.3 Loader → One Tree and a Name Table

Names are resolved per file (`specs/implemented/modules_v0_2_spec.md`, `semantics.md` §10).

- `loader/files.go` loads each `incl`uded file once (identity: the real path, symbolic links resolved; numbered in load order), checks file names and that imported files contain only definitions and constants, and builds each file's scope (top-level definitions, modules with their members, import bindings), reporting clashes as `QK-LOAD-001`.
- `loader/resolve.go` then walks every file. Each definition of imported file N (stem `s`) gets the internal name `q<N>_s__name` (module members `q<N>_s__module__name`); main-script module members become `module__name`; main-script top-level names are unchanged. Every reference, bare (local, then enclosing module, then file) or `::`-qualified, is rewritten to its internal name. Two definitions that would get the same internal name are a load error. A local of an imported file named like a main-script name becomes `q<N>__name__l`, since the spliced tree has one global scope. Every extern method of file N (main: 0) becomes `q<N>__name__m`, and so do the method calls of that file with its name, so only that file reaches it (a method call can't be qualified, and the loader doesn't know a call's receiver type; an extern method therefore can't be named like any prelude method). Constants of imported files are inlined at each use. User names can't contain `__` (`checkReservedNames`), so no user name can reach an internal one.
- The files are spliced into one tree, imported files first, without their `incl` statements. The resulting `names.Table` maps internal names back to what users write (`Display`, `DisplayText`); analyzer diagnostics and the runtime names the back end emits go through it.
- Nodes from imported files are tagged with `SourceFile`, which the back end's `q_set_source_loc` calls use so runtime errors name the module's file (code from `std/` sets no location, so its errors point at the user's call). Every generated function starts with a `QSourceLocGuard` that restores the caller's location when it returns.
- `extern link` and `extern include_path` directives of every loaded file are collected into `driver.Unit.Native` (§1.7).

### 1.4 Analyzer → `types.Analysis`

`analyzeCompilationUnit` (`types/analyzer_functions.go`) runs in this order:

1. **Structs**: `predeclareStructTypes` declares the structs of every file and module body, so any struct can be used before its definition.
2. **Fixed `fn` declarations**: `declareFixedFns` declares the top level's and every module body's `fn`s as non-reassignable symbols before any statement is analyzed (module bodies share the file scope, so they are hoisted together). Every block does the same for its own declarations on entry.
3. **Statements**, in order: scopes, types, closure captures, call plans, call shapes (§1.4.3), element types (§12).
4. `finalizeFixedFns` (`types/analyzer_fixed.go`): decides which declarations need a closure (a least fixpoint over captured symbols: a declaration that captures nothing, directly or through another declaration, is a plain C++ function), assigns their C++ names, registers fully annotated non-capturing ones as native functions, and sets each deferred call plan to `DispatchNative`, `DispatchDirect` or `DispatchClosure`. It runs `checkDefiniteAssignment` (modules rule 23: a call that would run a declaration before a variable it uses is assigned) and records the `HoistScope`s lowering needs.
5. `finalizeSymbols`: decides each symbol's storage and C++ name (§1.6).

The analyzer returns one `types.Analysis` (`types/analyzer_modules.go`), passed to the invariant checks and to the back end (`lower.Lower`). New analyzer outputs go in this struct, not new setters.

| Field | Contents |
|---|---|
| `CallPlans` | Per call site: dispatch mode, arity, defaults to inject (§5) |
| `Refs` | The `types.Symbol` of every resolved identifier node: reads, assignment targets, typed declarations, parameters, loop variables, `when` bindings and `fn` names. A symbol has a `Kind`, a `StoreType` (the type every value it holds is known to have: its annotation, a literal default's type, or its first value's type), a `Decl` node (where its C++ variable is declared), and after `finalizeSymbols` a `Storage` and `CName` |
| `Captures` | Per lambda, the captured symbols in capture-slot order |
| `NodeTypes` | The inferred type of every expression node; lowering uses it for operator lowering and to decide when a stored value needs a conversion or a check |
| `ReturnValidation` | Per function, whether the declared return type was validated |
| `NativeFns` | Prototype call plans for fully annotated, non-capturing functions (`DispatchNative`) |
| `ExternFns` | Prototype call plans for `extern fn` (`DispatchExtern`), keyed by name or `"type.method"` |
| `StructTypes` | Declared structs and their fields |
| `Names` | The loader's name table |
| `FixedFns`, `FnRefs`, `HoistScopes` | Fixed declarations (by lambda node), identifier nodes that name a declaration needing no closure, and per scope the cells and closures created at its start |
| `TableReads` | The calls of `csv::read_file` / `csv::parse_text`, with the native reader and the row struct the expected type names (`types/analyzer_tables.go`) |
| `Queries`, `ColumnRefs` | Every table query (`types.Query`: per step the verb, its input and output rows, the mutate columns, sort/group keys, aggregates), and every `e.col` of a verb expression with its column index (`types/analyzer_query.go`) |

Diagnostics are severity-tagged: errors (`QK-CHECK-001`) stop `check`/`emit`/`build`/`run`; warnings (`QK-CHECK-002`) are reported and don't. Every message goes through `names.DisplayText`.

#### 1.4.1 Knowability rule

When either side of a check (parameter and argument, slot and value) has an unknown or `any` type, the check is emitted for runtime. Only when both types are known does the analyzer decide it at compile time.

#### 1.4.2 Assignability

`CanAssign(dst, src)` (`types/types.go`) is the rule for a value stored into a slot (variable, parameter, element, field, return value); `IsSubtype` is the same without conversion, used below the top level:

- `any` accepts everything; a value of type `any` is a subtype of nothing else (it is checked where it is stored, or rejected for a typed slot it can't be checked into).
- `int` is assignable to `float` (widened where it is stored), but `list[int]` is not a subtype of `list[float]`.
- `null` is a value of struct and `resource` types only, not `list`, `dict`, `vec`, `fn` or `result`.
- Containers are covariant in element type (`list[int]` is a `list[any]`) and never the other way: a list's static element type is a guarantee about every value in it (§12).
- A union destination accepts a value of any of its options; a union source must fit every option.
- A function value fits a signature with the same arity whose return type it returns; a value typed only `fn` fits no specific signature.
- A table: a bare `tbl` holds any table. A `tbl[R]` slot accepts a `tbl[R]`, and, as a conversion (so `CanAssign` only), a table with an anonymous row of `R`'s shape (`RowMatch`: the same column names in any order, each of the field's type or an int for a float field) or with a row the compiler doesn't know (checked at runtime). Lowering stores such a value through the slot's check, which relabels it (§12).
- Everything else requires an exact match.

**Expected types** (`Analyzer.expected`, `types/analyzer_tables.go`). Before analyzing an expression whose value goes into a typed slot, the analyzer records the slot's type for it: typed declarations, assignments to typed variables, arguments of a callee whose signature is known by name, struct literal fields, and the last statement of a body with a return annotation; `unwrap` passes `result[T]` on to its argument. Only the std/csv reads consume it so far (D18): they are recognized by their definition (a `names.Origin` in `std/csv.qrk`), and with an expected `result[tbl[R], ...]` they are typed `result[tbl[R], str]` and lowered to the native reader with `R`'s `QStructDef`.

**Pending containers.** An empty `list []` / `dict {}` has a pending `ListType`/`DictType` (`NewPendingList`) that reads as `list[any]` until its element type is decided: by the container type it is stored into or merged with (`SettleContainers`, `MergeTypes`), or by its first write (`checkElemWrite` → `FixElementType`). Pending literals stored into each other become peers and are fixed together. One passed where writes can't be seen (an untyped parameter, an `any` slot) is fixed to `any` (`escapePending`). Every alias shares the one type object, so the fix applies to all of them and to the literal's runtime descriptor.

#### 1.4.3 Call shapes

`types/analyzer_callshapes.go` records how a body calls each parameter (`FunctionType.ParamCalls`: arity and argument types, propagated through parameters passed on to other functions) and checks function arguments with known signatures against them (`checkCallbackArgs`): wrong arity, wrong argument types, or a non-function where the body calls the parameter. Every call path in the analyzer calls `recordParamCall` and `checkCallbackArgs`. A parameter the body reassigns isn't tracked (`markParamReassigned`).

### 1.5 Invariants

`invariants/` validates the call plans (per dispatch mode, against the builtin catalog) and return annotations before lowering. A breach is a compiler bug (`INV-*`, `error_codes.md`) and fails loudly.

### 1.6 Lower → IR → Verify → Emit

The back end is two packages. `lower/` turns the analyzed AST into a small typed IR (`lower.Lower`) and checks it (`lower.Verify`); `emit/` prints it as one C++17 translation unit using the runtime headers (`emit.Program`). Every decision is made while lowering; the emitter only prints.

**The IR** (`lower/ir.go`). Every expression has a representation (`lower.Rep`): `QValue`, a raw `long long`/`double`/`bool`, or the native type of an extern or native signature (`const char*`, `QList*`, ...). Conversions are explicit nodes: `Box`, `Unbox` (a raw read of the union field, with the reason the tag is known, §12), `Cast` (int ↔ double), `Adapt` (the checked `q_as_*` conversions) and `Check` (the `q_param_*`, `q_var_*`, `q_field_*` and `q_ret_*` checks). Statements are `Let` (a temporary), `DeclVar` and `SetVar` (a symbol's C++ variable), `SetTemp`, `SetField`, `Do`, `If`, `While` (with a `Pre` block holding what the condition needs), `ForRange`, `ForIndex`, `Break`, `Continue`, `Return` and `SetLoc` (an index into the program's location table). A `Program` holds the forward declarations, `QFnInfo`s, struct and element-type statics, the source location table, default-argument functions, functions and `main`.

**Evaluation order is explicit.** Lowering an expression appends the statements it needs to the current block and returns an expression that is safe to evaluate where it is used.
- Operands run left to right: a function value before its arguments, a method's receiver before its arguments, the arguments, then filled-in defaults; the object (and index) of an assignment target before the value (`obj.k = v`, `xs[i] = v`); struct literal fields in the order written. An operand is moved into a temporary only when a later operand needs statements or has effects (`operands`, `sequence` in `lower/expr.go`), so `x + 1` stays `q_checked_add(quark_x, 1)`.
- Conditional and repeated contexts get their own blocks, so their temporaries live in the branch or loop that runs them: the right side of `and`/`or`, both branches of `x if c else y`, each `elseif` condition (inside the else branch of the test before it), and a `while` condition, which is re-run before every iteration (`while (true) { pre; if (!(cond)) break; body }`, or `while (cond)` when it needs no statements). `and`/`or` on two raw bools that need no statements are C++ `&&`/`||`.
- An `if` or `when` used as a statement assigns no result; used as a value, each branch assigns a `QValue` temporary.

**Queries** (`lower/query.go`, runtime `table/query.hpp`). The source becomes a `QTable*` temporary and each step a new one: `sort`, `head`/`tail` and `summarize` call engine kernels (`q_tbl_sort`, `q_tbl_group` + `q_tbl_agg_*` + `q_tbl_make`); `filter` and each `mutate` column are a `ForIndex` over the rows of the current table, with the columns the expression reads bound once (`const QVector&`) and read raw (`VecElem`, `q_tbl_str`). Per row, a strict part of the expression runs only inside `if (present)`, where `present` ANDs the validity of the columns it reads (`q_tbl_present`); `and`/`or`/`!`/ternary parts are lowered first into a value and a presence temporary each, with three-valued logic, and stand for themselves in the strict part through `lowerer.rowSubst`. Filter pushes kept rows into a `RowSet` (`q_tbl_take`); a mutate column pushes each value or a missing value into a `ColOut` and becomes a column of the next table (`q_tbl_put`, with the static `QStructDef` of the row after that column). Programs that use queries or table methods include `table/query.hpp` (a builtin's `Spec.Header`, passed on as `CallPlan.RuntimeHeader`).

**Naming** (see also §4):
- User names are prefixed `quark_` (`quark_x`). A module member is `quark_<internal name>`.
- Names the compiler invents start with `quark___` (`ir/storage.go`): lambdas `quark___lambda1`, nested `fn` declarations `quark___fn1_name`, a second variable with an existing variable's name `quark___v<id>_name`. User names never contain `__`, so these can't collide. Temporaries are `_t<N>`.
- A native (fully annotated) function `f` also gets a `quark_f__thunk` with the QValue convention, for use as a value. An `extern fn` is called by its own C++ symbol (`qei_sqrt`), and its first-class thunk is `_thunk_<symbol>`.
- Every function value carries a static `QFnInfo` (`_qfi_<name>`); functions with defaults also get `quark_<name>__default_arg`.

**Variables.** The analyzer decides each symbol's storage once (`finalizeSymbols`); lowering only reads it (`lower/store.go`). Five kinds (`ir.Storage`):

| Storage | C++ type | Condition |
|---|---|---|
| `StorageCell` | `QCell*` | captured by any closure; read and written as `->value` |
| `StorageInt` | `long long` | not captured, `StoreType` is `int` |
| `StorageFloat` | `double` | not captured, `StoreType` is `float` |
| `StorageBool` | `bool` | not captured, `StoreType` is `bool` |
| `StorageValue` | `QValue` | everything else (untyped, `any`, containers, `when` bindings, function values) |

Every value stored into a symbol is made to have its `StoreType` (`storeValue`): an int is widened to a float, and a value whose type the analyzer didn't prove (from `any`, a union) is checked: `q_param_*` on function entry, `q_var_*` for a typed declaration (`variable 'x' is declared int but got str`). Struct fields use the same check table (`checkFor`) with `q_field_*`, and returns with `q_ret_*` (`checkedReturn`).

A symbol's C++ variable is declared (`DeclVar`) at its `Decl` node (a hoisted cell, created unassigned at its scope's start, is only assigned there) and otherwise assigned (`SetVar`), so lowering keeps no scope state of its own; two variables with one name never share a C++ variable. Captures received from an enclosing scope (`_cl->captures[i]`) are `QCell*` under the same `CName`. A read (`VarRef`) has its storage's representation; a use that needs a `QValue` boxes it (`qv_int(quark_x)`).

A native function receives scalar parameters as `int64_t`/`double`/`bool` and keeps them raw unless captured. Loop variables over a list or vec use the element type the analyzer inferred; `range()` loop variables are `long long` unless captured.

**Function calls**, by the call plan's dispatch mode:
- `DispatchBuiltin` → `q_print(arg)`.
- `DispatchDirect` → `quark_foo(nullptr, arg)`: a `fn` declaration that captures nothing. This is the usual path for named functions.
- `DispatchClosure` → `q_call1(quark_foo, arg)`: a function value (lambda variable, parameter, list element) or a declaration that captures variables (held in a fixed binding).
- `DispatchExtern` → `qei_sqrt(q_as_float(arg))`: convert the arguments to the native types (a raw scalar of the right kind is passed as is), call native, box the result where a `QValue` is needed.
- `DispatchNative` → `quark_foo(nullptr, raw_arg)`: a fully annotated, non-capturing declaration with a native C++ signature.
- `DispatchDynamicMethod` → a chain of `if (recv.type == QValue::VAL_STRING) { _t = q_split(recv, arg); } else if ...`, ending in `q_no_method(recv, "split")`: a builtin method on a receiver whose type is unknown at compile time (for example the result of `unwrap`).

An unexpected dispatch mode, runtime symbol or missing analyzer output stops lowering with an internal compiler error (`lower.ICE`, `INV-*`) rather than emitting fallback code.

**Other lowerings:**
- **Defaults**: missing trailing arguments are filled from the call plan's `DefaultNodes`; calls through function values fill them at runtime (`q_call_adapt`, §2.2).
- **Lambdas**: emitted as top-level C++ functions, with the closure allocated where the lambda is evaluated (`MakeClosure`, always the value of a `Let`).
- **`for` loops**: index loops. Over a value typed as a list, the loop reads the list's elements directly; over an iterable of unknown type it calls `q_iter_get`. Both check before every step that a list still has the length it had at the start (semantics.md §14). The loop variable uses a scalar storage when the iterable's element type is known. `range()` loops and vector loops have fast paths (§9).
- **Conditions** go through `q_truthy()` (`result` and `vec` conditions are rejected), or are a raw C++ `bool` when the condition has a raw bool representation.
- **`when`**: the matched value in a temporary, then a chain of `if`/`else` with result payload extraction; a binding is a `QCell*` or a `QValue` per its symbol's storage.
- **Blocks**: every block, including `for` and `while` bodies, hoists its `fn` declarations first (`hoistScopes`) and sets each statement's source location (`SetLoc`, dropped when no code ran under the previous one). Every location is one entry of a static table, `static const QSourceLoc _qloc[]`, and setting it is one pointer store, `q_set_source_loc(&_qloc[3])`, so marking every statement of a loop body costs nothing measurable.

**The verifier** (`lower/verify.go`) runs on every compile. It checks that every variable and temporary is declared before it is used, in an enclosing scope of the same function, and only once per scope; that every value has the representation its use needs (a raw bool for a condition, a `QValue` for `q_truthy`, the storage's type for a variable); that every `Unbox` carries a proof that fits it (§12); that values a loop re-evaluates each iteration are temporaries or constants; that `break` and `continue` are inside a loop and `return` inside a function; that closures are bound by a `Let` and capture cells in scope; and that calls of generated functions and `q_callN` pass one argument per parameter. A failure is an internal compiler error (`INV-IR-*`, `error_codes.md`) reported by the driver; no C++ is emitted.

**Output order** (`emit.Program`):
1. `#include "quark/quark.hpp"`, then the `extern` glue headers.
2. Forward declarations of every function (and native thunks and extern thunks).
3. `QFnInfo` statics, `QStructDef` statics, the source location table (`_qloc`), element-type descriptors (`_qet_N`), default-argument functions.
4. Function definitions, then extern thunk bodies.
5. `main()`: `q_gc_init()`, `q_set_args`, `hoistScopes` for the top level and every module body (cells, fixed bindings and closures exist before the first statement), module-body bindings, then the top-level statements.

### 1.7 C++ Compiler → Binary

`toolchain.Find()` locates clang++, the runtime headers and the GC once. The build has two steps, so native objects and libraries can join the link:

1. **Compile** (`Toolchain.Compile`): C++ → object file with clang++ (g++ is not supported because of `gc_allocator` / `std::hash` incompatibilities): `-std=c++17 -O3`, `-march=x86-64-v3` on amd64, `-DQUARK_USE_GC` and the GC include flags, `-Wno-deprecated-declarations` on Windows, and `-include-pch <cached quark.hpp PCH>` unless `--no-pch`.
2. **Link** (`Toolchain.Link`): objects + native libraries from `extern link` + static `libgc` (`-pthread`, and `-ldl` on Linux) + `-lm` on non-Windows. With `--lto`, links with `lld` when `ld.lld` is on PATH, since the system linker needs LLVM's gold plugin for LTO.

**Native dependencies** (`toolchain/native.go`). `main.go` combines `driver.Unit.Native` (include directories, libraries in import-graph link order, the extern glue headers) with `--lib-path` / `--include-path` / `--link-arg` and `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH` into `toolchain.Options`. Before linking, `Link` looks for every library in the user directories, then `LIBRARY_PATH`, then clang's own search path (`clang++ -print-search-dirs`), and reports each missing one as `QK-FFI-002`. The link line gets `-L` for each user directory, `-Wl,-rpath,<dir>` for user directories that provide a shared library, then `-l<name>` in link order, then `--link-arg` values. Extra include directories are added to the compile but not to the precompiled header, which stays shared between programs. When clang reports an error located in an extern glue header (or a file it includes), `Compile` returns a `QK-FFI-003` diagnostic.

Intermediate files go in a fresh `os.MkdirTemp` directory per build, so concurrent builds never collide.

**Precompiled runtime header.** `toolchain/pch.go` precompiles `quark/quark.hpp` into `$XDG_CACHE_HOME/quark/pch/<key>.pch` (`os.UserCacheDir()`), where the key hashes the compiler binary, the compile flags, and the path, size and mtime of every runtime and GC header. A PCH clang rejects (for example after a system header update) is deleted and the compile retried without it. This roughly halves compile time for small programs.

---

## 2) Runtime Value System

The runtime is header-only C++17 under `src/core/quark/runtime/include/quark/`, included through `quark.hpp` (every `builtins/` header must be listed there). The table engine and CSV reader (`table/`) are not included by `quark.hpp`.

### 2.1 QValue

Every runtime value is a `QValue`, a tagged union (`core/value.hpp`):

```cpp
struct QValue {
    enum ValueType {
        VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL,
        VAL_LIST, VAL_VECTOR, VAL_DICT, VAL_FUNC, VAL_RESULT, VAL_RESOURCE,
        VAL_STRUCT, VAL_TABLE
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;
        QVector* vector_val;
        QDict* dict_val;
        void* func_val;        // a QClosure*
        QResult* result_val;   // {bool is_ok, QValue payload}
        QResourceHandle* resource_val;
        QStruct* struct_val;
        QTable* table_val;     // a tbl
    } data;
};
```

Value constructors use the `qv_` prefix: `qv_int(42)`, `qv_string("hello")`, `qv_null()`. `q_type_name(ValueType)` (`core/value.hpp`) is the canonical type-name table.

**Reading a union field.** Code may read `.data.int_val` (or any other field) without a tag check only where the analyzer has proven the type (§12). Values from untyped paths (`any`, callbacks, struct fields, native code) go through the checked `q_as_*` / `q_param_*` / `q_var_*` / `q_field_*` / `q_ret_*` helpers.

**Errors.** A runtime operation that gets a value it can't handle reports it and exits: `q_runtime_reportf(...)` (`core/diagnostics.hpp`), which prints `error[QK-RUNTIME-001] (runtime): message at file:line:col`, followed by `exit(1)`. It never returns `qv_null()` as a stand-in for a failure; `null` is returned only where it is the documented result (`list.get` past the end, `dict.get` on a missing key, `sys::env` of an unset variable).

### 2.2 QClosure and QFnInfo

Every function value is a `QClosure*` (`types/closure.hpp`):

```cpp
struct QFnInfo {
    const char* name;                  // display name for errors
    int min_arity;                     // parameters without a default
    int max_arity;                     // all parameters
    QValue (*default_arg)(int index);  // builds a default fresh on each call; null if none
};

struct QClosure {
    void* func;             // function pointer; takes QClosure* as hidden first arg
    const QFnInfo* info;    // never null
    int capture_count;
    QCell* captures[];      // captured variable cells
};
```

Every generated function takes a hidden first parameter `QClosure* _cl`. Calls through a closure (`q_callN`) pass the closure; direct calls (`DispatchDirect`, `DispatchNative`, the call inside a thunk) pass `nullptr`. New closure-creation sites must pass an info: `q_alloc_closure(fn, info, n)`, `qv_func(fn, info)`.

`q_call0` … `q_call12` (`types/function.hpp`) go through `q_call_values`, which calls directly when the argument count equals the parameter count and otherwise uses `q_call_adapt`: it checks the count against `min_arity`/`max_arity` (`function 'h' expects 2 arguments but got 1`) and fills defaults. `qext::call` uses the same path. At most 12 arguments (`Q_MAX_CALL_ARGS`).

### 2.3 QCell

`QCell { QValue value; }` is a heap cell for a variable captured by a closure. All closures over one variable share its cell, so they see each other's writes. A variable that is never captured lives on the stack (§1.6).

A cell for a variable that a hoisted function captures is created unassigned (`q_new_unassigned_cell`) at its scope's start and read with `q_cell_get`, which reports `'total' is used before it is assigned` instead of reading `null`.

### 2.4 Containers

- **QList**: a `std::vector<QValue, q_allocator<QValue>>` plus `elem`, the `const QElemType*` element type it was created with (`nullptr` for any value).
- **QDict**: a string-keyed `std::unordered_map` with `q_allocator`, plus `elem`, its value type.
- **QElemType** (`core/value.hpp`; checks in `types/elemtype.hpp`): a runtime type descriptor: tag, element type for lists and dicts, ok/err types for results, the `QStructDef` for structs, the dtype for vecs. Scalars use `q_elem_int`/`_float`/`_bool`/`_str`; lowering emits a static `_qet_N` for every other type. Every list and dict write in the runtime goes through `q_elem_store` (push, set, insert, concat, `d.k = v`, `.set`, `qext::list_push`, `qext::dict_set`), which widens an int for a float container and rejects anything else; a container value must have an element type `q_elem_sub` accepts (no conversion). List-creating builtins pass the element type their catalog return type claims (`qv_list(n, &q_elem_int)`).
- **QVector**: typed columnar storage, dtype f64, i64, bool or str, in the Arrow layout (`types/vector.hpp`): contiguous values, bit-packed bools, int32 offsets plus UTF-8 bytes for strings, a validity bitmap that is `nullptr` when there are no nulls, 64-byte aligned buffers. Vectors are immutable.
- **QResult**: `{ bool is_ok; QValue payload; }`.
- **QStruct** / **QStructDef** (`types/struct.hpp`): a struct value points to its definition; identity is the definition's address (`q_param_struct` / `q_field_struct` compare pointers), and `QStructDef.name` is the display name (`geo::Point`).
- **QTable** (`types/table.hpp`): a `QStructDef` naming the columns, a row count, and one `QVector*` per column (`specs/tables_v0_1_spec.md` §7). The `QStructDef` is the row type: `field_dtypes` gives each field's column type and `anonymous` marks a row a query derived or the CSV reader inferred. `q_table_conform` is the shape rule at runtime (§12).

### 2.5 Capture Analysis

The analyzer finds each lambda's free variables: identifiers that aren't its parameters or locals, aren't builtins, and resolve in an enclosing scope. These become the lambda's capture list (`Analysis.Captures`); nested lambdas capture transitively from the scope that defines the variable. A `for` variable or an `ok v`/`err e` binding that shadows an outer variable is not a capture of it.

---

## 3) Memory Management

All heap allocation goes through Boehm GC (`QUARK_USE_GC`, always defined):

| Path | Use | GC behaviour |
|---|---|---|
| `q_malloc(n)` / `q_new<T>(args...)` | Objects containing pointers (closures, cells, results, containers) | Scanned |
| `q_malloc_atomic(n)` / `q_strdup(s)` | Pointer-free data (strings, numeric buffers) | Not scanned |
| `q_allocator<T>` (STL allocator) | Buffers of `std::vector`, `std::unordered_map`, `std::string` stored in runtime data | Scanned or atomic by element type |

There is no manual free. The collector never runs destructors and doesn't scan `malloc`'d memory, so a `std::string`/`std::vector`/map stored in (or reachable only from) runtime data must use `q_allocator`, as dict keys and the resource registry do; otherwise it leaks, or its contents get collected while live. `integration_gc_test.go` (programs in `src/testfiles/gc/`) checks both by comparing the GC heap and peak RSS across repeated rounds.

`q_gc_init()` is the first statement of the generated `main()`.

---

## 4) Naming Conventions

| Prefix | Domain | Examples |
|---|---|---|
| `q_` | Runtime builtins and helpers (C++ runtime code) | `q_add()`, `q_print()`, `q_malloc()` |
| `quark_` | User names in generated C++; compiler-invented names start with `quark___` | `quark_x`, `quark_geo__area`, `quark___lambda1` |
| `qv_` | Value constructors | `qv_int()`, `qv_string()`, `qv_null()` |
| `QValue`, `QList`, `QClosure` | Type names (PascalCase) | |

**Never mix `q_` and `quark_`**: `q_` is runtime-only, `quark_` is for generated code only. The exceptions are `extern fn` symbols, called by their own C++ name, and their `_thunk_<symbol>` wrappers.

---

## 5) CallPlan IR

`ir.CallPlan` (`ir/call.go`) freezes call semantics after analysis so the back end doesn't re-derive them:

```go
type CallPlan struct {
    CalleeName         string
    MinArity, MaxArity int
    Dispatch           DispatchMode
    RuntimeSymbol      string            // C++ symbol: "q_upper", "quark_myfunc", "qei_sqrt"
    DefaultNodes       []*ast.TreeNode   // trailing default args to inject
    IsMethod           bool              // x.method(args): receiver is the first runtime arg
    ReceiverNode       *ast.TreeNode
    ReceiverTypeKey    string            // builtins.TypeKey of the receiver
    MethodCandidates   []MethodCandidate // for DispatchDynamicMethod: value tag, type key, runtime symbol
    NativeParamTypes   []string          // extern/native: "int64_t", "double", "bool", "const char*", "QVector*", "QList*", "QDict*", "QClosure*", "QValue"
    NativeReturnType   string
    NativeReceiverType string            // extern methods
}
```

**Dispatch modes:**
- `DispatchBuiltin`: a `q_*` runtime function.
- `DispatchDirect`: a `quark_*` function called with a `nullptr` closure; a `fn` declaration that captures nothing.
- `DispatchClosure`: `q_call*` through a `QClosure`.
- `DispatchExtern`: an `extern fn`. Arguments are unboxed to the native types above at the call site and the result boxed back; a QValue-convention thunk lets the function be used as a value.
- `DispatchNative`: a fully annotated, non-capturing `fn` declaration. The back end emits a native C++ signature and a QValue thunk. Whether a function qualifies is decided after the whole program is analyzed (`finalizeFixedFns`), so a native function may call other non-capturing functions.
- `DispatchDynamicMethod`: a builtin method on a receiver of unknown type. `resolveDynamicMethodCall` collects every builtin receiver type (str, list, dict, vec) that has the method and accepts the argument count; lowering tests the type tag, and any other type is a runtime error (`q_no_method`). Argument types are checked at compile time only when a single receiver type matches. If no type has the method, or none accepts the argument count, it is a compile-time error. `INV-DYNAMIC-METHOD` checks each candidate against the catalog.

Plans for calls to `fn` declarations are recorded with `deferFixedDispatch` and resolved in `finalizeFixedFns`.

---

## 6) Builtin Catalog

`builtins/catalog.go` is the single source of truth for prelude builtins. Its consumers are the analyzer and the invariant checks; the back end never reads it and emits `CallPlan.RuntimeSymbol`. The editor extension's completion data is generated from it (`internal/editordata`).

```go
type Spec struct {
    Name         string   // "upper"
    Runtime      string   // "q_upper"
    MinArgs      int
    MaxArgs      int
    ParamTypes   []TypeKey
    ReturnType   TypeKey
    ReceiverType TypeKey  // "" = free function; non-empty = method
}
```

Methods are indexed by `(ReceiverType, methodName)`, free functions by name. Builtin names can't be redefined or assigned by user code (parameters and other locals may shadow them).

**Adding a builtin:**
1. Add a `Spec` in `builtins/catalog.go` (`ReceiverType` for methods).
2. Implement it in `runtime/include/quark/builtins/*.hpp` with the `q_` prefix (a new header goes in `quark.hpp`'s include list). A list-returning builtin passes its element type (§2.4).
3. The analyzer and the back end pick it up through the catalog.
4. Add a smoke test in `src/testfiles/smoke_*.qrk`.
5. Regenerate the editor data: `go test ./internal/editordata -update`, and add the name to the grammar's `builtin-functions` pattern (the test says so if it's missing).

Functionality that isn't needed in the prelude belongs in a stdlib module (`src/core/stdlib/`), not the catalog.

---

## 7) Extern Function System

`extern fn` binds a C++ function (`semantics.md` §10.8, `specs/ffi_v0_1_spec.md`). Stdlib modules use it: each is a `.qrk` file with a primary `module X:` block that binds a sibling `.hpp` with `extern 'x.hpp'`.

### Pipeline

1. **Parser**: `ExternHeaderNode` (header path), `ExternLinkNode`, `ExternIncludePathNode`, and `ExternFnNode` (with `ExternSymbol` and `ExternReceiver`).
2. **Loader**: rewrites header paths to absolute paths (the back end emits `#include "/abs/path.hpp"`) and collects link and include-path directives (§1.7). The directives are allowed only at the top level of a file.
3. **Analyzer** (`analyzeExternFn`): maps annotations to C++ types (`quarkTypeToNativeCType`), registers free functions and methods (`externFns[name]`, `externFns["type.method"]`), and builds a `DispatchExtern` prototype plan. Parameters follow the same rules as Quark parameters (checked, int widens to float, `null` only for struct and `resource`).
4. **Lowering**: converts each argument with `q_as_int`, `q_as_float`, ... (which check the tag: `expected int, got str`), boxes the result, and emits a thunk for first-class use. A native function returning a null pointer for `str`, `list`, `dict`, `vec` or `fn` is a runtime error.

### Extension author API (`ext/api.hpp`, namespace `qext`)

| Category | Functions |
|---|---|
| Memory | `malloc`, `malloc_atomic`, `strdup` |
| Boxing | `box(int64_t)`, `box(double)`, `box(bool)`, `box(const char*)`, `box(QVector*)`, `box(QList*)`, `box(QDict*)`, `box(QClosure*)`, `box_own`, `null_val()`. Boxing a null container or closure pointer is an error (use `null_val()`) |
| Unboxing | `as_int`, `as_float`, `as_bool`, `as_str`, `as_vector`, `as_list`, `as_dict`, `as_closure` (checked) |
| Vectors | `as_f64`, `as_i64` (and `_mut` for freshly created vectors), `bool_at`, `set_bool`, `str_at`, `is_null_at`, `set_null`, `validity_bitmap`, `vec_size`, `vec_null_count`, `vec_has_nulls`, `vec_dtype`, `vec_dtype_name`, `new_f64`, `new_i64`, `new_bool_vec` |
| Lists | `new_list()` (holds any value), `list_push` (checks the element type) |
| Dicts | `dict_get`, `dict_set` (checks the value type), `dict_size`, `dict_has` |
| Calls | `call(fn, ...)` with 0-3 `QValue` arguments; arity-checked and default-filling like `q_callN` |

Native code that appends to a list with `push_back`, or writes `d->entries` directly, skips the element-type check; use `list_push` / `dict_set`.

---

## 8) Resource System

Opaque handles for external state (file handles today; `types/resource.hpp`). Every handle has the single Quark type `resource`; its kind (`file`) is runtime-only.

**Design**: a generational slot registry.
- `QResourceHandle`: an opaque token `{slot, generation, kind}` in the value's union, allocated with `q_malloc_atomic`.
- `QResourceSlot`: `{generation, kind, flags, payload, occupied}`; `payload` is an opaque `void*` (`FILE*` for files).
- `QResourceRegistry`: a global singleton with a `slots` vector and a `free_list`, both on `q_allocator` so the GC sees the payloads.

**Lifecycle:**
1. `q_resource_create(kind, payload, flags)` allocates a slot (reusing the free list), bumps its generation, and returns a handle.
2. `q_resource_resolve(value, expectedKind, &slot, &handle, &err)` validates the handle: slot in range, occupied, generation and kind match.
3. `q_resource_invalidate(handle)` frees the slot and pushes it to the free list; the generation stops stale handles from aliasing a reused slot.

| Condition | Error |
|---|---|
| Null or non-resource value | `invalid resource handle` |
| Slot index out of range | `invalid resource handle` |
| Slot not occupied | `resource is closed` |
| Generation mismatch | `stale resource handle` |
| Kind mismatch | `resource kind mismatch` |

---

## 9) Lowering Details

### Range loops

`for i in range(...)` becomes a raw C++ `for` loop over start/end/step temporaries:

```cpp
for (long long quark_i = _t1; _t3 > 0 ? (quark_i < _t2) : (quark_i > _t2); quark_i += _t3) { ... }
```

All three `range()` forms are supported; the 2-argument form picks step `1` or `-1` from the direction of start and end. No list is allocated. Not used when the loop variable is captured. Arguments follow the rules of the `range()` builtin: scalar ints are used directly, scalar floats are truncated, other values go through `q_range_arg`, and a step goes through `q_range_step` (a step of `0` is a runtime error).

### List loops

`for x in xs` over a value the analyzer typed as a list binds the `QList*` once and reads elements directly; the loop test, `q_for_list_more`, also fails (`q_list_resized`, at the `for`'s location) if the body changed the list's length:

```cpp
QList* _t3 = q_for_list(_t1);              // a runtime error if _t1 isn't a list
long long _t4 = q_list_len(_t3);           // the length at the start
for (long long _t5 = 0; q_for_list_more(_t3, _t4, _t5, &_qloc[0]); _t5++) {
    long long quark_x = (q_list_at(_t3, _t5)).data.int_val;
    // ... body ...
}
```

Since the length can't change, every read is in range, and the raw read of the loop variable relies on the list's element type (§12, rule C). The test runs before every step, so `continue` and a change in the last iteration are caught; `break` leaves without a test. An iterable of unknown type uses `q_iter_get` with the same test (`q_for_more`), which checks the length only when the value is a list. About 2.5x faster than `q_iter_get` on a loop that sums 5M ints 20 times.

### Vector loops

`for x in v` over a vector checks at runtime whether the dtype matches the loop variable's storage and the vector has no nulls; if so it iterates the typed buffer directly:

```cpp
if (q_vec_is_dense_of(_t8, QVector::Type::I64)) {
    const QVector& _t9 = *_t8.data.vector_val;
    for (long long _t10 = 0; _t10 < (long long)_t9.count; _t10++) {
        long long quark_x = static_cast<long long>(q_vec_i64_data(_t9)[_t10]);
        // ... body ...
    }
} else {
    // generic q_iter_get path
}
```

Not used for captured loop variables or unknown element types.

### Vector literals

When the analyzer knows a vector literal's element type, lowering uses typed constructors (`qv_vector_i64` + `q_vec_push_i64`); otherwise it builds a list and calls `q_to_vec`.

### Scalar expressions

Arithmetic (`+ - * / %`), unary `-` and comparisons are raw C++ (`Arith`, `Neg`, `Compare`) when both operands have a raw scalar representation: a literal, a scalar-stored variable, a native call's result, or a sub-expression that is itself raw. Int arithmetic keeps the boxed operators' failure semantics through checked helpers (overflow, division and modulo by zero are fatal):

```cpp
// c = a + b, d = a / b, f = (a + b) * 2 > 5   (a, b are long long)
long long quark_c = q_checked_add(quark_a, quark_b);
double quark_d = q_checked_fdiv((double)quark_a, (double)quark_b);
bool quark_f = (q_checked_mul(q_checked_add(quark_a, quark_b), 2) > 5);
// if a < b and b < 3: ...   (a, b are long long)
if (((quark_a < quark_b) && (quark_b < 3))) { ... }
```

Float `+ - *` and comparisons use plain C++ operators. `/` always produces `double`. `**` is never raw (`q_pow`). `and`/`or` of two raw bools is `&&`/`||`. The result stays unboxed when assigned to a scalar variable or used as a condition, and is boxed otherwise. Any `QValue` operand falls back to the boxed `q_add` etc.

---

## 10) GC Bootstrap

`toolchain.EnsureGC()` finds or builds Boehm GC. The repository commits only the GC's public headers and prebuilt static libraries (`deps/bdwgc/VENDORED.md`):

1. **GC directory**: `deps/bdwgc/` relative to the compiler executable or the current directory, validated by `include/gc/gc.h`.
2. **Prebuilt library**: `deps/bdwgc/lib/<GOOS>-<GOARCH>/libgc.a` (`gc.lib` on Windows), present for `linux-amd64`, `linux-arm64`, `darwin-arm64` and `windows-amd64`; `deps/bdwgc/build-prebuilt.sh` rebuilds them.
3. **Local build**: otherwise a static library in `deps/bdwgc/build-static/`. Shared libraries are never used, so executables run without `libgc` installed.
4. **Auto-build**: if neither exists, the GC source is present and `cmake` is available, it builds into `build-static/` with `GC_BUILD_SHARED_LIBS=OFF` (on Windows with `clang` as the C compiler, to match the MSVC ABI). Without the source, it fails with an error pointing to `VENDORED.md`.

---

## 11) Testing

All tests run from `src/core/quark` with `go test ./...`.

- **Integration tests** (`integration_*_test.go`): `TestMain` builds the `quark` binary once (`src/core/quark/quark_test`) and tests run it on programs. `TestSmokePrograms_Run` compiles and runs every `src/testfiles/smoke_*.qrk` and compares stdout exactly; `TestSmokePrograms_CompileError` checks expected error substrings. `TestSemanticsSafety_Rejected` holds the negative cases the robustness bar asks for (wrong type, arity, `null`, `any`, defaults, other modules, native code). `TestEvaluationOrder` checks that each part of an expression runs when and as often as it should. Other files cover the GC (`integration_gc_test.go`), FFI (`integration_ffi_test.go`, which builds the C libraries in `src/testfiles/ffi/`; `TestStdBlas` skips without OpenBLAS), element types (`integration_types_test.go`) and the table engine (`integration_table_test.go`).
- **C++ runtime tests**: `TestVectorArrowLayout` checks the vector layout byte by byte; `TestTableEngine` builds and runs `src/testfiles/table/*_test.cpp` (sharing `check.hpp`). Go's test cache doesn't see C++ changes, so run them with `-count=1`.
- **Package tests**: lexer, parser, loader (`loader/modules_v02_test.go`), analyzer, `lower/` (every smoke program lowers and verifies; malformed IR is rejected), `emit/` (the C++ shapes of each lowering) and `internal/editordata` (generated editor data, grammar coverage, snippets parse, and the extension's JS tests when `node` is available).
- **GC self-check**: a standalone C++ program that allocates every kind of runtime value and checks with `GC_base()` that every object and buffer is on the GC heap.

The integration binary is shared: a second `go test` in the same checkout deletes it mid-run, and every integration test then fails with `fork/exec .../quark_test: no such file or directory` (`known_issues.md`).

---

## 12) Soundness Contract

The analyzer's types serve two purposes. For diagnostics they are a best guess. For the back end they are a guarantee: it reads raw union fields and stores values in native C++ scalars whenever the analyzer says `int`. Every silent wrong result Quark has had came from a slot (variable, parameter, element, field, return value) whose type the analyzer stated, with a way in that nothing guarded.

**The contract.** A static type is a guarantee only when every way a value can reach the slot is guarded: proven at compile time, or checked at runtime. The back end may read raw bytes only for such types.

Parameters show the pattern:

```
fn add(x: int, y: int = 10) int -> x + y
add('x', 29)          // compile error: argument 1 of 'add' expects int, got str
p = 9 * 133
add(p, 93)            // proven int: no check
add(20, id('oops'))   // id is untyped: checked on entry, runtime error
```

One way in (the call), and it is guarded; after `long long quark_y = q_param_int(...)` the body reads raw values.

**Rules** that extend this to every slot (decided and implemented 2026-11-13):

- **A. Variables don't narrow.** A variable whose first value has an unknown type stays untyped whatever it is assigned later. To get raw speed, convert once: `n: int = x` (one check, raw afterwards). A variable holding a function keeps its signature; `g: fn = ...` holds any function. (`assignName`/`checkReassign`.)
- **B. A list's element type (a dict's value type) is fixed when it is created**, through every alias: `xs = list [1, 2]` is `list[int]`, and `xs.push('s')` is a compile error. A mixed literal or a literal under a `list`/`dict` annotation holds any value (`contextLiteral`). An empty literal is typed by its context or its first write (§1.4.2). (`checkElemWrite`, `checkMutation`.)
- **C. A write the compiler can't check is checked by the runtime.** An untyped function changing a typed list can't be rejected at compile time, so every list and dict records its element type and every write checks it (`q_elem_store`, §2.4), as Java arrays do. Reads stay raw.
- **D. Return annotations are checked on every function**: at compile time when the body's type is known (a body that can end without a value is an error; an `if` without `else` adds `void` to its type), otherwise on return (`checkedReturn`, `q_ret_*`).
- **E. A wrong-typed dict value is an error**, not a warning.
- **Null.** `null` is a value of struct and `resource` types only, in the checker (`IsSubtype`), in every runtime check (`paramTagChecks`), and in native boxing. Whether struct and resource annotations should reject it too is open.

**Cost.** Measured 2026-11-13 in tight loops that do nothing but the operation (`-O3 -march=x86-64-v3`, best of 5):

| Benchmark | Base | Variant | Change |
|---|---|---|---|
| 20M `out.push(i)` | 0.330s | element tag check on each write (rule C) | +1.6% |
| 100M list element reads | 0.775s | tag check on each read (the alternative to C) | +1.5% |
| 30M calls of `fn step(x: int, k: int = 3) int` | 0.160s | checked return (rule D) | -0.1% |

Rules A, B and E cost nothing at runtime and let the compiler prove more types. Checking writes was chosen over checking reads because it keeps reads raw, which the list-loop speedup in §13 depends on.

**Raw reads say why they are safe.** Every `Unbox` in the IR carries a proof, and the verifier rejects one without it (`INV-IR-UNBOX`):
- `ProofType`: the analyzer typed the value exactly as the scalar (`Unbox.Type` is `int` for a `long long` read). Lowering takes the type from the analysis at the point it unboxes; a union, `any` or another scalar is rejected.
- `ProofElement`: a `for` loop's element read (`q_list_at`, `q_iter_get`), whose type the runtime enforces: every write into a list is checked against its element type (rule C), a list keeps its length during the loop (§9), and a missing vec entry taken out is an error (D12).

A runtime function's `QValue` result is never unboxed on trust; a function whose result lowering needs raw returns the C++ type itself (`q_for_len` returns `long long`, `q_match_eq` a `bool`). `q_eq` shows why: on a vec it returns a vec, and `when` value patterns, which unboxed it as a bool, read garbage for one; they call `q_match_eq`, a runtime error for a vec.

The reads that could give `null` where the analyzer said "element" are closed: `xs[i]` and `s[i]` out of range are runtime errors (`q_index`), and `xs.get(i)` and `d.get(k)` have type `any`.

**Tables.** A `tbl[R]`'s columns are read by index and their buffers raw, so every table in a `tbl[R]` slot has `R`'s definition, or an anonymous one with the same field order. Every way in is guarded: a typed read gets the definition from the compiler; a query step builds its output with the static definition the analyzer derived; a value from anywhere else reaching a `tbl[R]` parameter, variable, field or return value goes through `q_param_table` / `q_var_table` / `q_field_table` / `q_ret_table`, which accept `R`'s own definition, relabel an anonymous row of `R`'s shape (a new `QTable` with the columns reordered, an int column copied as floats) and reject anything else; a function with a `tbl[R]` in its signature keeps the QValue convention, since a native `QTable*` can't carry the check. Containers never relabel: a `tbl[R]` element descriptor matches `R`'s definition only. A missing value taken out of a column is an error (D12), and verb expressions read a slot only after testing its validity.

**Holes left** (listed with reproductions in `known_issues.md`): native code writing lists directly skips the element check.

**Finding holes mechanically** (planned):
- A *checked mode* build flag turns every raw read into a checked one; running the test corpus in both modes, any check failure or output difference is a soundness bug.
- A differential test for single expressions: `print(E)` (boxed, trusts nothing) must print what `x = E`, `print(x)` (stored as the claimed type) prints.

---

## 13) Roadmap: Back End

The back end is the lowering pass, IR, verifier and emitter (§1.6). What is left to do in it:

**Remaining steps** (each ends with the full suite green):

1. **Checked mode**: a build flag that turns every `ProofType` and `ProofElement` unbox into a checked read (§12, Finding holes mechanically). Running the test corpus in both modes, any check failure or output difference is a soundness bug.

Tables (`specs/tables_v0_1_spec.md`) are lowered in `lower/query.go` (§1.6, Queries), with their runtime in `table/query.hpp`.

---

## 14) Structural Debt

Independent of §13 and can be done in any order. Each is a decision made in more than one place, where the copies can disagree.

**Go**
- Three functions decide what a constant is: `isConstantDefault` and `isLiteralDefault` (`parser/parser.go`) and `isConstantValue` (`loader/files.go`).
- `parseBlock` repeats `Parse`'s statement loop.
- Internal errors: lowering and the invariant checks return them as errors the driver reports (`lower.ICE`, recovered inside `lower.Lower`), but the analyzer panics (`popScope`).
- The integration tests share one binary (§11); each run should build into its own temporary directory.

**Runtime (C++)**
- No `[[noreturn]] q_fatalf`: about 200 sites call `q_runtime_reportf` then `exit(1)`, and `types/dict.hpp` has dead `return qv_null()` branches after `q_require_dict`. String indexing with a non-int returns `null` (a known issue).
- Type names: `q_type_name` (`core/value.hpp`) is canonical, but `q_type_name_arith`, `q_type_name_math`, an inline table in `ops/member.hpp` (which says `string`, not `str`) and `q_as_type_name` (`ext/api.hpp`) remain. Replace them with `q_type_name`.
- Text formatting: `print_qvalue` (`builtins/io.hpp`, missing the `q_` prefix), `q_str`, `q_repr_into`, `q_fmt_cell` and the `std/fmt` cell formatters. `builtins/repr.hpp` should be the only renderer, with options.
- Boxing: `qv_vector_from`, `qv_vector_ptr` and `qext::box(QVector*)` do the same thing; so do `qv_list_ptr` and `qext::box(QList*)`. `qext::box(int)` doesn't compile (ambiguous overloads).
- Index and size types: list and str methods resolve indexes in 64 bits (`q_resolve_index`, `core/checked.hpp`), but `q_vec_size` returns `int` and `qv_list` takes an `int` capacity. Use `int64_t` throughout.
- Numeric conversions: float→int conversions go through `q_checked_f2i` (`core/checked.hpp`), except `q_to_i64_scalar` (`types/vector.hpp`), which still truncates a float scalar unchecked for `vec[i64]` pushes and the i64 kernels' scalar reader. No Quark program is known to reach it with a float; it should check or reject one.
- `types/vector.hpp` (about 1320 lines): nested reader lambdas multiply template instantiations, about eight per-dtype switches repeat each other, and `q_vec_validate` is called by some entry points and not others. Proposed split: `vector/layout.hpp` (struct, bitmap, buffers, builders: the Arrow contract `ext/api.hpp` depends on), `kernels.hpp` (arithmetic, comparison), `reduce.hpp` (reductions, with `all`/`any`), `convert.hpp` (`to_vec`, `to_list`, `astype`, `fillna`), `index.hpp` (indexing, masks). A single `q_vec_visit_dtype(v, f)` would replace the switches.
- `QFilePayload` holds no pointers and could use `q_malloc_atomic`.

---

## 15) Performance Findings

Measured with the benchmark suite (`src/benchmarks/`; the numbers and how they were checked are in `src/benchmarks/RESULTS.md`, one thread, 2026-11-13 and, for tables and the items that changed, 2026-11-14). Each item is a cost the suite found, its cause, and a direction to try; none is scheduled yet. To measure a change, rerun the affected benchmarks against the old and new compiler: `python3 bench.py run --bench fib,reduce --lang quark --quark <path>`.

**Generated code**
- **Source-location tracking is the largest remaining cost of a function call.** Every generated function saves the current location on entry and restores it on exit (`QSourceLocGuard`, plus an exception cleanup path), and every statement stores one pointer into a static table of locations. `fib(30)` takes 10.7 ms; rebuilt by hand with the runtime headers, 8.7 ms without the per-call save and restore, 7.3 ms with no tracking, 4.3 ms as plain C++ (three stores per statement, the earlier scheme, took 22.1 ms). Direction: avoid the per-call save and restore, for example by recording the location at call sites instead.
- **Arithmetic on call results must stay unboxed.** Boxing both results of `fib(n - 1) + fib(n - 2)` and adding them with the generic `q_add` (not inlined; it tests for vectors and strings first) costs 31.5 against 22.4 ms for `q_checked_add` on the two `int64`s. The lowering pass emits the typed add when both operand types are proven.
- **Overflow checks** cost the last 40% to plain C++ in `fib` (7.7 against 4.3 ms). That is the price of checked integers; it may shrink where the compiler can prove a range.

**Vectors**
- **Fresh memory dominates large element-wise operations.** `2.5 * a + b` on 10M doubles takes 15 ms into memory already mapped and 74 ms into a new `malloc` buffer, because every 4 KB page faults on its first write. NumPy asks the kernel for huge pages for large arrays (`madvise(MADV_HUGEPAGE)`): 30 ms with them, 82 ms without. Quark's GC reuses its heap and takes 47 ms. Direction: huge pages for large vec buffers.
- **Dense fast paths.** Division checks every slot for null and every divisor for zero, `sum` and the mask filter test validity per element even when `null_count == 0`, and the filter switches on the dtype inside its loop (`filter` 195 ms; NumPy 180 ms, Polars 27 ms). Direction: one loop for vectors without nulls, written so clang can vectorize it.
- **Fixed cost per operation** is about 1.9 µs for a 300-element vector (allocation and validation), so row-by-row code on short vectors gains little from `vec` (`gemm`: 341 ms against 374 ms with lists).
- **No fusion**: an expression of eight operations writes seven 80 MB temporaries (195 ms; NumPy 117 ms, Polars 94 ms).

**Lists, dicts and strings** (the list tier is no faster than CPython for these)
- **`list.push`** costs 35-65 ns per element (C++'s `push_back` 19 ns on the same machine). A `QList` is a `std::vector<QValue>` on the GC's scanned heap, so every collection scans every element of every live list, even a `list[float]`; a list of 10M bools peaks at 789 MB (Python: 85 MB), and runs vary by up to 3 times. Directions: allocate the buffer of an `int`/`float`/`bool` list atomic (never scanned), or store such lists unboxed (`list[T]`); a fill constructor (`[x] * n`) and `reserve`.
- **Dict lookups** build a new key string and hash it again on every `get` and `set`, in a node-based `std::unordered_map`; "get, then set" is two full lookups. A dict-based group-by of 1M rows takes 332 ms (CPython 397 ms, Polars 55 ms). Directions: open addressing with stored hashes, an update-in-place operation. The table engine already groups with its own open-addressing table.
- **Strings** are NUL-terminated without a stored length: every method calls `strlen` again, `lower` calls the locale's `tolower` per byte, and every step allocates. `trim().lower().replace()` on 1M strings takes 530 ms (CPython 402 ms). Directions: store the length; ASCII fast paths.
- **Sorting** compares `QValue`s with a type-checking comparator: 280 ms for 1M floats (NumPy 27 ms, R 156 ms). Direction: sort raw doubles when the element type is known; give `vec` a sort.

**Tables** (`specs/tables_v0_1_spec.md`; 500K rows with one float column 1% missing, measured with `quark run`, and the `quark_tbl` benchmarks, 2026-11-14)
- Numeric verb expressions run at loop speed: `mutate(bonus = e.salary * 0.1, total = e.salary + e.bonus)` takes 10 ms, `group_by` + `summarize(mean, count)` 27 ms. In the benchmarks a query of five steps on 1M rows takes 134 ms (data.table 142 ms, Polars 167 ms), `group_by` + `summarize(sum, count)` 52 ms (Polars 55 ms), and an eight-operation `mutate` on 10M rows 59 ms, one fused loop (Polars 93 ms, Quark's `vec` 197 ms).
- **`min` and `max` aggregates use the generic comparator** (`agg_extremum` → `compare_rows`, a type switch and null and NaN tests per row) and have no whole-table path: 76 ms for `summarize(lo = min(e.a))` on 10M floats, against 6.1 ms for `min(t.a)` (`known_issues.md`, Slow paths). Direction: typed loops, and the vec reductions for a whole table, as `agg_sum` and `agg_mean` do.
- **A sort with a str key or several keys** sorts row positions with `compare_rows` per key and comparison: `sort(e.k, desc(e.v))` on 1M rows takes 1.13 s (data.table 255 ms). Direction: replace a str key by its value's rank among the distinct values, then compare ints; a radix sort for the one-dense-numeric-key fast path (173 ms for 1M floats; NumPy 26 ms, Polars 80 ms).
- **`filter` and `mutate` loops test validity per row and per column even when a column has no missing values**, and `filter` appends selected rows with a branch, so neither vectorizes: `filter(e.a > 0.5)` on 10M rows 99 ms (Polars 24 ms), `mutate(z = 2.5 * e.a + e.b)` 65 ms (`vec` 45 ms). Direction: a dense loop when every column read has `null_count == 0`, a branch-free append, column pointers in locals.
- **The CSV reader runs at about 100 MB/s**: 513 ms for 52 MB of five columns (pandas 226 ms, data.table 301 ms, Polars 357 ms, one thread); splitting fields is about half (a read that keeps only one str column takes 280 ms). Direction: a vectorized scan for separators, quotes and line ends.
- **A str column read allocates a Quark string per row** (`q_tbl_str`), so `filter(e.dept == 'eng' or ...)` takes 38 ms and `mutate(up = e.name.upper())` 54 ms. Direction: compare and hash str cells as views of the column buffer, and allocate only where a string escapes (a call or a new column).
- **Sorting by a nullable key** takes the engine's generic comparator: 239 ms, against 97 ms for a dense key (`known_issues.md`, Slow paths).

**Libraries**
- A program that imports `std/blas` starts OpenBLAS's thread pool at load (one thread per CPU), and then large vec allocations make the GC print `Repeated allocation of very large block` warnings and get slower (`blas::axpy` on 10M doubles: 30 ms with `OPENBLAS_NUM_THREADS=1`, 135 ms without). Direction: find why the pool disturbs the GC's large-block allocation, or start OpenBLAS with one thread and let programs ask for more.

