# Quark Language Support for VS Code

Syntax highlighting and language support for the Quark programming language.

## Features

- **Syntax Highlighting** - Full syntax highlighting for `.qrk` files
- **Keywords** - `fn`, `if`, `elseif`, `else`, `when`, `for`, `while`, `in`, `and`, `or`, `not`, `true`, `false`, `null`, `module`, `use`
- **Operators** - Arithmetic, comparison, logical, pipe (`|`), range (`..`)
- **Comments** - Single-line comments with `//`
- **Strings** - Single-quoted strings with escape sequences

## About Quark

Quark is a high-level, dynamically-typed language that compiles to C++, designed for fast data-heavy applications. It combines Python-like syntax with native performance.

```quark
// Quark example
fn factorial n:
    when n:
        0 or 1: 1
        _: n * factorial n - 1

factorial 10 | println
```

## Installation

1. Open VS Code
2. Go to Extensions (Ctrl+Shift+X)
3. Search for "Quark"
4. Click Install

Or install from VSIX:
```bash
code --install-extension quark-0.0.1.vsix
```

## Requirements

- VS Code 1.60.0 or higher

## Extension Settings

This extension does not add any VS Code settings.

## Known Issues

- No language server support yet (no autocomplete, go-to-definition)
- No debugger integration

## Release Notes

### 0.0.1

Initial release with basic syntax highlighting.

## More Information

- [Quark Repository](https://github.com/user/quark-lang)
- [Language Documentation](https://github.com/user/quark-lang/blob/main/CLAUDE.md)
