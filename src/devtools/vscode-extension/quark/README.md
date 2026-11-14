# Quark for VS Code and Positron

Editor support for [Quark](https://github.com/amolbrkr/quark-lang) `.qrk` files: highlighting, snippets, completion, hover, check diagnostics and a run command. Version 0.2.0.

## Contents

- [Features](#features)
- [Install](#install)
- [Settings](#settings)
- [Development](#development)

## Features

- **Highlighting** for the current language: `fn`, `struct`, `module`, `incl ... as`, `when`, `extern`, `extern link` / `extern include_path`, `::` paths (`io::exists`, `geo::shapes::Sq`), type annotations and return types (including `tbl` / `tbl[Row]`), struct literals, and `list [...]` / `dict {...}` / `vec [...]`.
- **Python and R habits are flagged.** `def`, `elif`, `return`, `True` / `False` / `None`, `TRUE` / `FALSE` / `NULL`, `function(`, `&&`, `||` and `#` comments are shown as errors. Hover one to see what Quark uses instead.
- **Snippets** that show the Quark form of each construct. Type the prefix and press Tab:

  | Prefix | Inserts |
  |---|---|
  | `fn`, `fn1`, `fnt`, `fnd` | function (block, one line, typed, default parameter) |
  | `lambda` | `fn(x) -> x` |
  | `if`, `ife`, `ifel`, `tern` | if, if/else, if/elseif/else, `a if cond else b` |
  | `when`, `whenr` | pattern match; match `ok` / `err` results |
  | `for`, `forr`, `fore`, `while` | loops over items, `range(...)`, `.enumerate()` |
  | `list`, `dict`, `vec` | collection literals |
  | `struct`, `new` | struct type, struct value |
  | `module`, `incl`, `inclf` | module block, stdlib import, file import |
  | `query` | table query (`filter` / `summarize` pipeline) |
  | `okerr`, `pipe` | function returning `ok` / `err`, pipe chain |
  | `extern`, `externlib` | call C++, link a native library |

- **Completion**:
  - keywords, builtins with their signatures, import bindings, and names used in the file;
  - methods for the receiver's type (`str`, `list`, `dict`, `vec`, `tbl`), from annotations like `s: str` or `t: tbl[Emp]`, literals, and assignments;
  - `name::` lists the functions of an imported file (stdlib or another file: its top-level functions and those of its primary module), of a module of an imported file (`file::module::`), or of a module in the same file;
  - `point.` lists a struct's fields;
  - `incl '` lists stdlib modules and `.qrk` files, as paths relative to the current file.
- **Hover** shows the signatures of builtins, methods and imported functions (`io::exists`), and explains flagged Python and R habits.
- **Check diagnostics:** `quark check` runs on open, save and (debounced) as you type. Errors and warnings appear as squiggles with their `QK-*` codes. **Quark: Check Current File** (`Ctrl+Shift+B` / `Cmd+Shift+B`) or the check button in the editor title forces a run. There is still no language server, so there is no go-to-definition.
- **Run** the current file with `Ctrl+F5` (`Cmd+F5` on macOS), the ▶ button in the editor title, or **Quark: Run Current File** in the Command Palette. It saves the file and runs `quark run <file>` in a terminal named Quark.

## Install

The extension isn't published to a marketplace. Build the package from a Quark checkout (needs only Go, no Node or npm):

```bash
cd src/core/quark
go run ./cmd/quark-vsix            # or: go run ./cmd/quark-vsix -o <path>.vsix
# -> src/devtools/vscode-extension/dist/quark-language-<version>.vsix
```

Then install it:

- **VS Code or Positron:** open the Command Palette, run **Extensions: Install from VSIX...**, and pick the file.
- **Command line:** `code --install-extension <file>.vsix` (VS Code) or `positron --install-extension <file>.vsix` (Positron desktop).

Reload the window afterwards. To update, rebuild and install again; to remove it, uninstall "Quark Language Support" from the Extensions view.

With this Quark checkout open, the extension finds `src/core/quark/quark.exe` automatically for Check and Run. Otherwise set `quark.executablePath` or put `quark` on PATH.

## Settings

| Setting | Default | Description |
|---|---|---|
| `quark.executablePath` | `quark` | Compiler used by Run, Check and live diagnostics. Left at `quark`, the extension uses `src/core/quark/quark.exe` (or `src/core/quark/quark`) from an open Quark checkout if it exists, then `quark` from PATH. |
| `quark.checkOnType` | `true` | Run `quark check` while typing (debounced). Open and save always check. |

## Development

- Builtins, methods, keywords and stdlib modules come from `data/quark.json`, which is generated from the compiler's builtin catalog, lexer keywords and stdlib. After changing any of them, regenerate it:

  ```bash
  cd src/core/quark && go test ./internal/editordata -update
  ```

- `go test ./internal/editordata` fails when the data file is stale, when the grammar misses a keyword or builtin (a new builtin must be added to the grammar's `builtin-functions` pattern), or when a snippet doesn't parse as Quark. With `node` on PATH (or `QUARK_NODE` set to one, such as the node bundled with VS Code or Positron) it also runs `test/extension.test.js`.
- Try changes without packaging: open `src/devtools/vscode-extension/quark` in VS Code and press F5 to start an Extension Development Host.
