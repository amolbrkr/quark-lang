# Quark Modules v0.2 Spec

Status: Draft, not implemented.
Last Updated: 2026-11-12
Scope: import unit, name scoping and qualification (`::`), function declarations, struct naming across files, and the symbol metadata the compiler carries from front end to back end. Supersedes the module and import rules in README.md ("Modules and imports"), grammar.md and semantics.md §10 once implemented.

## 1. Summary

A **file** is the import unit and the scope boundary. Every top-level name in an imported file is reachable from the importer, always qualified with `::`: `fmt::show_vec`, `fmt::fmt_vec`, `geo::Point`. `module X:` blocks stay as named namespaces inside a file (`formats::csv::read`, `formats::json::parse`). Names are resolved per file, so two files can define the same name without affecting each other, and a library's internal calls can't be redirected by the code that imports it.

`fn` declarations become fixed, hoisted definitions: a name can be declared once per scope and never reassigned. Variables holding functions (`g = fn(x) -> ...`) stay reassignable.

Everything is public. There are no access modifiers. Discouraging direct use of internal names such as `fmt::fmt_vec` is left to a future documentation feature (§9).

## 2. Problems This Fixes

Today an import splices the imported file's top-level statements into the importing program's tree, and only members of `module` blocks are renamed (`module__member`, `loader/namespace.go`). Everything else shares one global scope. Observed behavior (2026-11-11):

| Situation | Result today |
|---|---|
| Library has top-level `fn helper`, used by its module; importer also defines `fn helper` | The library's own calls run the importer's `helper` (`l.api(1)` returned 3, not 102) |
| Library and importer both assign a top-level variable | One shared variable |
| Importer defines `fn io_open` while `std/io` declares `extern fn io_open` | The importer's function is ignored; calls go to the extern (externs live in the builtins table, which the analyzer checks first) |
| Two libraries each contain `module utils` | `module 'utils' already defined`, even with different aliases |
| Two files define `struct Point` | `struct 'Point' already defined`; struct names are global |
| A struct is used from another file | No qualified type syntax (`q: geo.Point` is a parse error); literals and annotations use the bare global name (`Point { ... }`) |
| `incl 'std/fmt' as fmt` | `fmt_vec`, `blas_dot` etc. are callable unqualified, colliding with user names |
| An imported file has top-level statements (`print(...)`) | They run inside the importer's program, at the `incl` |
| `fn f` declared twice | The second silently wins |
| `fn f`, then `f = fn(x) -> 0` | Silently replaced; for a fully annotated `f`, the program fails to compile with a C++ error (`redefinition of 'quark_f'`) |
| `fn a() -> b()`, `a()`, then `fn b` | Runtime failure: `b` is bound only when its line runs |
| Runtime struct parameter check (`q_param_struct`) | Compares struct *names* (`strcmp`), so two different structs named `Point` would pass each other's check once struct names are no longer global |

## 3. Rules

### 3.1 Files and import paths

1. A `.qrk` file name, and every directory name in an import path, must match `[a-z][a-z0-9_]*`: lowercase letters, digits and underscores, starting with a letter. `incl './lib/geo_utils'` is valid; `incl './My Lib/geo-utils'` is a load error. The rule applies to files that are imported; the main script passed to `quark run` is exempt.
2. Import path forms are unchanged: `./` and `../` relative paths, absolute paths, and `std/` for the stdlib. The `.qrk` extension is omitted.
3. `incl 'path'` binds the file under its **stem** (last path segment): `incl 'std/fmt'` binds `fmt`, `incl './lib/geo_utils'` binds `geo_utils`.
4. `incl 'path' as name` binds it under `name` instead.
5. An imported file no longer has to contain a `module` block. A file of top-level definitions is a valid library.
6. A file imported along several paths (directly and transitively) is loaded once and has one identity, keyed by its absolute path.

### 3.2 Qualified names

7. `::` separates the parts of a qualified name. The forms are:
   - `file::name`: a top-level name of an imported file (`fmt::fmt_vec`, `geo::Point`).
   - `file::module::name`: a member of a `module` block in an imported file (`formats::csv::read`).
   - `module::name`: inside the file that defines the module (`csv::read`), including same-file modules in a main script (`mathx::square`).
8. **Primary module.** A module whose name equals its file's stem is the file's primary module. Its members can also be written `file::name`: in `std/fmt.qrk`, `fmt::show_vec` means `fmt::fmt::show_vec`. Every stdlib file is a single primary module, so stdlib calls stay two-part (`io::open`, `fmt::show_vec`, `blas::dot`). The primary module is reached through the file's binding, so `incl 'std/fmt' as f` gives `f::show_vec`.
9. A qualified name is valid wherever a name is:
   - a value: `f = io::exists`;
   - a call: `io::exists(p)`;
   - a pipe target: `p | io::exists()`;
   - a type annotation: `p: geo::Point`;
   - a struct literal: `geo::Point { x: 1, y: 2 }`.
10. `.` is only for fields and methods of values. `io.exists` where `io` is an import binding is a compile error with the hint `incl io::exists`.
11. Imports are not re-exported. If `geo.qrk` imports `std/io`, an importer of `geo` can't write `geo::io::open`.

### 3.3 Scopes

12. Each file has its own scope. Inside a file, a name resolves, in order, to:
    1. a local (parameter, loop variable, `when` binding, local variable);
    2. a member of the enclosing module, when inside a module body;
    3. a top-level name of the file: a definition, a module name, or an import binding;
    4. the prelude.

    Nothing from another file is visible unqualified.
13. Within a module body, the module's own members are written bare. Other modules of the same file are written `module::name`. File-level code reaches a module's members as `module::name`.
14. Names never collide silently. Each of these is a compile error naming both definitions:
    - a module member with the same name as a top-level name of the same file;
    - an import binding (stem or `as` name) equal to a top-level name of the importing file, or to another import binding in it;
    - two modules with the same name in one file.

### 3.4 What an imported file may contain

15. A file that is imported may contain only definitions, at the top level and inside module bodies:
    - `fn` declarations;
    - `struct` definitions;
    - `module` blocks;
    - `incl` (top level only);
    - `extern`, `extern fn`, `extern link` and `extern include_path` (with their existing placement rules);
    - `name = <constant expression>`, using the same constant expressions as struct field defaults.

    Any other statement, such as a call or `print`, is a load error naming the statement. Importing a file therefore never runs code, and there is no initialization order to define.
16. The main script (the file passed to `quark run` or `quark build`) has no such restriction.

### 3.5 Definitions are fixed

17. `fn f(...)` **declares** `f` in the current scope; it is no longer sugar for the assignment `f = fn(...)`.
18. Declaring a name that is already declared in the same scope is a compile error (`'f' is already defined on line 3`). This applies to `fn`, `struct`, `extern fn`, module members and top-level constants alike.
19. Assigning to a declared function name is a compile error (`'f' is a function declaration and can't be reassigned; use a variable: g = fn ...`). The same applies to `struct` and `extern fn` names, and to top-level constants of imported files.
20. A `fn` declaration may not reuse a name visible from an enclosing scope. Parameters, loop variables and `when` bindings may shadow outer names, as today. (Restrictive on purpose; relaxing it later is backward compatible.)
21. Variables are unchanged: `g = fn(x) -> x + 1` then `g = fn(x) -> 0` is fine. A function is still a value: it can be passed, stored, returned, and assigned to a variable. Only the name introduced by `fn` is fixed.
22. **Hoisting.** All `fn` declarations of a scope exist before any statement of that scope runs, so calls in any order and mutual recursion work. `checkCallsBeforeDefinition` is replaced by rule 23.
23. **Definite assignment of captured variables.** A function that captures a variable must not be called before that variable is assigned. The analyzer rejects a call when it can see the order: a direct call, or a chain of direct calls through hoisted functions, at a point before the variable's first assignment in that scope (`'report' uses 'total', which is assigned on line 9, after this call`). A call it can't order statically (through a function value) reads the variable through a cell that starts *unassigned*, and reading it is a runtime error (`'total' is used before it is assigned`), not `null`.

### 3.6 Imported names are read-only

24. An importer can read and call every name of an imported file, but can't assign to it: `fmt::x = 1` is a compile error. A library's top-level values are constants (rule 15), so all importers see the same value.

### 3.7 Structs

25. A `struct` can be defined at a file's top level or inside a `module` block. A struct in an imported file is named `file::Point` or `file::module::Point`.
26. Struct identity is the definition, not the name: `a::Point` and `b::Point` are different types at compile time and at runtime (§5.3).
27. Inside its defining file, a struct is written bare (`Point { ... }`), or `module::Point` from outside its module.
28. Diagnostics, `type()` and `print` show an imported struct's name qualified by its defining file's stem (`geo::Point`) and a main-script struct bare (`Point`). The runtime uses the stem, not the importer's alias, because one struct may be imported under different aliases.

### 3.8 Removed

- **Flat imports.** `incl 'std/fmt'` no longer puts `show_vec` (or anything else) into the importer's scope; it binds `fmt`. Selective imports may come later (§9).
- **`incl` of a same-file module** (`incl mathx as m`). Same-file modules are written `mathx::name` directly.
- **Global module names.** Module identity is (file, module name).
- **`.` for module access** (rule 10).
- **The requirement that an imported file define a module** (rule 5).

## 4. Diagnostics

| Error | Stage / code | Message (shape) |
|---|---|---|
| Bad file or directory name | load, `QK-LOAD-001` | `module file name 'My Lib' must be lowercase letters, digits and underscores (e.g. my_lib)` |
| Statement in an imported file | load, `QK-LOAD-001` | `'lib.qrk' is imported, so it can only contain definitions; line 3 is a statement (print(...))` |
| Import binding clashes | check, `QK-CHECK-001` | `import 'geo' (line 2) conflicts with the function 'geo' defined on line 5; incl 'path' as other_name` |
| Unknown qualified name | check, `QK-CHECK-001` | `'fmt' (std/fmt) has no 'show_vector'`, with a closest-match hint when one exists |
| `.` on an import binding | check, `QK-CHECK-001` | `'io' is an imported file; use io::exists` |
| Assignment to imported name | check, `QK-CHECK-001` | `'fmt::x' belongs to std/fmt and can't be assigned here` |
| Duplicate declaration | check, `QK-CHECK-001` | `'f' is already defined on line 3` |
| Reassigned declaration | check, `QK-CHECK-001` | `'f' is a function declaration and can't be reassigned; use a variable: g = fn ...` |
| Shadowing `fn` declaration | check, `QK-CHECK-001` | `'f' is already a function in an enclosing scope (line 1); choose another name` |
| Member/top-level clash in a file | check, `QK-CHECK-001` | `module member 'csv::read' has the same name as the top-level 'read' on line 4` |
| Call before captured variable is assigned | check, `QK-CHECK-001` | `'report' uses 'total', which is assigned on line 9, after this call` |
| Unassigned read at runtime | runtime, `QK-RUNTIME-001` | `'total' is used before it is assigned` |
| Struct type mismatch, same bare name | check / runtime | `expects geo::Point, got shapes::Point` |

error_codes.md gets the new `QK-LOAD-001` cases.

## 5. Implementation Notes

### 5.1 Symbol metadata, front end to back end

Today identity is the *name string* at every stage:
- the loader renames strings (`module__member`);
- the analyzer keys `functions`, `nativeFns`, `externFns`, `structTypes` and scopes by string;
- codegen keys `funcNames`, `declaredVars` and `nativeFns` by string and derives C++ names with `sanitizeVarName(name)`;
- the runtime compares struct names with `strcmp`.

That is why per-module renaming had to happen before analysis, and why the runtime struct check is unsound across files. This spec makes identity explicit.

**Phase A (with this spec): the loader keeps renaming, and also produces a name table.**
- The loader assigns each loaded file an id and renames every definition of an imported file to its internal name (§5.2). It rewrites every reference, including `alias::name` and `alias::module::name` paths, to that internal name, so the analyzer and codegen still work on unique strings.
- It returns a `NameTable` in `driver.Unit`:

  ```go
  type NameOrigin struct {
      File    *SourceFile // id, absolute path, stem
      Module  string      // "" for a top-level name
      Name    string      // as written: "show_vec"
      Kind    NameKind    // Func, Struct, Extern, Value, Module
      Def     *ast.TreeNode
  }
  type NameTable map[string]*NameOrigin // internal name -> origin
  ```

- The analyzer and codegen use the `NameTable` for every user-facing string: diagnostics (`fmt::show_vec`, `geo::Point`), `QFnInfo.name`, `QStructDef` display names and call-shape messages. Display helpers that today strip `module__` (`displayMember`, `displayFuncName`, `displayName` in `types/analyzer_callshapes.go`) are replaced by one `NameTable.Display(internal)` lookup.
- The analyzer gets `Unit.Names` at construction, and `types.Analysis` carries it on to codegen (`Analysis.Names`), following the rule that analyzer outputs go in `Analysis`.

**Phase B (later): symbols instead of strings.** The analyzer resolves each identifier and qualified-name node to a `SymbolID` (`Analysis.Resolutions map[*ast.TreeNode]SymbolID`, `Analysis.Symbols []SymbolInfo`). `SymbolInfo` holds the origin, type, kind, C++ name and fixedness. Codegen emits `SymbolInfo.CName` instead of deriving names from strings. This removes the loader's renaming pass and the string-keyed maps. It is not required for the behavior in this spec, but every new piece of code written for Phase A should look names up through one function, so that Phase B is a change in one place.

### 5.2 Mangling

- **File ids** are assigned in load order (deterministic for a given program): the main script is file 0, imports are 1, 2, …
- **Internal names:**
  - top-level name of an imported file: `q<N>_<stem>__<name>` (`q2_fmt__fmt_vector`);
  - module member: `q<N>_<stem>__<module>__<name>` (`q2_fmt__fmt__show_vec`);
  - main script: names stay as today (`<name>`, `<module>__<name>`), so generated code for single-file programs is unchanged.
- **C++ names** follow the existing conventions: `quark_` plus the internal name for functions and variables (`quark_q2_fmt__show_vec`), `_qfi_` plus it for `QFnInfo`, and `_sfields_`/struct-def statics plus it for structs.
- **Hygiene** (a user identifier spelled like an internal name) is not addressed in v0.2.

### 5.3 Runtime struct identity

- `QStructDef` gets a `display_name` (`"geo::Point"`, from the `NameTable`); `name` becomes the internal name. `repr.hpp` (struct and table printing) and `conversion.hpp` (`type()`) use `display_name`.
- `q_param_struct(v, structName, fn, param)` becomes `q_param_struct(v, const QStructDef* def, fn, param)` and compares `v.data.struct_val->def == def`. Codegen passes `&_sdef_<internal>`. One `QStructDef` is emitted per struct definition, so pointer identity is type identity.
- Any other runtime code comparing struct names must compare definitions instead. Today `params.hpp` is the only one.
- The analyzer's `StructType` keys `structTypes` by internal name and carries the display name, so compile-time messages read `expects geo::Point, got shapes::Point`.

### 5.4 Lexer and parser

- **Lexer:** a `COLONCOLON` token for `::`. Check that `:` keeps working in annotations, dict literals and blocks; `x: int` and `dict { a: b }` must not lex as `::`.
- **Parser:** a `QualifiedNameNode` (`ID ('::' ID)+`) in expression position, in `parseTypeExpr` (and `isTypeToken`) for annotations, and before `{` as a struct literal. `Q::...::Name {` is always a struct literal, as `Name {` already is (the parser has no struct-name pre-pass).
- **Parser:** `struct` definitions inside `module` bodies.
- **Parser:** `fn f` stops desugaring to `f = fn(...)`. It produces a declaration node (`FunctionNode` already exists, and codegen and the analyzer handle it) that the analyzer treats as fixed. Anonymous lambdas are unchanged.

### 5.5 Loader

- Validate file and directory names (rule 1) when resolving each import.
- Load each file once by absolute path, and assign file ids.
- Enforce the definitions-only rule (rule 15) on imported files, at the top level and in module bodies.
- Replace `namespaceModules` with a renamer that, per file:
  1. collects the file's top-level names and module members;
  2. renames definitions to internal names;
  3. resolves bare references by rule 12, using parameters and locals as shadows, as today;
  4. resolves `binding::name` and `binding::module::name` against the imported file, with the primary-module rule;
  5. reports the clash rules (rule 14) and unknown qualified names.
- Remove: flat-import handling (`bare` map), the same-file `incl module` form, the global-module-name check, and the "does not define a module" error.
- Emit the `NameTable`.

### 5.6 Analyzer

- **Declarations:** `FunctionNode` declarations are fixed symbols (`Mutable: false`). Duplicate declarations, assignment to declared names, and shadowing `fn` declarations are errors (rules 18–20). Assignment to imported names is an error (rule 24). These checks live where `analyzeOperator` handles `=` on an existing symbol.
- **Hoisting:** analyze all `fn` signatures of a scope (parameter types, annotations, defaults) before any body, then the bodies. Recursive and forward calls then see the real `FunctionType`, including `ParamCalls`, instead of a placeholder. Call-shape checks apply to them, and the `nat` C++ redefinition failure becomes impossible.
- **Definite assignment** (rule 23): per scope, record the statement index of each variable's first assignment, and each hoisted function's captured variables (`collectFreeVars`), transitively through direct calls. Reject a direct call at an earlier index.
- **Externs** stop being registered in the global builtins table. They become ordinary file-scoped fixed symbols: still `DispatchExtern` call plans, but resolved by scope, so an extern can't shadow or be shadowed across files.
- **Structs** are predeclared per file, including inside module bodies, and keyed by internal name.
- **The `.`-on-import error** (rule 10) lives where `alias.member` is resolved today (`lookupModuleCall`, `moduleMemberRefs`). Those paths are replaced by `QualifiedNameNode` handling.

### 5.7 Codegen and runtime

- **Fixed functions** are constants. A call to a declared function's name is always `DispatchDirect` or `DispatchNative` (`quark_<internal>(nullptr, ...)`), and closures don't capture declared functions. The `collectFreeVars` exclusion for native functions extends to every fixed declaration, which removes the cell, the indirect call and the closure allocation for named functions.
- **Unassigned cells** (rule 23): `predeclareMainFunctionBindings` initializes captured variables to an unassigned sentinel, not `qv_null()`. Reading one goes through a checked read that reports the variable's name.
- **`QFnInfo.name` and `QStructDef.display_name`** come from the `NameTable`.

### 5.8 Interactions

- **Call shapes** (`types/analyzer_callshapes.go`) become stronger. Fixed declarations mean a function name's `FunctionType` never changes, and hoisting gives every call its real signature.
- **`QFnInfo`** (runtime arity checks) is unchanged, apart from qualified display names.
- **Native functions:** reassigning a fully annotated function can no longer break the C++ compile, because reassignment is rejected (rule 19).
- **FFI:** `extern link` and `extern include_path` placement is unchanged. C++ symbols in glue headers remain global: two libraries whose headers define the same C++ function still clash in C++. That is a constraint on glue authors (ffi_v0_1_spec.md), not a Quark scoping matter.
- **Editor extension:** the grammar recognizes `::` paths and qualified type names. `internal/editordata` stdlib data lists members as `fmt::show_vec`, and snippets use `::`.

## 6. Migration

| Area | Change |
|---|---|
| Stdlib (`src/core/stdlib/*.qrk`) | None expected (each file is one primary module, with definitions only). Verify none reassigns a declared function |
| Programs using stdlib modules | `io.exists(p)` becomes `io::exists(p)`; `p \| io.exists()` becomes `p \| io::exists()` |
| Flat imports | `incl 'std/fmt'` + `show_vec(x)` becomes `fmt::show_vec(x)`. `src/testfiles/test.qrk`: `fmt_vec(x, len(x), true)` becomes `fmt::show_vec(x)`, `blas_dot(...)` becomes `blas::dot(...)` |
| Same-file modules | `incl mathx as m` + `m.square(9)` becomes `mathx::square(9)` (README "Same-file module" example, smoke tests) |
| `src/testfiles/lib/core.qrk` (three modules) | Stays; importers write `core::helpers::double`. `smoke_modules_error_graph.qrk` (unaliased collision of `collision_a`/`collision_b`) is rewritten: that collision can no longer happen, so the test checks the new clash rules instead |
| `smoke_modules_success.qrk` and other module smoke tests | Rewritten for `::`, file bindings and no flat imports |
| Code that reassigns a declared function or declares one twice | Becomes a compile error; use a variable |
| Docs | README (Modules and imports, Known limitations: drop the clash and struct-namespacing entries), grammar.md, semantics.md §10, stdlib.md (module sections, "what an import exposes"), CLAUDE.md (Loader row, Naming Conventions, Stdlib modules), error_codes.md |

## 7. Tests

Every rule gets a negative test (CLAUDE.md, Robustness Bar). At minimum:

**Positive:**
- Two imported files both define `helper`, `Point` and `module utils`, and the importer uses both pairs.
- A library's internal call to its `helper` is unaffected by the importer's own `helper`.
- Primary-module access (`fmt::show_vec`), `file::module::name`, and `as`.
- `geo::Point` in an annotation, a struct literal and a runtime parameter check.
- Hoisted mutual recursion in any order.
- Variables holding functions are reassignable.
- A file imported twice (directly and transitively) has one identity: its struct is the same type on both paths.

**Negative**, one per row of §4, plus:
- **Runtime struct identity:** a value of `a::Point` passed through an untyped path to a parameter annotated `b::Point` is rejected at runtime. This is the `strcmp` bug, and the test must fail on today's runtime.
- **Unassigned read:** reading a captured variable through a function value before it is assigned is a runtime error, not `null`.
- **Reassigned native function:** reassigning a fully annotated function is a Quark compile error, not a C++ error.

## 8. Rollout

1. **Lexer/parser:** `::`, `QualifiedNameNode`, structs in modules, `fn` as a declaration. Parse-level tests.
2. **Loader:** file ids, name validation, definitions-only rule, the renamer and `NameTable`, clash rules. Loader unit tests (`loader/loader_test.go`).
3. **Analyzer:** fixed declarations, hoisting, definite assignment, read-only imports, externs as scoped symbols, structs keyed by internal name, qualified display names.
4. **Codegen/runtime:** C++ names from internal names, `QStructDef` identity and display names, unassigned cells, direct dispatch for fixed functions.
5. **Migration of tests and docs** (§6); editor extension.

Steps 1–4 land together for module syntax: there is no useful intermediate state where `::` parses but doesn't resolve. Fixed `fn` declarations (rules 17–23) are independent of modules and could land first.

## 9. Deferred

- **Selective imports:** `incl 'std/fmt' (show_vec, table)`.
- **Discouraging internal names:** documentation comments that mark a name as internal, shown by the editor and possibly warned about at use (`fmt::fmt_vec`).
- **Renaming one module of a multi-module file on import.**
- **Re-exports and facade modules.**
- **Mutable module-level state.**
- **Mangling hygiene** (rejecting user identifiers that look like internal names).
- **Phase B symbol IDs** (§5.1).

## 10. Decisions

| Decision | Choice |
|---|---|
| Import unit | The file |
| Visibility | Every top-level name and module member of an imported file is reachable, qualified; no access modifiers |
| Qualifier | `file::name`, `file::module::name`; the primary module (named after its file) is also `file::name` |
| `module` blocks | Kept, as namespaces inside a file; several per file allowed |
| File names | `[a-z][a-z0-9_]*`, for files and directories in import paths |
| Flat imports | Removed |
| `fn` declarations | Fixed and hoisted: no second declaration in a scope, no reassignment, no shadowing by another `fn` declaration |
| Function variables | Reassignable, as any variable |
| Imported files | Definitions and constant values only |
| Imported names | Read-only to the importer |
| Struct identity | The definition (pointer at runtime, internal name at compile time), shown qualified |
| Mangling | `q<N>_<stem>__[<module>__]<name>`, emitted as `quark_...`; main-script names unchanged |
