# Quark REPL v0.1 Spec

Status: Proposal; nothing is implemented. Open: the line-editing dependency (§8, decision 11).
Last Updated: 2026-11-13
Scope: `quark repl`, an interactive session that compiles each input with the normal compiler and runs it in a long-lived process. Notebooks and Jupyter kernels are out of scope (§7).

## Contents

- [1. Summary](#1-summary)
- [2. Rationale](#2-rationale)
- [3. Behaviour](#3-behaviour)
  - [3.1 A session is a file that grows](#31-a-session-is-a-file-that-grows)
  - [3.2 Inputs that fail](#32-inputs-that-fail)
  - [3.3 Showing values](#33-showing-values)
  - [3.4 Changing a variable's type](#34-changing-a-variables-type)
  - [3.5 Empty lists and dicts](#35-empty-lists-and-dicts)
  - [3.6 Entering input](#36-entering-input)
  - [3.7 Imports and native code](#37-imports-and-native-code)
  - [3.8 Differences from a file](#38-differences-from-a-file)
- [4. Diagnostics](#4-diagnostics)
- [5. Implementation](#5-implementation)
  - [5.1 Processes](#51-processes)
  - [5.2 The runtime library and compiling an input](#52-the-runtime-library-and-compiling-an-input)
  - [5.3 Front end: re-analysing the committed inputs](#53-front-end-re-analysing-the-committed-inputs)
  - [5.4 Lowering and emission in session mode](#54-lowering-and-emission-in-session-mode)
  - [5.5 Line input](#55-line-input)
  - [5.6 Platforms](#56-platforms)
- [6. Tests](#6-tests)
- [7. Deferred](#7-deferred)
- [8. Decisions](#8-decisions)

## 1. Summary

`quark repl` opens a prompt. Each input (one line, a block, or a paste) goes through the same pipeline as `quark run`, is compiled into a shared library, and runs inside a host process that keeps the session's variables, functions and structs between inputs.

A session follows the rules of one file that grows by one input at a time: the same type checks, the same scoping, the same errors. The few differences are listed in §3.8.

```
$ quark repl
Quark REPL. Ctrl-D exits.
> xs = list []
> for i in range(5):
.     xs.push(i * i)
.
> xs
[0, 1, 4, 9, 16]
> fn sq(n: int) int -> n * n
> sq(xs[4])
256
> print(xs[9])
error[QK-RUNTIME-001] (runtime): list index 9 out of range (list length 5); use .get(i) when the index may be past the end at <input 6>:1:6
> xs.push('a')
error[QK-CHECK-001] (check): cannot add str to list[int]: a list keeps the element type it was created with (set by its first write, on line 2 of <input 2>); create it as 'xs: list = list [...]' to hold mixed values at <input 7>:1:9
```

## 2. Rationale

A REPL needs three things `quark run` doesn't: state that survives between inputs, compiling an input that refers to earlier ones, and errors that don't end the session.

| Approach | Verdict |
|---|---|
| Replay: concatenate all inputs, recompile and rerun everything for each new input | Rejected. Every input reruns every earlier side effect and re-reads every data file, and the cost grows with the session. |
| An interpreter for the REPL | Rejected. A second implementation of the language would drift from compiled programs. |
| A C++ JIT (`clang-repl`, Cling) fed with the generated C++ | Rejected. The GC doesn't scan the JIT's memory for roots, it adds a second C++ code path, and it needs a clang tool not every install has. |
| **A host process that keeps the state, plus one shared library per input** | **Chosen.** The same compiler, lowering and clang as `quark run`; only the program's entry point and where top-level variables live change. |

Measured on 2026-11-13 (clang 22, linux-amd64):

- The front end (lexing to emitted C++) takes 6-10 ms for a smoke program; the rest of `quark run` is clang.
- With the runtime compiled once (§5.2), compiling an input at `-O0` takes 0.06 s (19 lines of C++) to 0.10 s (486 lines), and linking it into a shared library about 0.13 s. At `-O1` the compile takes 0.27-0.96 s.
- Building the runtime once takes about 10 s; it is cached.
- Code compiled at `-O0` calling the `-O3` runtime: numeric loops ran 5-11 times slower than at `-O3`, list, string and vector work about 1.4 times slower.

## 3. Behaviour

### 3.1 A session is a file that grows

1. Each input is analysed as the next part of one file. Every rule of a file applies: types and element types, scoping by text order (semantics.md), a name defined twice (`QK-LOAD-001`, also for `fn` and `struct`), variables keeping their type (except §3.4), imports.
2. `fn` declarations are hoisted within their input, not across inputs. A function can call a function defined later in the same input, but not one from a later input: when its input is compiled, that name is undefined. The same holds for a variable a function uses. Pasting a whole script as one input behaves exactly as the file does.
3. An input counts only if it compiles and runs to its end (§3.2).
4. Locations name the input: `<input N>:line:col`, inputs numbered from 1, lines counted within the input. A message that refers to another line (`already defined on line 1`) names its input too (`on line 1 of <input 3>`).

### 3.2 Inputs that fail

| What happens | Shown | The session afterwards |
|---|---|---|
| Compile error | The diagnostics, as for a program | Unchanged; nothing ran |
| Runtime error | `error[QK-RUNTIME-001] ...`, as in a program | As it was before the input |
| Ctrl-C while the input runs | `interrupted` | As it was before the input |
| A crash (a signal, usually from native code) | `QK-REPL-001` | As it was before the input |

"As it was before the input" means every variable has the value it had before, and names the input introduced don't exist; the input can be typed again. Output the input printed and files it wrote stay: effects outside the process aren't undone.

```
> n = 1
> n = 2                // these two lines pasted as one input
  print(1 / 0)
error[QK-RUNTIME-001] (runtime): division by zero at <input 2>:2:6
> n
1
```

### 3.3 Showing values

If the last statement of an input is a bare expression that has a value, the REPL prints the value as `print` does. An expression of type `void` (`print(x)`, `xs.push(1)`) shows nothing; a `null` value shows `null`. Bare expressions before the last statement print nothing, as in a file.

### 3.4 Changing a variable's type

1. A top-level assignment in a later input may give a variable from an earlier input a value of another type. From that statement on, the variable has the new type, and the compiler warns (`QK-CHECK-002`).
2. Only when no function defined so far uses the variable: a `fn` declaration or a function value (`fn(...) -> ...`), in an earlier input or earlier in this one. Otherwise it is an error naming the function (`QK-CHECK-001`). Recompiling those functions for the new type is deferred (§7); keeping them on the old variable would let them compute with a value the user no longer sees.
3. Only at the top level of the input, not inside `if`, `for` or `while`: a type that depends on which branch ran can't be checked.
4. A variable defined in the same input follows the file rule: a type change is an error.
5. An assignment of the same type is an ordinary assignment. An `int` assigned to a `float` variable widens as usual and is not a type change.
6. Only variables change type this way; a second `fn` or `struct` with the same name is `QK-LOAD-001`.
7. The old value is released, so the GC can free it.

```
> x = 1
> y = x + 1
> x = 'text'
warning[QK-CHECK-002] (check): 'x' changes type from int to str; a compiled program rejects this at <input 3>:1:1
> n = 1
> fn bump() -> n + 1
> n = 'text'
error[QK-CHECK-001] (check): 'n' can't change type from int to str: 'bump' (<input 5>:1:1) uses it at <input 6>:1:1
```

### 3.5 Empty lists and dicts

An empty `list []` or `dict {}` that its input doesn't write keeps an open element type, and its first write in a later input fixes it, as the first write further down a file does.

```
> xs = list []
> xs.push(1)
> xs.push('a')
error[QK-CHECK-001] (check): cannot add str to list[int]: ...
```

If an untyped path (`any`) put a value in the container before the input that fixes its type, that input stops with a runtime error before any of its statements run, reported at the write that fixed the type. (The same program as a file fails at the untyped write instead, because the container's type is fixed from the start.)

### 3.6 Entering input

- The prompt is `> `; continuation lines get `. `.
- An input goes on while a bracket is open, or when its first line ends with `:` or `->` (a block); a block input ends at an empty line. Otherwise Enter runs the line.
- A paste is one input, whatever it contains (bracketed paste).
- A typed line that starts with `|` (an indented pipe line, grammar.md) can't continue the previous line, which already ran; it is a syntax error. Paste such a statement, or keep it on one line.
- Left and right, Home and End, and Up and Down for the session's history. Ctrl-C at the prompt clears the current input; Ctrl-D at an empty prompt ends the session.
- When standard input isn't a terminal (`quark repl < inputs.txt`), lines are read with the same rules, without prompts or line editing. The tests use this.

### 3.7 Imports and native code

- `incl` works as in a file, with paths relative to the working directory. A file is loaded once per session; its definitions are compiled into the library of the input that includes it. Including it again is the error a file gives (`import 'fmt' (line 2) conflicts with the import 'fmt' defined on line 1`).
- `extern link` libraries of an included module are linked into that input's library and loaded with it. Native code runs in the host process, as it does in a program.

### 3.8 Differences from a file

| | In a file | In a session |
|---|---|---|
| A function using a definition from further down | Allowed (hoisting) | Allowed within one input; an error across inputs (§3.1) |
| A bare expression as the last statement | Prints nothing | Prints its value (§3.3) |
| A variable assigned a value of another type | Error | Warning, if no function uses the variable (§3.4) |
| A runtime error | Ends the program | Ends the input; the session goes back to its state before the input (§3.2) |
| An untyped write into an empty container whose type a later write fixes | Runtime error at the untyped write | Runtime error at the start of the input with the fixing write (§3.5) |

## 4. Diagnostics

| Code | When | Message |
|---|---|---|
| `QK-CHECK-001` | A type change of a variable that a function uses (§3.4) | `'n' can't change type from int to str: 'bump' (<input 5>:1:1) uses it` |
| `QK-CHECK-002` | Warning: a variable changes type (§3.4) | `'x' changes type from int to str; a compiled program rejects this` |
| `QK-RUNTIME-001` | An existing element doesn't match the type a later input fixes (§3.5) | `'xs' holds a str, but this input makes it a list[int]` |
| `QK-REPL-001` (new) | The input stopped with a signal | `the input stopped with signal SIGSEGV; the session is as it was before the input` |
| `QK-REPL-002` (new) | The host process stopped outside an input, or couldn't start; the session ends | `the REPL host stopped unexpectedly; the session has ended` |

`interrupted` (Ctrl-C) has no code: it isn't an error. error_codes.md gets these rows when they are implemented.

## 5. Implementation

### 5.1 Processes

- **`quark repl`** (Go, `main.go` plus a new `repl/` package): reads input, keeps the compiler session (§5.3), runs the toolchain, and sends each compiled input to the host.
- **The host** (C++, a new `runtime/repl/host.cpp`): the runtime and the GC with a loop that runs inputs. It is built once and cached (§5.2). It gets commands and sends results over two pipes; its standard output and error are the terminal's, so program output is never relayed.
- **Running an input.** The host flushes its output and forks. The child `dlopen`s the input's library (`RTLD_NOW | RTLD_GLOBAL`), calls its entry function, flushes its output, reports success and becomes the waiting host; the parent learns of the success through a private pipe and exits. If the child exits instead (a runtime error prints its message and calls `exit(1)`, as in a program), the parent reports the failure and keeps waiting with the state from before the input. Runtime errors need no change in the runtime.
- **Ctrl-C.** While an input runs, the terminal sends `SIGINT` to the whole process group. `quark repl` and the waiting host ignore it; the child keeps the default action and stops. At the prompt the terminal is in raw mode, so Ctrl-C is a key, not a signal.
- **GC.** The host enables the GC's fork handling (`GC_set_handle_fork`) before initializing it, so every child can collect. Session variables live in a table of `QCell*` that the host owns (`q_repl_slots`, scanned by the GC). The libraries of inputs hold no GC roots of their own.
- **Libraries are never unloaded**, so the code and static data of every input (source locations, `QFnInfo`, `QStructDef`) stay valid for the whole session.
- **Cost.** A fork copies the process's page tables, so it grows with the heap; the tests measure it on a heap of several GB.

### 5.2 The runtime library and compiling an input

- The runtime header is precompiled once with `-fPIC -fpch-instantiate-templates -fpch-codegen` at `-O0`, and that precompiled header is compiled once at `-O3` into the runtime object: every template and inline function of the runtime, compiled once. The host links that object and the GC with `-rdynamic`, so the libraries of inputs resolve every runtime and GC symbol against the host: one GC, one resource registry, one `q_runtime_loc`.
- An input: `clang++ -O0 -fPIC -include-pch <pch> -c input.cpp`, then `clang++ -shared` with the native libraries of the modules it includes.
- The cache key is the precompiled header's (`toolchain.pchKey`: compiler binary, flags, runtime and GC headers) plus the host source. The first `quark repl` builds the cache (about 10 s) and says so.
- `quark run` and `quark build` don't change. Whether they also use the runtime object is a question for the pipeline review (§7).

### 5.3 Front end: re-analysing the committed inputs

- The session keeps the source of every committed input. A new input is loaded and analysed together with all committed inputs, as one tree whose top level is a list of input blocks; only the new block is lowered and emitted.
- This makes a failed input free to undo in the compiler: the next analysis doesn't include it. Keeping analyser state between inputs instead would need snapshots, because the analyser changes shared state as it goes (`FixElementType` fixes a pending container type in place).
- The cost is front-end time that grows with the session, about 10 ms per few hundred lines today.
- **Loader**: resolves names over all blocks with the clash rules of one file, and loads imported files once, in the same order every time, so their internal names (`q<N>_stem__name`) stay the same.
- **Analyser** in session mode:
  - Hoists `fn` declarations per input block (`types/analyzer_fixed.go`); a block doesn't see the definitions of later blocks.
  - Applies §3.4 where it reports `cannot assign value of type ...` today (`types/analyzer_expressions.go`), using `Analysis.Captures` to find the functions that use the variable.
  - Records which variables were created with an open element type in an earlier block and fixed by the new block (`FixElementType` already gets the position of the fixing write), for §5.4.
- **Stable names.** Everything a new input refers to in earlier inputs (slot numbers, C names of top-level functions and their thunks and default-argument functions, `_qfi_` infos, `_sdef_` struct definitions) must come out the same in every re-analysis: derived from internal names and declaration order, never from map iteration order or from a counter that later blocks advance. The session compares these assignments for the committed blocks with the previous analysis and stops with a new `INV-REPL-*` invariant error if they differ.

### 5.4 Lowering and emission in session mode

- The entry function is `extern "C" void quark_input_<N>()` instead of `main`; the host has already initialized the GC and the arguments.
- Every variable assigned at the top level of an input is a session variable: a cell in the host's table, read with `q_cell_get` (with the check for "used before assigned", which a function called earlier in the input than the assignment needs, as for a captured variable in a file). Arithmetic on session variables is therefore boxed; raw scalar storage is deferred (§7).
- Top-level `fn` declarations read session variables from the table instead of capturing them, so every one is a plain C++ function (`DispatchDirect`, or `DispatchNative` when fully annotated), called directly from later inputs as well. Closures inside functions work as in a file.
- What later inputs refer to by name gets external linkage under its internal name: top-level functions, their thunks and default-argument functions, `_qfi_` infos, `_sdef_` struct definitions. Later inputs declare these `extern`, and the dynamic linker binds them when the input is loaded. Everything else (temporaries, lambdas, location tables) has internal linkage, so the counters that name them can restart with every input.
- A last statement shown by §3.3 is lowered as `q_print` of its value.
- A type change (§3.4) gives the variable a new symbol and slot, and clears the old slot at the change.
- At the start of the entry function, every variable from §5.3's list is passed to a new runtime helper, `q_settle_elems(value, elem_type, loc)` (`types/elemtype.hpp`). It gives the containers in the value that still have no element type the type this input fixed, following nested lists and dicts, and checks the elements already there; one that doesn't match is a `QK-RUNTIME-001` reported at `loc`, the fixing write.
- The IR verifier runs as for a program.

### 5.5 Line input

- Raw mode, line editing, history and bracketed paste come from `golang.org/x/term` (decision 11). When standard input isn't a terminal, lines are read as they come.
- Whether an input goes on (§3.6) is decided from the lexer's tokens: an open bracket, or a first line whose last token is `:` or `->`.

### 5.6 Platforms

Linux and macOS. Windows is deferred: it has no `fork`, and loading inputs would need the host to export the runtime from an executable as DLL symbols.

## 6. Tests

An integration test file (`integration_repl_test.go`) runs `quark repl` with inputs on standard input and checks the output, plus Go unit tests of the session front end. Cases:

- State across inputs: a variable, a `fn` (direct and native dispatch), a `struct` (identity across inputs: `q_param_struct` accepts a value built in an earlier input), default parameters, an `incl`'d stdlib module and its constants, a function value stored in one input and called in another.
- Failing inputs: a runtime error leaves earlier assignments of the input undone and its new names undefined; a compile error defines nothing; Ctrl-C during a long loop (`SIGINT` to the process group) keeps the session; a crash in native code (a C test library in `src/testfiles/ffi/` that dereferences null) gives `QK-REPL-001` and keeps the session.
- Display: the last bare expression is shown; `void` expressions aren't; earlier bare expressions aren't.
- File rules: `fn` and `struct` defined twice across inputs (`QK-LOAD-001`); the same `incl` twice; a function using a definition from a later input is undefined; the whole script pasted as one input behaves like the file.
- Type changes: allowed with the warning; refused when a `fn` uses the variable, when a function value in a list uses it, when a function earlier in the same input uses it; refused inside `if`; refused within one input; same-type assignment and int-to-float widening are not type changes; the old value is collected (GC heap across rounds, as in `integration_gc_test.go`).
- Empty containers: typed by a write in a later input; the runtime check when an `any` path wrote an element of another type first; nested (`dict` of lists).
- Misuse probes (Robustness Bar rule 2) on a function from an earlier input: the wrong type, the wrong number of arguments, `null`, an `any` value, defaults, a value from a module, a call from native code.
- The fork: a session holding a heap of several GB (time per input), and `std/blas` (OpenBLAS starts threads; check it survives the fork).
- `INV-REPL-*`: re-analysis gives the same names over a long session that uses lambdas, modules and structs.

## 7. Deferred

- Notebooks, a Jupyter kernel and rich output (HTML tables, plots): when the language surface is stable.
- Redefining a `fn` or `struct`, and a type change of a variable that a function uses: both need the functions that use the old definition recompiled against the new one.
- Raw scalar storage for session variables.
- Windows.
- REPL commands (`:help`, `:reset`, `:type x`), completion, history kept between sessions.
- Continuing a typed input with an indented pipe line.
- The optimization level of inputs, and using the runtime object for `quark run`: in the pipeline review, after the table work lands.

## 8. Decisions

1. **A host process plus one shared library per input** (§2): the REPL runs the same compiled code as a program.
2. **A session follows the rules of a file**; the differences are only those in §3.8.
3. **Fork per input**, not unwinding to the start of the input: runtime errors keep calling `exit(1)`, Ctrl-C and native crashes stop only the input, and no input is ever half-applied. The costs: Linux and macOS only, and a fork whose time grows with the heap.
4. **Redefining `fn` and `struct` is an error**, as in a file.
5. **A variable may change type with a warning**, but only when no function uses it; otherwise those functions would compute with the old value.
6. **The last bare expression is shown.**
7. **An empty container's element type is fixed by its first write**, even in a later input, as in a file; a runtime check covers writes the analyser can't see.
8. **Re-analysis of the committed inputs** for every new input, not analyser state kept between inputs: a failed input needs no undo.
9. **Inputs compile at `-O0`** against the `-O3` runtime: about 0.2 s per input. Revisited in the pipeline review.
10. **Session variables are cells** (boxed) in v0.1.
11. **Open:** line editing with `golang.org/x/term`, which would be the module's first dependency. The alternative is no line editing (users wrap the REPL in `rlwrap`).
