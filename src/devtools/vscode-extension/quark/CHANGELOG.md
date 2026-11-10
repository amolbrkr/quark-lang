# Change Log

## [0.1.0] - 2026-11-06

- Install without Node or npm: `go run ./cmd/quark-vsix` (from `src/core/quark`) builds the `.vsix`. Install instructions fixed (the extension was never on a marketplace).
- Grammar updated for the current language: `struct`, `extern`, `extern link` / `include_path`, `module` and `use ... as` names, struct types and literals, return types, `.5` / `2.` floats, strings ending at end of line. `while (x)` and `if (x)` are no longer highlighted as function calls.
- Python and R habits (`def`, `elif`, `return`, `True`/`False`/`None`, `TRUE`/`FALSE`/`NULL`, `function(`, `&&`, `||`, `#` comments) are highlighted as errors; hover explains the Quark form.
- Snippets for every construct (`fn`, `when`, `whenr`, `forr`, `struct`, `use`, `externlib`, ...), checked against the compiler's parser.
- Completion uses data generated from the compiler instead of hand-copied lists. Function and method completions now place the cursor inside the parentheses (they inserted a literal `$0` before). Adds completion for module members after `alias.`, struct fields, and method types from annotations. Import paths are relative to the current file, and stdlib modules are always listed.
- Hover for builtin and module function signatures.
- Run picks the compiler in this order: the `quark.executablePath` setting if set, then `src/core/quark/quark.exe` or `src/core/quark/quark` from an open Quark checkout, then `quark` from PATH. `Cmd+F5` on macOS.

## [0.0.3]

- Add `Quark: Run Current File` command.
- Add `Ctrl+F5` keybinding for active Quark files.
- Add `quark.executablePath` setting for compiler path.
- Sync grammar highlighting with current language keywords/operators.

## [0.0.2]

- Update syntax highlighting for arrow-based function bodies
- Remove obsolete `class` and range operator patterns
- Add extension metadata for local installation
