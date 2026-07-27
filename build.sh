#!/usr/bin/env bash
set -e

BUILD_DIR="build"

# Detect OS (respects TARGET_OS or ODIN_TARGET if explicitly set)
if [ -n "$TARGET_OS" ]; then
    OS="$TARGET_OS"
elif [ -n "$ODIN_TARGET" ]; then
    OS="$ODIN_TARGET"
else
    UNAME_S="$(uname -s 2>/dev/null || echo "unknown")"
    case "$UNAME_S" in
        Linux*)               OS="linux" ;;
        Darwin*)              OS="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
        FreeBSD*)             OS="freebsd" ;;
        OpenBSD*)             OS="openbsd" ;;
        *)                    OS="linux" ;;
    esac
fi

# Set binary name extension
BINARY_NAME="tinoc"
if [ "$OS" = "windows" ]; then
    BINARY_NAME="tinoc.exe"
fi

OUTPUT_PATH="${BUILD_DIR}/${BINARY_NAME}"

# Handle clean
if [ "$1" = "clean" ]; then
    rm -rf "${BUILD_DIR}"
    echo "Cleaned ${BUILD_DIR}/"
    exit 0
fi

# Build steps
mkdir -p "${BUILD_DIR}"

echo "Building ${BINARY_NAME} for target: ${OS}..."

# Build package in current directory with speed optimizations
odin build . -out:"${OUTPUT_PATH}" -o:speed -target:"${OS}"

echo "Build complete -> ${OUTPUT_PATH}"

# Print file size
if command -v du >/dev/null 2>&1; then
    SIZE=$(du -h "${OUTPUT_PATH}" | awk '{print $1}')
    echo "Size: ${SIZE}"
fi
