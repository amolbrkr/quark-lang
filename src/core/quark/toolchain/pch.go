package toolchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The runtime is header-only, so every compile parses ~70k lines of runtime
// headers before reaching user code. A precompiled quark.hpp, cached per
// (compiler, flags, header state), removes most of that cost.

// precompiledHeader returns the cached PCH for the runtime header compiled
// with flags, building it if needed.
func (tc *Toolchain) precompiledHeader(flags []string, opts Options) (string, error) {
	key, err := tc.pchKey(flags)
	if err != nil {
		return "", err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "quark", "pch")
	pch := filepath.Join(dir, key+".pch")
	if _, err := os.Stat(pch); err == nil {
		return pch, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Build under a unique name and rename, so concurrent builds never see a
	// partially written PCH.
	tmp, err := os.CreateTemp(dir, key+".*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	args := append([]string{}, flags...)
	args = append(args, "-x", "c++-header", filepath.Join(tc.RuntimeInclude, "quark", "quark.hpp"), "-o", tmpPath)
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Debug: building precompiled header: %s %s\n", tc.Compiler, strings.Join(args, " "))
	}
	var out bytes.Buffer
	cmd := exec.Command(tc.Compiler, args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v\n%s", err, out.String())
	}
	if err := os.Rename(tmpPath, pch); err != nil {
		return "", err
	}
	return pch, nil
}

// pchKey hashes everything a PCH depends on: the compiler binary, the flags,
// and the path, size and mtime of every runtime and GC header. clang rejects
// a PCH whose headers changed on disk, so mtimes belong in the key.
func (tc *Toolchain) pchKey(flags []string) (string, error) {
	h := sha256.New()
	compiler, err := filepath.EvalSymlinks(tc.Compiler)
	if err != nil {
		return "", err
	}
	if err := hashFileStat(h, compiler); err != nil {
		return "", err
	}
	fmt.Fprintf(h, "flags:%s\n", strings.Join(flags, "\x00"))
	for _, root := range []string{tc.RuntimeInclude, tc.GCInclude} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			return hashFileStat(h, path)
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:24], nil
}

func hashFileStat(h interface{ Write([]byte) (int, error) }, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(h, "%s:%d:%d\n", path, info.Size(), info.ModTime().UnixNano())
	return nil
}

// isPCHError reports whether clang output says the PCH itself was rejected
// (for example a system header changed after it was built).
func isPCHError(output string) bool {
	return strings.Contains(output, "precompiled header") || strings.Contains(output, "PCH file")
}
