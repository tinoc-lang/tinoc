#!/usr/bin/env bash
set -euo pipefail

# ---------- defaults ----------
BUILD_DIR="build"
BUILD_TYPE="Debug"
CLEAN=0
RUN_TESTS=0
RUN_AFTER=0
VERBOSE=0
BUILD_TESTS=1
TARGET_BIN="tinoc"

# ---------- colors ----------
if [[ -t 1 ]]; then
    C_RESET="\033[0m"; C_BOLD="\033[1m"
    C_GREEN="\033[32m"; C_RED="\033[31m"; C_YELLOW="\033[33m"; C_BLUE="\033[34m"
else
    C_RESET=""; C_BOLD=""; C_GREEN=""; C_RED=""; C_YELLOW=""; C_BLUE=""
fi

info()  { echo -e "${C_BLUE}==>${C_RESET} $*"; }
ok()    { echo -e "${C_GREEN}✓${C_RESET} $*"; }
warn()  { echo -e "${C_YELLOW}!${C_RESET} $*"; }
error() { echo -e "${C_RED}✗ error:${C_RESET} $*" >&2; }

usage() {
    cat <<EOF
${C_BOLD}usage:${C_RESET} $0 [options]

options:
  -t <type>   build type: Debug|Release|RelWithDebInfo (default: $BUILD_TYPE)
  -d <dir>    build directory (default: $BUILD_DIR)
  -j <N>      parallel jobs (default: nproc)
  -c          clean build directory first
  -s          src only — skip configuring/building tests (sets TINOC_BUILD_TESTS=OFF)
  -T          run tests after building (ctest)
  -r          run the resulting binary after building
  -v          verbose build output
  -h          show this help

examples:
  $0                    # normal debug build, src + tests
  $0 -s                 # build only the tinoc binary, no tests at all
  $0 -t Release -T      # release build + run tests
  $0 -c -r              # clean rebuild, then run the binary
EOF
    exit 1
}

# ---------- arg parsing ----------
while getopts "t:d:j:csTrvh" opt; do
    case "$opt" in
        t) BUILD_TYPE="$OPTARG" ;;
        d) BUILD_DIR="$OPTARG" ;;
        j) JOBS="$OPTARG" ;;
        c) CLEAN=1 ;;
        s) BUILD_TESTS=0 ;;
        T) RUN_TESTS=1 ;;
        r) RUN_AFTER=1 ;;
        v) VERBOSE=1 ;;
        h) usage ;;
        *) usage ;;
    esac
done

JOBS="${JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

if [[ "$BUILD_TESTS" -eq 0 && "$RUN_TESTS" -eq 1 ]]; then
    error "-s (skip tests) and -T (run tests) are mutually exclusive"
    exit 1
fi

case "$BUILD_TYPE" in
    Debug|Release|RelWithDebInfo|MinSizeRel) ;;
    *)
        error "invalid build type '$BUILD_TYPE' (expected Debug|Release|RelWithDebInfo|MinSizeRel)"
        exit 1
        ;;
esac

# ---------- sanity checks ----------
if [[ ! -f "CMakeLists.txt" ]]; then
    error "run this from the project root (CMakeLists.txt not found)"
    exit 1
fi

if ! command -v cmake >/dev/null 2>&1; then
    error "cmake not found — install it first"
    exit 1
fi

if command -v clang++ >/dev/null 2>&1; then
    export CC=clang
    export CXX=clang++
else
    error "clang++ not found — this project requires Clang 17+"
    exit 1
fi

# ---------- clean ----------
if [[ "$CLEAN" -eq 1 && -d "$BUILD_DIR" ]]; then
    info "cleaning $BUILD_DIR"
    rm -rf "$BUILD_DIR"
fi

# ---------- configure ----------
info "configuring (${BUILD_TYPE}, tests=$([[ $BUILD_TESTS -eq 1 ]] && echo ON || echo OFF)) in ${BUILD_DIR}/"
cmake -S . -B "$BUILD_DIR" \
    -DCMAKE_BUILD_TYPE="$BUILD_TYPE" \
    -DCMAKE_EXPORT_COMPILE_COMMANDS=ON \
    -DTINOC_BUILD_TESTS=$([[ $BUILD_TESTS -eq 1 ]] && echo ON || echo OFF)

# ---------- build ----------
info "building with ${JOBS} parallel job(s)"
BUILD_START=$(date +%s)

BUILD_TARGET_ARGS=()
if [[ "$BUILD_TESTS" -eq 0 ]]; then
    BUILD_TARGET_ARGS=(--target "$TARGET_BIN")
fi

if [[ "$VERBOSE" -eq 1 ]]; then
    cmake --build "$BUILD_DIR" "${BUILD_TARGET_ARGS[@]}" --parallel "$JOBS"
else
    if ! cmake --build "$BUILD_DIR" "${BUILD_TARGET_ARGS[@]}" --parallel "$JOBS" 2>build_errors.log; then
        error "build failed:"
        cat build_errors.log >&2
        rm -f build_errors.log
        exit 1
    fi
    rm -f build_errors.log
fi

BUILD_END=$(date +%s)
ok "build finished in $((BUILD_END - BUILD_START))s -> ${BUILD_DIR}/bin/${TARGET_BIN}"

# ---------- tests ----------
if [[ "$RUN_TESTS" -eq 1 ]]; then
    info "running tests"
    if (cd "$BUILD_DIR" && ctest --output-on-failure); then
        ok "all tests passed"
    else
        error "some tests failed"
        exit 1
    fi
fi

# ---------- run ----------
if [[ "$RUN_AFTER" -eq 1 ]]; then
    BIN_PATH="${BUILD_DIR}/bin/${TARGET_BIN}"
    if [[ -x "$BIN_PATH" ]]; then
        info "running ${BIN_PATH}"
        echo "----------------------------------------"
        "$BIN_PATH"
    else
        error "binary not found at ${BIN_PATH}"
        exit 1
    fi
fi