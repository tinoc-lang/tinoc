#!/usr/bin/env bash
set -eo pipefail

# ==============================================================================
# Configuration (Defaults can be overridden via Environment Variables)
# ==============================================================================
BINARY_NAME="${BINARY_NAME:-tinoc}"
BUILD_DIR="${BUILD_DIR:-build}"
MODE="${1:-release}"

# ANSI Colors (disabled if output is not a TTY)
if [ -t 1 ]; then
    COLOR_BLUE='\033[1;34m'
    COLOR_GREEN='\033[1;32m'
    COLOR_YELLOW='\033[1;33m'
    COLOR_RED='\033[1;31m'
    COLOR_RESET='\033[0m'
else
    COLOR_BLUE='' COLOR_GREEN='' COLOR_YELLOW='' COLOR_RED='' COLOR_RESET=''
fi

log_info()  { echo -e "${COLOR_BLUE}[INFO]${COLOR_RESET} $1"; }
log_ok()    { echo -e "${COLOR_GREEN}[OK]${COLOR_RESET} $1"; }
log_warn()  { echo -e "${COLOR_YELLOW}[WARN]${COLOR_RESET} $1"; }
log_error() { echo -e "${COLOR_RED}[ERROR]${COLOR_RESET} $1"; }

# ==============================================================================
# Dependency & Environment Checks
# ==============================================================================
if ! command -v odin >/dev/null 2>&1; then
    log_error "Odin compiler ('odin') not found in PATH."
    exit 1
fi

is_termux() {
    [ -d "/data/data/com.termux" ] || [ -n "${TERMUX_VERSION:-}" ]
}

# ==============================================================================
# Target Auto-Detection
# ==============================================================================
if [ -n "${ODIN_TARGET:-}" ]; then
    TARGET_SPEC="$ODIN_TARGET"
else
    # 1. Detect OS
    if [ -n "${TARGET_OS:-}" ]; then
        OS="$TARGET_OS"
    else
        UNAME_S="$(uname -s 2>/dev/null || echo "unknown")"
        case "$UNAME_S" in
            Linux*)               OS="linux" ;;
            Darwin*)              OS="darwin" ;;
            MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
            FreeBSD*)             OS="freebsd" ;;
            NetBSD*)              OS="netbsd" ;;
            OpenBSD*)             OS="openbsd" ;;
            *)                    OS="linux" ;;
        esac
    fi

    # 2. Detect Architecture
    if [ -n "${TARGET_ARCH:-}" ]; then
        ARCH="$TARGET_ARCH"
    else
        UNAME_M="$(uname -m 2>/dev/null || echo "unknown")"
        case "$UNAME_M" in
            x86_64|amd64)         ARCH="amd64" ;;
            aarch64|arm64)        ARCH="arm64" ;;
            i386|i686)            ARCH="i386" ;;
            armv7l|armv6l|arm32)  ARCH="arm32" ;;
            riscv64)              ARCH="riscv64" ;;
            *)                    ARCH="amd64" ;;
        esac
    fi

    TARGET_SPEC="${OS}_${ARCH}"
fi

# Set binary extension
FINAL_BINARY="${BINARY_NAME}"
if [[ "$TARGET_SPEC" == windows* ]]; then
    FINAL_BINARY="${BINARY_NAME}.exe"
fi
OUTPUT_PATH="${BUILD_DIR}/${FINAL_BINARY}"

# ==============================================================================
# Action Handler
# ==============================================================================
case "$MODE" in
    clean)
        log_info "Cleaning build directory (${BUILD_DIR}/)..."
        rm -rf "${BUILD_DIR}"
        log_ok "Clean complete."
        exit 0
        ;;
    debug)
        ODIN_FLAGS="-o:none -debug"
        log_info "Build Mode: DEBUG"
        ;;
    release)
        ODIN_FLAGS="-o:speed"
        log_info "Build Mode: RELEASE (Optimized for speed)"
        ;;
    size)
        ODIN_FLAGS="-o:size"
        log_info "Build Mode: SIZE (Optimized for binary size)"
        ;;
    run)
        MODE="release"
        ODIN_FLAGS="-o:speed"
        SHOULD_RUN=true
        ;;
    test)
        log_info "Running Odin package tests..."
        odin test .
        exit 0
        ;;
    *)
        log_error "Unknown build mode '$MODE'. Allowed: release | debug | size | clean | run | test"
        exit 1
        ;;
esac

# ==============================================================================
# Platform-Specific Adjustments (Termux / Android Fix)
# ==============================================================================
if is_termux || [ "${OS:-}" = "android" ]; then
    log_warn "Termux/Android environment detected. Applying -no-thread-local fix..."
    ODIN_FLAGS="${ODIN_FLAGS} -no-thread-local"
fi

# ==============================================================================
# Execution
# ==============================================================================
mkdir -p "${BUILD_DIR}"

log_info "Building '${BINARY_NAME}' for target '${TARGET_SPEC}'..."
START_TIME=$(date +%s%N 2>/dev/null || date +%s)

# Execute Build
odin build . -out:"${OUTPUT_PATH}" -target:"${TARGET_SPEC}" ${ODIN_FLAGS}

# Post-Build Fixes for Android Linker
if is_termux && command -v termux-elf-cleaner >/dev/null 2>&1; then
    termux-elf-cleaner --api-level 24 "${OUTPUT_PATH}" >/dev/null 2>&1 || true
fi

END_TIME=$(date +%s%N 2>/dev/null || date +%s)
ELAPSED=$(( (END_TIME - START_TIME) / 1000000 )) 2>/dev/null || ELAPSED="?"

log_ok "Build complete -> ${OUTPUT_PATH} (${ELAPSED} ms)"

# Print Binary Size
if command -v du >/dev/null 2>&1; then
    SIZE=$(du -h "${OUTPUT_PATH}" | awk '{print $1}')
    log_info "Binary Size: ${SIZE}"
fi

# Run binary if requested
if [ "${SHOULD_RUN:-false}" = true ]; then
    log_info "Executing ${OUTPUT_PATH}...\n"
    "${OUTPUT_PATH}"
fi
