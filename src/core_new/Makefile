.PHONY: build release test clean run lex parse visualize fmt clippy install

# Default target
all: build

# Build in debug mode
build:
	cargo build

# Build in release mode (optimized)
release:
	cargo build --release

# Run tests
test:
	cargo test

# Run tests with output
test-verbose:
	cargo test -- --nocapture

# Clean build artifacts
clean:
	cargo clean
	rm -f treeviz.dot treeviz.png

# Run complete pipeline on test.qrk
run:
	cargo run -- run ../../test.qrk

# Run complete pipeline on examples.qrk
run-examples:
	cargo run -- run examples.qrk

# Lex test.qrk
lex:
	cargo run -- lex ../../test.qrk

# Parse test.qrk with tree output
parse:
	cargo run -- parse ../../test.qrk --tree

# Visualize test.qrk
visualize:
	cargo run -- visualize ../../test.qrk

# Format code
fmt:
	cargo fmt

# Lint code
clippy:
	cargo clippy

# Install binary to ~/.cargo/bin
install:
	cargo install --path .

# Quick test with release build
quick: release
	./target/release/quark run ../../test.qrk

# Development watch mode (requires cargo-watch)
watch:
	cargo watch -x 'run -- run ../../test.qrk'

# Help
help:
	@echo "Quark Compiler Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build         - Build in debug mode"
	@echo "  release       - Build in release mode (optimized)"
	@echo "  test          - Run tests"
	@echo "  test-verbose  - Run tests with output"
	@echo "  clean         - Clean build artifacts"
	@echo "  run           - Run complete pipeline on test.qrk"
	@echo "  run-examples  - Run complete pipeline on examples.qrk"
	@echo "  lex           - Lex test.qrk"
	@echo "  parse         - Parse test.qrk with tree output"
	@echo "  visualize     - Visualize test.qrk AST"
	@echo "  fmt           - Format code"
	@echo "  clippy        - Lint code"
	@echo "  install       - Install binary to ~/.cargo/bin"
	@echo "  quick         - Quick test with release build"
	@echo "  watch         - Development watch mode (requires cargo-watch)"
	@echo "  help          - Show this help message"
