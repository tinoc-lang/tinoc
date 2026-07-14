#!/usr/bin/env bash
set -euo pipefail

BUILD_DIR="build"
BUILD_TYPE="Debug"
CLEAN=0

usage() {
    echo "usage: $0 [-t Debug|Release|RelWithDebInfo] [-c] [-j N]"
    echo "  -t  build type (default: Debug)"
    echo "  -c  clean build directory first"
    echo "  -j  parallel jobs (default: nproc)"
    exit 1
}

JOBS="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)"

while getopts "t:cj:h" opt; do
    case "$opt" in
        t) BUILD_TYPE="$OPTARG" ;;
        c) CLEAN=1 ;;
        j) JOBS="$OPTARG" ;;
        h) usage ;;
        *) usage ;;
    esac
done

if [[ ! -f "CMakeLists.txt" ]]; then
    echo "error: run this from the project root (CMakeLists.txt not found)" >&2
    exit 1
fi

if [[ "$CLEAN" -eq 1 ]]; then
    rm -rf "$BUILD_DIR"
fi

export CC=clang
export CXX=clang++

cmake -S . -B "$BUILD_DIR" \
    -DCMAKE_BUILD_TYPE="$BUILD_TYPE" \
    -DCMAKE_EXPORT_COMPILE_COMMANDS=ON

cmake --build "$BUILD_DIR" --parallel "$JOBS"

echo "build ok -> $BUILD_DIR/bin/tinoc"
