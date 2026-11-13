# Vendored Boehm GC

## Contents

- [What is committed](#what-is-committed)
- [Platforms without a prebuilt library](#platforms-without-a-prebuilt-library)
- [Rebuilding the prebuilt libraries](#rebuilding-the-prebuilt-libraries)

## What is committed

Only the files Quark needs at build time are committed here, not the GC source:

- `include/gc.h`, `include/gc_cpp.h`, `include/gc/*.h` — public headers
- `lib/<os>-<arch>/` — prebuilt static libraries, picked by `toolchain.EnsureGC`
  from the compiler's `GOOS`/`GOARCH`:

  | Directory | File | Built with |
  |---|---|---|
  | `lib/linux-amd64` | `libgc.a` | `zig cc -target x86_64-linux-gnu.2.28` |
  | `lib/linux-arm64` | `libgc.a` | `zig cc -target aarch64-linux-gnu.2.28` |
  | `lib/darwin-arm64` | `libgc.a` | `zig cc -target aarch64-macos.11.0` |
  | `lib/windows-amd64` | `gc.lib` | `clang --target=x86_64-pc-windows-msvc`, static CRT (`libcmt`) |

- `build-prebuilt.sh` — rebuilds the libraries above
- `LICENSE`

Upstream: https://github.com/ivmai/bdwgc
Commit: 946696b9b16580fcece3bc4d640f35afc139426d (version 8.3.0, development)

All libraries use the CMake options `toolchain.EnsureGC` uses (Release, static,
threads on, cord/docs/tests off). Platform notes:

- **glibc 2.28** is the oldest glibc the Linux headers target; the libraries
  work with that version and newer.
- **macOS** is built with `-DNO_MPROTECT_VDB -DMISSING_MACH_O_GETSECT_H`,
  because Zig's macOS headers lack `mach/exception.h` and `mach-o/getsect.h`.
  This drops the mprotect-based dirty bits used only by incremental collection,
  which Quark never enables. Minimum macOS version: 11.0.
- **Windows** links the static CRT, which is the default for the `clang++` that
  links Quark programs. A DLL CRT (`_DLL`) would also make `gc.h` declare the GC
  as a DLL import.

## Platforms without a prebuilt library

On any other platform (for example macOS x86_64), `quark build` falls back to
`build-static/`, a local CMake build of the GC source. To get the source, clone
upstream at the commit above into this directory (the source files are ignored
by git) and install CMake. The first build compiles the GC once.

## Rebuilding the prebuilt libraries

With the GC source checked out here (as above):

```bash
ZIG=/path/to/zig \
LLVM_BIN=/path/to/llvm/bin \
XWIN_SDK=/path/to/xwin-splat \
./build-prebuilt.sh            # or name targets: linux-arm64 windows-amd64 ...
```

- `ZIG`: Zig 0.15.2 was used.
- `LLVM_BIN`: needs `clang`, `lld-link`, `llvm-ar`, `llvm-ranlib` and `llvm-rc`
  (LLVM 22.1.0 was used).
- `XWIN_SDK`: the output of
  `xwin --accept-license --arch x86_64 splat --output <dir>`
  ([xwin](https://github.com/Jake-Shadle/xwin) 0.10.0 was used). This downloads
  the MSVC CRT and Windows SDK, and `--accept-license` accepts Microsoft's
  license for them.
