# Setup Guide for Quark Rust Implementation

## Prerequisites

### 1. Install Rust

If you don't have Rust installed, install it using rustup:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

Follow the prompts and restart your terminal, or run:

```bash
source $HOME/.cargo/env
```

Verify installation:

```bash
rustc --version
cargo --version
```

### 2. Install GraphViz (Optional, for PNG visualization)

**Ubuntu/Debian:**
```bash
sudo apt-get install graphviz
```

**Arch Linux:**
```bash
sudo pacman -S graphviz
```

**macOS:**
```bash
brew install graphviz
```

**Windows:**
Download from: https://graphviz.org/download/

## Building the Project

Navigate to the Rust implementation directory:

```bash
cd /home/amol/code/quark-lang/src/core_new
```

Build the project:

```bash
cargo build --release
```

The compiled binary will be at: `target/release/quark`

## Quick Start

### Test with the existing test.qrk file:

```bash
# Run the complete pipeline
cargo run --release -- run ../../test.qrk

# Or use the compiled binary directly
./target/release/quark run ../../test.qrk
```

### Other commands:

```bash
# Lex only
cargo run -- lex ../../test.qrk

# Parse only
cargo run -- parse ../../test.qrk --tree

# Visualize only
cargo run -- visualize ../../test.qrk
```

## Running Tests

```bash
cargo test
```

To see test output:

```bash
cargo test -- --nocapture
```

## Development

### Watch mode (auto-rebuild on changes):

Install cargo-watch:
```bash
cargo install cargo-watch
```

Run in watch mode:
```bash
cargo watch -x 'run -- run ../../test.qrk'
```

### Format code:

```bash
cargo fmt
```

### Lint code:

```bash
cargo clippy
```

## Troubleshooting

### Error: "dot command not found"

The `dot` command is part of GraphViz. Install it as described above.

If you still get errors, you can generate just the DOT file:

```bash
cargo run -- visualize ../../test.qrk --no-png
```

Then manually convert with:

```bash
dot -Tpng treeviz.dot -o treeviz.png
```

### Error: "cannot find -lstdc++"

Install g++ development tools:

```bash
# Ubuntu/Debian
sudo apt-get install build-essential

# Arch Linux
sudo pacman -S base-devel
```

## Project Structure

```
core_new/
├── Cargo.toml          # Project configuration and dependencies
├── lib.rs              # Library entry point (exports all modules)
├── main.rs             # CLI application entry point
├── token.rs            # Token types and definitions
├── lexer.rs            # Lexer implementation
├── ast.rs              # AST node types and precedence
├── parser.rs           # Pratt parser implementation
├── visualizer.rs       # GraphViz DOT generation
├── README.md           # Usage documentation
└── SETUP.md           # This file
```

## Next Steps

After successful build:

1. Test with `test.qrk`: `cargo run -- run ../../test.qrk`
2. Try parsing your own Quark programs
3. View the generated `treeviz.png` to see the AST structure
4. Run tests with `cargo test`

## Performance Notes

- Debug builds (`cargo build`) include debugging symbols and are slower
- Release builds (`cargo build --release`) are optimized and ~10-50x faster
- Use `--release` for parsing large files
- The lexer and parser are single-threaded (sufficient for current use case)

## Differences from Python Implementation

This Rust implementation provides the same language features as the Python version but with:

- **Better performance**: Rust is compiled and optimized
- **Type safety**: Compile-time guarantees prevent many runtime errors
- **Single binary**: No Python interpreter needed
- **Unified CLI**: All commands in one tool
- **Better error messages**: Rust's error handling with anyhow
- **Memory safety**: No garbage collection overhead

Both implementations generate identical ASTs for the same input.
