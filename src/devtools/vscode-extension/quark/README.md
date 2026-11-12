# Quark for VS Code and Positron

Editor support for [Quark](https://github.com/amolbrkr/quark-lang) `.qrk` files.

## Features

- **Highlighting** for the current language: `fn`, `struct`, `module`, `when`, `extern link`, type annotations, struct literals, `list [...]` / `dict {...}` / `vec [...]`.
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
  | `okerr`, `pipe` | function returning `ok` / `err`, pipe chain |
  | `extern`, `externlib` | call C++, link a native library |

- **Completion**:
  - keywords and builtins, with their signatures;
  - methods for the receiver's type (`str`, `list`, `dict`, `vec`), from annotations like `s: str`, literals, and assignments;
  - `alias.` lists the functions of an imported module (stdlib, another file, or a module in the same file);
  - `point.` lists a struct's fields;
  - `incl '` lists stdlib modules and `.qrk` files, as paths relative to the current file.
- **Hover** shows builtin and module function signatures.
- **Run** the current file with `Ctrl+F5` (`Cmd+F5` on macOS) or the ▶ button in the editor title.

There is no language server yet, so there are no live error squiggles or go-to-definition. Completion reads the open file as text: it doesn't run the compiler.

## Install

The extension isn't published to a marketplace. Build the package from a Quark checkout (needs only Go):

```bash
cd src/core/quark
go run ./cmd/quark-vsix
# -> src/devtools/vscode-extension/dist/quark-language-<version>.vsix
```

Then install it:

- **VS Code or Positron:** open the Command Palette, run **Extensions: Install from VSIX...**, and pick the file.
- **Command line:** `code --install-extension <file>.vsix` (VS Code) or `positron --install-extension <file>.vsix` (Positron desktop).

Reload the window afterwards. To update, rebuild and install again; to remove it, uninstall "Quark Language Support" from the Extensions view.

## Settings

| Setting | Default | Description |
|---|---|---|
| `quark.executablePath` | `quark` | Compiler used by **Quark: Run Current File**. Left at `quark`, the extension uses `src/core/quark/quark.exe` (or `src/core/quark/quark`) from an open Quark checkout if it exists, then `quark` from PATH. |

## Development

- Builtins, keywords and stdlib modules come from `data/quark.json`, which is generated from the compiler. After changing builtins, keywords or the stdlib, regenerate it:

  ```bash
  cd src/core/quark && go test ./internal/editordata -update
  ```

- `go test ./internal/editordata` fails when the data file is stale, when the grammar misses a keyword or builtin, or when a snippet doesn't parse as Quark. With `node` on PATH (or `QUARK_NODE` set to one, such as the node bundled with VS Code or Positron) it also runs `test/extension.test.js`.
- Try changes without packaging: open `src/devtools/vscode-extension/quark` in VS Code and press F5 to start an Extension Development Host.
