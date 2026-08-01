#!/usr/bin/env bash
set -e

BUILD_DIR="build"

# Detect OS (respects GOOS if explicitly set for cross-compiling)
if [ -n "$GOOS" ]; then
    TARGET_OS="$GOOS"
else
    UNAME_S="$(uname -s 2>/dev/null || echo "unknown")"
    case "$UNAME_S" in
        Linux*)               TARGET_OS="linux" ;;
        Darwin*)              TARGET_OS="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) TARGET_OS="windows" ;;
        FreeBSD*)             TARGET_OS="freebsd" ;;
        OpenBSD*)             TARGET_OS="openbsd" ;;
        *)                    TARGET_OS="linux" ;;
    esac
fi

# Set binary name extension
BINARY_NAME="tinoc"
if [ "$TARGET_OS" = "windows" ]; then
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

echo "Building ${BINARY_NAME} for target: ${TARGET_OS}..."
GOOS="${TARGET_OS}" go build -trimpath -ldflags="-s -w" -o "${OUTPUT_PATH}" main.go

echo "Build complete -> ${OUTPUT_PATH}"

# Print file size
if command -v du >/dev/null 2>&1; then
    SIZE=$(du -h "${OUTPUT_PATH}" | awk '{print $1}')
    echo "Size: ${SIZE}"
fi

