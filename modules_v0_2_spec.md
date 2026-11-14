# Quark Modules v0.2 Spec

Status: Implemented 2026-11-12: fixed and hoisted `fn` declarations (rules 17-23) and the module rules (`::`, one scope per file, struct identity). This spec is the reference for imports, name scoping and `fn` declarations; README.md, grammar.md and semantics.md §10 summarize it. Not implemented: the items in §7.
Last Updated: 2026-11-14
Scope: the import unit, name scoping and qualification (`::`), `fn` declarations, struct naming across files, and how the compiler carries names from the loader to generated code.

## Contents

- [1. Summary](#1-summary)
- [2. Rationale](#2-rationale)
- [3. Rules](#3-rules)
  - [3.1 Files and import paths](#31-files-and-import-paths)
  - [3.2 Qualified names](#32-qualified-names)
  - [3.3 Scopes](#33-scopes)
  - [3.4 What an imported file may contain](#34-what-an-imported-file-may-contain)
  - [3.5 Definitions are fixed](#35-definitions-are-fixed)
  - [3.6 Imported names are read-only](#36-imported-names-are-read-only)
  - [3.7 Structs](#37-structs)
  - [3.8 Reserved names](#38-reserved-names)
- [4. Diagnostics](#4-diagnostics)
- [5. Implementation](#5-implementation)
  - [5.1 Loader and the name table](#51-loader-and-the-name-table)
  - [5.2 Internal names](#52-internal-names)
  - [5.3 Analyzer](#53-analyzer)
  - [5.4 Back end and runtime](#54-back-end-and-runtime)
  - [5.5 Interactions](#55-interactions)
- [6. Tests](#6-tests)
- [7. Deferred](#7-deferred)
- [8. Decisions](#8-decisions)

## 1. Summary

A **file** is the import unit and the scope boundary. Every top-level name of an imported file is reachable from the importer, always qualified with `::`: `fmt::show_vec`, `fmt::fmt_vec`, `geo::Point`. `module X:` blocks are named namespaces inside a file (`formats::csv::read`). Names are resolved per file, so two files can define the same name without affecting each other, and an importer can't redirect a library's internal calls.

`fn` declarations are fixed and hoisted: a name is declared once per scope and never reassigned, and every declaration of a scope exists before the scope's first statement runs. Variables holding functions (`g = fn(x) -> ...`) stay reassignable.

Everything is public; there are no access modifiers.

## 2. Rationale

- **No silent collisions.** When imports shared one global scope, an importer's `fn helper` replaced a library's `helper`, and the last-spliced definition won. Per-file scopes make that impossible, and every remaining clash inside a file is an error (rule 14).
- **No hidden code at import.** Imported files contain only definitions (rule 15), so importing never runs code and there is no initialization order.
- **Fixed functions are cheaper and checkable.** A declared function's signature never changes, so every call is checked against the real signature (call shapes), and a function that captures nothing compiles to a plain C++ function called directly.

## 3. Rules

### 3.1 Files and import paths

1. An imported `.qrk` file name, and every directory written in a relative or `std/` import path, must match `[a-z][a-z0-9_]*`. `incl './lib/geo_utils'` is valid; `incl './My Lib/geo-utils'` is a load error. The main script passed to `quark run` or `quark build`, and the directories of an absolute import path, are exempt.
2. Import paths are `./` and `../` relative paths, absolute paths, and `std/` for the stdlib. The `.qrk` extension is omitted.
3. `incl 'path'` binds the file under its **stem** (last path segment): `incl 'std/fmt'` binds `fmt`, `incl './lib/geo_utils'` binds `geo_utils`.
4. `incl 'path' as name` binds it under `name` instead.
5. Any file of definitions is a valid library; it doesn't need a `module` block.
6. A file imported along several paths (directly, transitively, through a symbolic link) is loaded once and has one identity: its real path, with symbolic links resolved.

### 3.2 Qualified names

7. `::` separates the parts of a qualified name:
   - `file::name`: a top-level name of an imported file (`fmt::fmt_vec`, `geo::Point`);
   - `file::module::name`: a member of a `module` block in an imported file (`formats::csv::read`);
   - `module::name`: a module of the same file (`mathx::square`), in an imported file or the main script.
8. **Primary module.** A module named like its file's stem is the file's primary module, and its members can also be written `file::name`: in `std/fmt.qrk`, `fmt::show_vec` means `fmt::fmt::show_vec`. Every stdlib file is one primary module, so stdlib calls are two-part (`io::open`, `fmt::show_vec`, `blas::dot`). The primary module is reached through the file's binding: `incl 'std/fmt' as f` gives `f::show_vec`.
9. A qualified name is valid wherever a name is: a value (`f = io::exists`), a call (`io::exists(p)`), a pipe target (`p | io::exists()`), a type annotation (`p: geo::Point`), a return type, and a struct literal (`geo::Point { x: 1, y: 2 }`).
10. `.` is only for fields and methods of values. `io.exists`, where `io` is an import binding, is an error with the hint `use io::exists`; `X.myfn()` where `X` is a module is `'X' is a module; use X::myfn`.
11. Imports are not re-exported. If `geo.qrk` imports `std/io`, an importer of `geo` can't write `geo::io::open`.

### 3.3 Scopes

12. Each file has its own scope. Inside a file, a name resolves, in order, to:
    1. a local (parameter, loop variable, `when` binding, local variable);
    2. a member of the enclosing module, inside a module body;
    3. a top-level name of the file: a definition, a module name, or an import binding;
    4. the prelude.

    Nothing from another file is visible unqualified. This includes extern methods, since a method call can't be qualified: `extern fn str.shout() str as 'h_shout'` in `lib.qrk` is called as `s.shout()` inside `lib.qrk` only, the importer and other imported files get `type 'str' has no method 'shout'`, and they reach it through a function of `lib.qrk` (`lib::shout(s)`, or `s | lib::shout()`). Two files may declare the same method without affecting each other.
13. Within a module body, the module's own members are written bare (`X::myfn` is also accepted). Other modules of the same file, and file-level code, write `module::name`.
14. Names never collide silently. Each of these is an error naming both definitions:
    - a name defined twice at one level (two functions, a struct and a function, an `extern fn` and a function, two modules);
    - a module member named like a top-level name of the same file;
    - an import binding (stem or `as` name) equal to a top-level name of the importing file, or to another import binding;
    - two definitions that would get the same internal name (§5.2), such as member `_x` of `module m` and member `x` of `module m_`;
    - an extern method declared twice for one receiver in a file, at the top level or in modules.

    An extern method can't be named like a prelude method of any type (`extern fn list.contains(x)` is rejected because of `str.contains`): the loader resolves method names without knowing a call's receiver type, so in that file a prelude method of the same name would become unreachable.

    Prelude builtins can't be redefined or assigned (`fn len`, `sum = 0`); parameters, loop variables and `when` bindings may shadow them.

### 3.4 What an imported file may contain

15. An imported file may contain only definitions, at the top level and in module bodies:
    - `fn` declarations, `struct` definitions and `module` blocks (not nested);
    - `incl` (top level only);
    - `extern '<header>'`, `extern fn`, `extern link` and `extern include_path` (directives at the top level only, `ffi_v0_1_spec.md` P1);
    - `name = <constant>`: an `int`, `float`, `str` or `bool` literal, unary minus on one, or `+ - * / %` over constants (`-1`, `60 * 60`). `list []` and `dict {}` are rejected, so a library has no mutable state.

    Any other statement, such as a call or `print`, is a load error naming the statement. References to an imported file's constants are replaced by the constant's value, so library functions don't capture them.
16. The main script has no such restriction. A `module` nested in another module is an error in every file.

### 3.5 Definitions are fixed

17. `fn f(...)` **declares** `f` in the current scope (a `FnDeclNode`); it is not an assignment.
18. Declaring a name already declared in the same scope is an error (`'f' is already defined on line 3`). This applies to `fn`, `struct`, `extern fn`, module members and constants alike.
19. Assigning to a declared function name, as a function or any other value, is an error: `'f' is a function declaration (line 1) and can't be reassigned; use a variable: g = fn ...`. The same applies to `struct` and `extern fn` names and to an imported file's constants.
20. A nested `fn` declaration may not reuse a name visible from an enclosing scope (an outer function, variable, parameter or builtin): `'f' is already a function in an enclosing scope (line 1); choose another name`. Parameters, loop variables and `when` bindings may shadow outer names.
21. Variables are unaffected: `g = fn(x) -> x + 1`, then `g = fn(x) -> 0`, is fine (as long as the signature stays the same, `architecture.md` §12). A function is a value: it can be passed, stored, returned and assigned to a variable. Only the name introduced by `fn` is fixed.
22. **Hoisting.** All `fn` declarations of a scope (the file together with its module bodies, a function body, a block) exist before any statement of that scope runs, so calls work in any order and mutual recursion needs no particular order. Variables are not hoisted: a function body only sees variables assigned above its declaration; anything else is `undefined identifier`.
23. **Definite assignment of captured variables.** A function that captures a variable must not be called before that variable is assigned. A direct call, or a chain of direct calls through declared functions or pipes, before the variable's first assignment in that scope is a compile error: `'report' uses 'total', which is assigned on line 2, after this call`. A call the analyzer can't order (through a function value) reads the variable through a cell that starts *unassigned*; reading it is a runtime error, `'total' is used before it is assigned`, never `null`.

### 3.6 Imported names are read-only

24. An importer can read and call every name of an imported file, but can't assign to it: `geo::x = 1` is `'geo::x' belongs to geo.qrk and can't be assigned here`. A library can't assign its own constants either, so every importer sees the same value.

### 3.7 Structs

25. A `struct` can be defined at a file's top level or inside a `module` block. A struct of an imported file is `file::Point` or `file::module::Point`.
26. Struct identity is the definition, not the name: `a::Point` and `b::Point` are different types at compile time and at runtime (§5.4).
27. Inside its file a struct is written bare (`Point { ... }`), or `module::Point` from outside its module. `geo.Point { ... }` is a parse error that points to `::`.
28. Diagnostics, `type()` and `print` show an imported struct qualified by its defining file's stem (`geo::Point`), a struct of a main-script module as `m::S`, and a main-script top-level struct bare (`Point`). The stem is used, not the importer's alias, because one struct may be imported under several aliases.

`structs_v0_1_spec.md` covers the rest of struct semantics.

### 3.8 Reserved names

A user name containing `__` is a load error (`'mathx__square': names can't contain '__', which is reserved for names the compiler generates`). Every internal name the compiler generates contains `__` (§5.2), so user code can never reach or clash with one.

## 4. Diagnostics

Name resolution happens in the loader, so most violations are load errors (`QK-LOAD-001`); the rules that need types or scopes of locals are check errors (`QK-CHECK-001`).

| Error | Code | Message |
|---|---|---|
| Bad file or directory name | `QK-LOAD-001` | `directory name 'My Lib' in import path './My Lib/geo' must be lowercase letters, digits and underscores (e.g. my_lib)` |
| Statement in an imported file | `QK-LOAD-001` | `'lib.qrk' is imported, so it can only contain definitions; line 3 is a statement (print(...))` |
| Non-constant value in an imported file | `QK-LOAD-001` | `'lib3.qrk' is imported, so it can only contain definitions; line 1 assigns 'X' a value that isn't a constant (an int, float, str or bool literal, or arithmetic on them)` |
| Import binding clash | `QK-LOAD-001` | `the function 'geo' (line 2) conflicts with import 'geo' on line 1; incl './geo' as other_name` |
| Unknown qualified name | `QK-LOAD-001` | `'fmt' (std/fmt) has no 'show_vector'; did you mean 'show_vec'?`; `module 'm' has no 'nope'` |
| Not re-exported | `QK-LOAD-001` | `imports are not re-exported: 'rex' imports 'io'; incl 'std/io' in this file` |
| `.` on an import binding | `QK-LOAD-001` | `'io' is an imported file; use io::exists` |
| `.` on a module name | `QK-LOAD-001` | `'X' is a module; use X::myfn` |
| `incl` of a same-file module | `QK-LOAD-001` | `incl m: a module in the same file needs no incl; write m::name directly` |
| Assignment to an imported name | `QK-LOAD-001` | `'geo::x' belongs to geo.qrk and can't be assigned here` |
| Duplicate declaration | `QK-LOAD-001` | `'f' is already defined on line 1`; `module 'a' is already defined on line 1` |
| Duplicate extern method | `QK-LOAD-001` | `extern method 'str.shout' is already declared on line 2` |
| Extern method named like a prelude method | `QK-LOAD-001` | `extern method 'list.contains': 'contains' is a prelude method (str.contains), so it can't be the name of an extern method of any type; choose another name` |
| Another file's extern method | `QK-CHECK-001` | `type 'str' has no method 'shout'` |
| Member/top-level clash | `QK-LOAD-001` | `module member 'csv::read' has the same name as the top-level function 'read' on line 1` |
| Internal-name clash | `QK-LOAD-001` | `'m_::x' and 'm::_x' (line 2) can't both be defined: the compiler would give them the same internal name; rename one of them` |
| Nested module | `QK-LOAD-001` | `module 'inner' is inside module 'outer'; modules can't be nested, so define it at the top level of the file` |
| Reserved name | `QK-LOAD-001` | `'mathx__square': names can't contain '__', which is reserved for names the compiler generates` |
| Reassigned declaration | `QK-CHECK-001` | `'f' is a function declaration (line 1) and can't be reassigned; use a variable: g = fn ...` |
| Shadowing `fn` declaration | `QK-CHECK-001` | `'f' is already a function in an enclosing scope (line 1); choose another name` |
| Redefined builtin | `QK-CHECK-001` | `'len' is a builtin function and can't be redefined or assigned; choose another name` |
| Call before a captured variable is assigned | `QK-CHECK-001` | `'report' uses 'total', which is assigned on line 2, after this call` |
| Unassigned read | `QK-RUNTIME-001` | `'total' is used before it is assigned` |
| Struct mismatch, same bare name | `QK-CHECK-001` / `QK-RUNTIME-001` | `argument 1 of 'f' expects g1::Point, got g2::Point` / `argument 'p' of 'f' expects g1::Point, got g2::Point` |
| `.` in a qualified struct literal | `QK-PARSE-001` | `a struct from a module or imported file is written g1::Point { ... }, with '::'` |

## 5. Implementation

The analyzer never sees `::` or `incl`: the loader resolves every name and hands the analyzer one tree of unique names.

### 5.1 Loader and the name table

`loader/files.go` and `loader/resolve.go`:

1. Load each file once, keyed by its real path, and give it an id in load order: the main script is file 0, imports are 1, 2, ... (deterministic for a given program).
2. Check file and directory names (rule 1) and the definitions-only rule (rule 15) for imported files.
3. Collect each file's top-level definitions and module members, and report the clash rules (rule 14) and reserved names (§3.8).
4. Rename every definition of an imported file, and every module member, to its internal name (§5.2). Resolve bare references by rule 12, with parameters and locals as shadows, and `binding::name` / `binding::module::name` against the imported file with the primary-module rule. Inline imported constants.
5. Splice the files into one tree and drop `incl`.
6. Give each extern method of a file its file's internal method name (§5.2, `collectMethods`) and rename the method calls of that file to it (`renameMethodCall`); calls of a method no other file of the call's file declares keep their written name.
7. Return a `names.Table` (package `names`) in `driver.Unit.Names`: internal name → `Origin{File, Module, Name, Kind}`, with `Kind` one of `Func`, `Struct`, `Extern`, `Value`, `Module`, `Local`, `Method`.

Everything user-facing goes through the table: `Table.Display(internal)` gives `geo::Point` or `fmt::show_vec`, and `Table.DisplayText(msg)` rewrites every internal name in a message. The analyzer gets the table with `SetNames`, applies `DisplayText` to every diagnostic, and passes it on in `Analysis.Names`; lowering uses it for `QFnInfo` and struct display names.

### 5.2 Internal names

| Definition | Internal name | Example |
|---|---|---|
| Top-level name of imported file N, stem `s` | `q<N>_s__name` | `q2_fmt__fmt_vec` |
| Member of a module of imported file N | `q<N>_s__module__name` | `q2_fmt__fmt__show_vec` |
| Main-script top-level name | unchanged | `helper` |
| Main-script module member | `module__name` | `mathx__square` |
| Local of an imported file named like any main-script name | `q<N>__name__l` | `q2__x__l` |
| Extern method of file N (main script: 0), for every receiver | `q<N>__name__m` | `q1__shout__m` |

The local rename exists because the spliced tree has one global scope in which main-script names are bare. Two definitions that would get the same internal name are a load error, never merged. The back end prefixes internal names with `quark_` for functions and variables (`quark_q2_lib2__sq`), emits `_qfi_<internal>` for each `QFnInfo` and `_sdef_quark_<internal>` / `_sfields_quark_<internal>` for each struct. Names the compiler generates itself (lambdas, nested declarations, duplicate variables) start with `quark___`.

### 5.3 Analyzer

- **Symbols.** Every variable, parameter, loop variable, `when` binding and `fn` declaration is one `types.Symbol`; `Analysis.Refs` maps every resolved identifier node to it, and `finalizeSymbols` decides its storage and C++ name once (`architecture.md` §1.6).
- **Fixed declarations** (`types/analyzer_fixed.go`): `declareFixedFns` declares a scope's `fn` declarations as non-mutable symbols before its statements and checks rules 18 and 20; rule 19 is checked at assignments and typed declarations. Signatures (parameter types, annotations, defaults) are known before any body is analyzed, so forward and recursive calls see the real `FunctionType`, including `ParamCalls`.
- **Definite assignment** (rule 23): `checkDefiniteAssignment` orders statements per statement list, follows direct calls through declared functions transitively, and records each `HoistScope`: the cells lowering creates unassigned and the closures it creates at the scope's start. The top level and module bodies are one hoisting unit, since they all run in `main`.
- **Module bodies** are analyzed in the file scope (their members already have unique internal names), and their declarations are hoisted with the top level. Structs are predeclared per file, including those in module bodies.
- **Externs** are registered under their internal names, which are unique per file, so an `extern fn` is scoped to its file like any other definition. Extern methods are registered by receiver type and internal method name, so a call finds only the methods of its own file (rule 12).
- **Dispatch** (`finalizeFixedFns`, once the program is analyzed): a declaration that captures nothing, directly or through another declaration, is a plain C++ function, called with `DispatchDirect` or, when fully annotated, `DispatchNative`, and never captured. A declaration that captures stays a closure in a binding that never changes, called with `DispatchClosure`.

### 5.4 Back end and runtime

- `hoistScopes` creates every scope's closures at the scope's start; variables they capture start as unassigned cells (`q_new_unassigned_cell`, read with `q_cell_get`, `core/cell.hpp`).
- One `QStructDef` is emitted per struct definition, and `QStructDef.name` holds the display name. `q_param_struct(v, &_sdef_X, fn, param)` and `q_field_struct` compare `QStructDef` pointers, so pointer identity is type identity.
- `QFnInfo.name` is the display name, so runtime messages read `function 'io::exists' expects 1 argument but got 0`.

### 5.5 Interactions

- **Call shapes** (`types/analyzer_callshapes.go`): fixed declarations mean a function name's `FunctionType` never changes, and hoisting gives every call its real signature.
- **FFI:** `extern link` and `extern include_path` are collected across all files (`ffi_v0_1_spec.md` P1). C++ symbols in glue headers are global: two glue headers that define the same C++ function clash in C++. That is a rule for glue authors, not a Quark scoping matter.
- **Editor extension:** highlights `::` paths, completes `name::` and `file::module::`, and hovers qualified functions; `internal/editordata` lists stdlib members as `fmt::show_vec`.

## 6. Tests

- `loader/modules_v02_test.go`: resolution, display names, primary modules, file ids and identity, symlinks, locals of imported files, and the rejected programs (`TestModulesV02_*`).
- Integration: `TestModuleNamespacing`, `TestNamesArePerFile`, `TestStructsAcrossFiles`; smoke programs `smoke_modules_success.qrk`, `smoke_modules_error_graph.qrk`, `smoke_modules_error_resolve.qrk`, `smoke_module_calls_in_fns.qrk` and `smoke_fn_declarations.qrk`.
- `TestSemanticsSafety_Rejected` holds the fixed-declaration, hoisting and definite-assignment cases (including the runtime unassigned read and the reassigned native function).

## 7. Deferred

- **Selective imports:** `incl 'std/fmt' (show_vec, table)`.
- **Marking internal names:** documentation comments that mark a name as internal, shown by the editor and possibly warned about at use (`fmt::fmt_vec`).
- **Renaming one module of a multi-module file on import.**
- **Re-exports and facade modules.**
- **Mutable module-level state.**
- **Resolution without renaming:** the analyzer already resolves identifiers to symbols (§5.3), but the loader still renames strings and diagnostics go through `DisplayText`. Resolving `::` names to symbols directly would remove the renaming pass and the string-keyed tables.

Known gaps (`known_issues.md`): a call written above a `fn` declaration is checked less than one below it; a syntax error in an imported file is reported at the importer's `incl` line.

## 8. Decisions

Decided 2026-11-12, before implementation.

| Decision | Choice |
|---|---|
| Import unit | The file |
| Visibility | Every top-level name and module member of an imported file is reachable, qualified; no access modifiers |
| Qualifier | `::` everywhere: `file::name`, `file::module::name`, same-file `module::name`; the primary module is also `file::name` |
| `module` blocks | Kept, as namespaces inside a file; several per file, not nested |
| Self-qualification in a module | Allowed (`X::f` inside `X`) |
| File names | `[a-z][a-z0-9_]*`, for imported files and the directories of relative and `std/` paths |
| Flat imports | Removed |
| `fn` declarations | Fixed and hoisted: no second declaration in a scope, no reassignment, no shadowing by a nested `fn` declaration |
| Function variables | Reassignable, as any variable |
| Imported files | Definitions and scalar constants only (no `list []`/`dict {}`) |
| Capturing declared functions | Stay closures in a fixed binding; only non-capturing ones are direct calls |
| Variables used by a function body | Must be assigned above the declaration (variables are not hoisted) |
| `.` on a module or import name | Error suggesting `::` |
| Imported names | Read-only to the importer and to the library itself |
| Struct identity | The definition (`QStructDef` pointer at runtime, internal name at compile time), shown qualified by the file's stem |
| Internal names | `q<N>_<stem>__[<module>__]<name>`, emitted as `quark_...`; main-script names unchanged; `__` reserved in user names |
