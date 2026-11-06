// Command quark-vsix packages the Quark VS Code / Positron extension
// (src/devtools/vscode-extension/quark) into a .vsix file, without Node,
// npm or vsce.
//
//	cd src/core/quark && go run ./cmd/quark-vsix [-o out.vsix]
//
// The default output is src/devtools/vscode-extension/dist/<name>-<version>.vsix.
// Install it with "Extensions: Install from VSIX..." in VS Code or Positron,
// or `code --install-extension <file>`.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"quark/cmd/quark-vsix/vsix"
)

func main() {
	out := flag.String("o", "", "output .vsix path")
	flag.Parse()

	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", ".."))
	extDir := filepath.Join(repo, "src", "devtools", "vscode-extension", "quark")

	path, err := vsix.Build(extDir, filepath.Join(repo, "LICENSE"), *out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "quark-vsix: %s\n", err)
		os.Exit(1)
	}
	fmt.Println(path)
}
