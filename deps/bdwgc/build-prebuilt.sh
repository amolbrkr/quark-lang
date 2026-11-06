#!/usr/bin/env bash
# Rebuilds the prebuilt static Boehm GC libraries under deps/bdwgc/lib/.
#
# Needs the GC source checked out in deps/bdwgc (see VENDORED.md), CMake, make,
# and these tools (paths via environment variables):
#
#   ZIG       zig binary, used as the C cross-compiler for Linux and macOS
#   LLVM_BIN  directory with clang, lld-link, llvm-ar, llvm-ranlib, llvm-rc (Windows)
#   XWIN_SDK  output of `xwin splat` (MSVC CRT + Windows SDK)        (Windows)
#
# Usage: ZIG=... LLVM_BIN=... XWIN_SDK=... ./build-prebuilt.sh [target...]
# Targets: linux-amd64 linux-arm64 darwin-arm64 windows-amd64 (default: all)
#
# The CMake options match toolchain.EnsureGC so a prebuilt library behaves
# like one bootstrapped on the user's machine.
set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
[ -f "$SRC/CMakeLists.txt" ] || { echo "GC source missing in $SRC; see VENDORED.md" >&2; exit 1; }
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

COMMON=(
  -DCMAKE_BUILD_TYPE=Release
  -DGC_BUILD_SHARED_LIBS=OFF -DBUILD_SHARED_LIBS=OFF
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON
  -Denable_docs=OFF -Dbuild_cord=OFF -DBUILD_TESTING=OFF
  -DCMAKE_TRY_COMPILE_TARGET_TYPE=STATIC_LIBRARY
)

# zig_target <key> <cmake system name> <processor> <zig target triple> [extra C flags]
zig_target() {
  local key=$1 sys=$2 cpu=$3 triple=$4 cflags=${5:-} bin="$WORK/$1-bin"
  mkdir -p "$bin"
  # zig cc enables UBSan and debug info unless told otherwise.
  printf '#!/bin/sh\nexec "%s" cc -target %s -fno-sanitize=undefined -g0 "$@"\n' "$ZIG" "$triple" > "$bin/cc"
  printf '#!/bin/sh\nexec "%s" ar "$@"\n' "$ZIG" > "$bin/ar"
  printf '#!/bin/sh\nexec "%s" ranlib "$@"\n' "$ZIG" > "$bin/ranlib"
  chmod +x "$bin"/*
  cat > "$WORK/$key.cmake" <<EOF
set(CMAKE_SYSTEM_NAME $sys)
set(CMAKE_SYSTEM_PROCESSOR $cpu)
set(CMAKE_C_COMPILER "$bin/cc")
set(CMAKE_AR "$bin/ar")
set(CMAKE_RANLIB "$bin/ranlib")
set(CMAKE_OSX_SYSROOT "")
set(CMAKE_OSX_ARCHITECTURES "")
set(CMAKE_C_FLAGS_INIT "$cflags")
EOF
  build "$key" libgc.a
}

windows_target() {
  local key=windows-amd64 crt="$XWIN_SDK/crt" sdk="$XWIN_SDK/sdk"
  local inc="-isystem $crt/include -isystem $sdk/include/ucrt -isystem $sdk/include/um -isystem $sdk/include/shared"
  cat > "$WORK/$key.cmake" <<EOF
set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR AMD64)
set(CMAKE_C_COMPILER "$LLVM_BIN/clang")
set(CMAKE_C_COMPILER_TARGET x86_64-pc-windows-msvc)
set(CMAKE_AR "$LLVM_BIN/llvm-ar")
set(CMAKE_RANLIB "$LLVM_BIN/llvm-ranlib")
set(CMAKE_LINKER "$LLVM_BIN/lld-link")
set(CMAKE_RC_COMPILER "$LLVM_BIN/llvm-rc")
set(CMAKE_C_FLAGS_INIT "$inc")
# Static CRT (libcmt), the default of the clang++ that links Quark programs.
# A DLL CRT (_DLL) would also make gc.h declare the GC as a DLL import.
set(CMAKE_C_FLAGS_RELEASE "-O3 -DNDEBUG -D_MT -Xclang --dependent-lib=libcmt" CACHE STRING "" FORCE)
set(CMAKE_EXE_LINKER_FLAGS_INIT "-fuse-ld=lld -L$crt/lib/x86_64 -L$sdk/lib/um/x86_64 -L$sdk/lib/ucrt/x86_64")
EOF
  build "$key" gc.lib
}

# build <key> <library file name>
build() {
  local key=$1 lib=$2 dir="$WORK/$1"
  echo "== $key" >&2
  cmake -S "$SRC" -B "$dir" -G "Unix Makefiles" "${COMMON[@]}" \
    -DCMAKE_TOOLCHAIN_FILE="$WORK/$key.cmake" > "$dir.log" 2>&1 \
    || { cat "$dir.log" >&2; exit 1; }
  cmake --build "$dir" --target gc -j"$(nproc)" >> "$dir.log" 2>&1 \
    || { tail -50 "$dir.log" >&2; exit 1; }
  local out
  out="$(find "$dir" -maxdepth 2 -name "$lib" -o -maxdepth 2 -name "libgc.lib" -o -maxdepth 2 -name "libgc.a" | head -1)"
  [ -n "$out" ] || { echo "no $lib built for $key" >&2; exit 1; }
  mkdir -p "$SRC/lib/$key"
  cp "$out" "$SRC/lib/$key/$lib"
  echo "   -> lib/$key/$lib" >&2
}

targets=("$@")
[ ${#targets[@]} -gt 0 ] || targets=(linux-amd64 linux-arm64 darwin-arm64 windows-amd64)
for t in "${targets[@]}"; do
  case $t in
    linux-amd64)   zig_target "$t" Linux  x86_64  x86_64-linux-gnu.2.28 ;;
    linux-arm64)   zig_target "$t" Linux  aarch64 aarch64-linux-gnu.2.28 ;;
    # Zig's macOS headers lack mach/exception.h and mach-o/getsect.h.
    # NO_MPROTECT_VDB drops the mprotect dirty-bit code that needs the first
    # (it only matters for incremental GC, which Quark never enables);
    # MISSING_MACH_O_GETSECT_H makes bdwgc declare getsectiondata itself.
    darwin-arm64)  zig_target "$t" Darwin arm64   aarch64-macos.11.0 "-DNO_MPROTECT_VDB -DMISSING_MACH_O_GETSECT_H" ;;
    windows-amd64) windows_target ;;
    *) echo "unknown target $t" >&2; exit 1 ;;
  esac
done
