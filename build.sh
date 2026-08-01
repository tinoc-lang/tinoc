#!/usr/bin/env bash
#
# Tinoc build script.
# Usage: ./build.sh [command] [flags]
# Run './build.sh help' for the full command list.

set -euo pipefail

BUILD_DIR="build"
DIST_DIR="dist"
COVER_DIR="coverage"
BINARY_BASE="tinoc"
MODULE_PATH="main.go"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
TEST_TIMEOUT="${TEST_TIMEOUT:-120s}"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
GO_VERSION="$(go version 2>/dev/null | awk '{print $3}' || echo unknown)"

MODE="release"
VERBOSE=0
RACE=0

# --- colors ------------------------------------------------------------
# Disabled automatically when stdout is not a terminal, or when NO_COLOR
# is set (see https://no-color.org).
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_RESET="\033[0m"; C_BOLD="\033[1m"; C_DIM="\033[2m"
    C_RED="\033[31m"; C_GREEN="\033[32m"; C_YELLOW="\033[33m"
    C_BLUE="\033[34m"; C_CYAN="\033[36m"
else
    C_RESET=""; C_BOLD=""; C_DIM=""
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""
fi

info()  { printf "${C_BLUE}==>${C_RESET} ${C_BOLD}%s${C_RESET}\n" "$1"; }
step()  { printf "${C_CYAN}  ->${C_RESET} %s\n" "$1"; }
ok()    { printf "${C_GREEN}OK${C_RESET}   %s\n" "$1"; }
warn()  { printf "${C_YELLOW}WARN${C_RESET} %s\n" "$1"; }
error() { printf "${C_RED}FAIL${C_RESET} %s\n" "$1" >&2; }
kv()    { printf "  ${C_DIM}%-10s${C_RESET} %s\n" "$1" "$2"; }

# --- platform detection -------------------------------------------------

detect_os() {
    if [ -n "${GOOS:-}" ]; then echo "$GOOS"; return; fi
    case "$(uname -s 2>/dev/null || echo unknown)" in
        Linux*)               echo "linux" ;;
        Darwin*)              echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        FreeBSD*)             echo "freebsd" ;;
        OpenBSD*)             echo "openbsd" ;;
        *)                    echo "linux" ;;
    esac
}

detect_arch() {
    if [ -n "${GOARCH:-}" ]; then echo "$GOARCH"; return; fi
    case "$(uname -m 2>/dev/null || echo unknown)" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        i386|i686)     echo "386" ;;
        *)             echo "amd64" ;;
    esac
}

binary_name() {
    if [ "$1" = "windows" ]; then echo "${BINARY_BASE}.exe"; else echo "${BINARY_BASE}"; fi
}

ldflags() {
    local flags="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}"
    if [ "$1" = "release" ]; then flags="-s -w ${flags}"; fi
    echo "$flags"
}

check_go() {
    if ! command -v go >/dev/null 2>&1; then
        error "Go toolchain not found. Install it from https://go.dev/dl/"
        exit 1
    fi
}

require_module() {
    if [ ! -f "$MODULE_PATH" ] && [ ! -f "go.mod" ]; then
        error "No go.mod or ${MODULE_PATH} found in $(pwd). Run this script from the project root."
        exit 1
    fi
}

print_build_summary() {
    kv "Version" "$VERSION"
    kv "Commit"  "$COMMIT"
    kv "Target"  "${1}/${2}"
    kv "Mode"    "$3"
    kv "Go"      "$GO_VERSION"
    kv "Output"  "$4"
}

# --- commands ------------------------------------------------------------

cmd_build() {
    check_go
    require_module
    local os arch out
    os="$(detect_os)"; arch="$(detect_arch)"
    mkdir -p "${BUILD_DIR}"
    out="${BUILD_DIR}/$(binary_name "$os")"

    info "Building tinoc"
    print_build_summary "$os" "$arch" "$MODE" "$out"
    echo

    local build_args=(build -trimpath -ldflags="$(ldflags "$MODE")" -o "$out" "$MODULE_PATH")
    if [ "$MODE" = "debug" ]; then
        build_args=(build -trimpath -gcflags="all=-N -l" -ldflags="$(ldflags "$MODE")" -o "$out" "$MODULE_PATH")
    fi

    if [ "$VERBOSE" = "1" ]; then
        step "GOOS=${os} GOARCH=${arch} go ${build_args[*]}"
    fi

    if GOOS="$os" GOARCH="$arch" go "${build_args[@]}"; then
        ok "Build succeeded"
        if command -v du >/dev/null 2>&1; then
            kv "Size" "$(du -h "$out" | awk '{print $1}')"
        fi
    else
        error "Build failed"
        exit 1
    fi
}

cmd_build_all() {
    check_go
    require_module
    info "Cross-compiling tinoc for all platforms"
    mkdir -p "${DIST_DIR}"

    local targets=("linux amd64" "linux arm64" "darwin amd64" "darwin arm64" "windows amd64")
    local failures=0

    for target in "${targets[@]}"; do
        read -r os arch <<< "$target"
        local ext="" out
        [ "$os" = "windows" ] && ext=".exe"
        out="${DIST_DIR}/${BINARY_BASE}-${os}-${arch}${ext}"
        step "${os}/${arch}"
        if GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="$(ldflags release)" -o "$out" "$MODULE_PATH" 2>/tmp/tinoc_build_err; then
            ok "  ${out} ($(du -h "$out" 2>/dev/null | awk '{print $1}'))"
        else
            error "  ${os}/${arch} failed"
            cat /tmp/tinoc_build_err >&2
            failures=$((failures + 1))
        fi
    done

    echo
    if [ "$failures" -eq 0 ]; then
        ok "Cross-compile complete -> ${DIST_DIR}/"
    else
        error "${failures} target(s) failed"
        exit 1
    fi
}

cmd_run() {
    check_go
    require_module
    info "Running tinoc"
    go run "$MODULE_PATH" "${EXTRA_ARGS[@]:-}"
}

cmd_test() {
    check_go
    require_module
    info "Running test suite"
    local test_args=(test -timeout "$TEST_TIMEOUT" ./...)
    [ "$RACE" = "1" ]    && test_args=(test -timeout "$TEST_TIMEOUT" -race ./...)
    [ "$VERBOSE" = "1" ] && test_args+=(-v)

    if go "${test_args[@]}"; then
        ok "All tests passed"
    else
        error "Tests failed"
        exit 1
    fi
}

cmd_cover() {
    check_go
    require_module
    info "Running tests with coverage"
    mkdir -p "${COVER_DIR}"
    local profile="${COVER_DIR}/coverage.out"

    if go test -timeout "$TEST_TIMEOUT" -covermode=atomic -coverprofile="$profile" ./...; then
        ok "Coverage profile written -> ${profile}"
        go tool cover -func="$profile" | tail -n 1
        go tool cover -html="$profile" -o "${COVER_DIR}/coverage.html"
        ok "HTML report -> ${COVER_DIR}/coverage.html"
    else
        error "Tests failed, no coverage report generated"
        exit 1
    fi
}

cmd_vet() {
    check_go
    require_module
    info "Running go vet"
    if go vet ./...; then
        ok "Vet passed"
    else
        error "Vet found issues"
        exit 1
    fi
}

cmd_fmt() {
    check_go
    info "Formatting source"
    local unformatted
    unformatted="$(gofmt -l .)"
    if [ -n "$unformatted" ]; then
        gofmt -l -w .
        step "Reformatted:"
        echo "$unformatted" | sed 's/^/    /'
    fi
    ok "Formatting complete"
}

cmd_fmt_check() {
    check_go
    info "Checking formatting"
    local unformatted
    unformatted="$(gofmt -l .)"
    if [ -n "$unformatted" ]; then
        error "The following files are not gofmt'd:"
        echo "$unformatted" | sed 's/^/    /' >&2
        error "Run './build.sh fmt' to fix"
        exit 1
    fi
    ok "All files formatted correctly"
}

cmd_lint() {
    if command -v golangci-lint >/dev/null 2>&1; then
        info "Running golangci-lint"
        if golangci-lint run ./...; then
            ok "Lint passed"
        else
            error "Lint found issues"
            exit 1
        fi
    else
        warn "golangci-lint not installed, skipping (see https://golangci-lint.run)"
    fi
}

cmd_deps() {
    check_go
    require_module
    info "Verifying module dependencies"
    go mod tidy
    go mod verify
    ok "Dependencies tidy and verified"
}

cmd_check() {
    info "Running pre-commit checks"
    cmd_fmt_check
    cmd_vet
    cmd_lint
    cmd_test
    ok "All checks passed"
}

cmd_install() {
    cmd_build
    local os out dest
    os="$(detect_os)"
    out="${BUILD_DIR}/$(binary_name "$os")"
    dest="${INSTALL_DIR}/$(binary_name "$os")"
    info "Installing to ${dest}"
    mkdir -p "${INSTALL_DIR}" 2>/dev/null || true
    if cp "$out" "$dest" 2>/tmp/tinoc_install_err; then
        chmod +x "$dest"
        ok "Installed -> ${dest}"
    else
        error "Install failed (try: sudo INSTALL_DIR=${INSTALL_DIR} ./build.sh install)"
        cat /tmp/tinoc_install_err >&2
        exit 1
    fi
}

cmd_clean() {
    rm -rf "${BUILD_DIR}" "${DIST_DIR}" "${COVER_DIR}"
    ok "Cleaned ${BUILD_DIR}/, ${DIST_DIR}/, and ${COVER_DIR}/"
}

cmd_ci() {
    info "Running CI pipeline"
    cmd_deps
    cmd_fmt_check
    cmd_vet
    cmd_lint
    RACE=1
    cmd_test
    MODE="release"
    cmd_build
    ok "CI pipeline complete"
}

cmd_version() {
    kv "Version" "$VERSION"
    kv "Commit"  "$COMMIT"
    kv "Built"   "$BUILD_DATE"
    kv "Go"      "$GO_VERSION"
}

cmd_help() {
    printf '%b\n' "${C_BOLD}Tinoc build script${C_RESET}"
    printf '\n'
    printf '%b\n' "${C_BOLD}Usage:${C_RESET}"
    printf '  ./build.sh [command] [flags]\n'
    printf '\n'
    printf '%b\n' "${C_BOLD}Commands:${C_RESET}"
    printf '  %-13s %s\n' \
        "build"     "Build the tinoc binary for the current platform (default)" \
        "build-all" "Cross-compile for linux, darwin, windows (amd64/arm64) into dist/" \
        "run"       "Build and run tinoc directly via 'go run'" \
        "test"      "Run the test suite" \
        "cover"     "Run tests with coverage and generate an HTML report" \
        "vet"       "Run 'go vet'" \
        "fmt"       "Format source with gofmt, in place" \
        "fmt-check" "Check formatting without modifying files (used in CI)" \
        "lint"      "Run golangci-lint, if installed" \
        "deps"      "Tidy and verify module dependencies" \
        "check"     "Run fmt-check, vet, lint, and test (fast pre-commit gate)" \
        "install"   "Build and install to \$INSTALL_DIR (default: /usr/local/bin)" \
        "ci"        "Full pipeline: deps, fmt-check, vet, lint, race tests, build" \
        "clean"     "Remove build/, dist/, and coverage/" \
        "version"   "Print version, commit, and toolchain info" \
        "help"      "Show this message"
    printf '\n'
    printf '%b\n' "${C_BOLD}Flags:${C_RESET}"
    printf '  %-13s %s\n' \
        "--debug"   "Build with debug symbols, skip optimization stripping" \
        "--race"    "Enable the race detector for 'test'" \
        "--verbose" "Print the underlying commands being run"
    printf '\n'
    printf '%b\n' "${C_BOLD}Environment:${C_RESET}"
    printf '  %-14s %s\n' \
        "GOOS, GOARCH" "Override target platform/architecture" \
        "VERSION"      "Override the injected version string" \
        "INSTALL_DIR"  "Override install destination" \
        "TEST_TIMEOUT" "Override test timeout (default: 120s)" \
        "NO_COLOR"     "Disable colored output"
    printf '\n'
    printf '%b\n' "${C_BOLD}Examples:${C_RESET}"
    printf '  ./build.sh\n'
    printf '  ./build.sh build --debug\n'
    printf '  ./build.sh test --race\n'
    printf '  GOOS=windows GOARCH=arm64 ./build.sh build\n'
    printf '  ./build.sh check\n'
    printf '  ./build.sh ci\n'
}

# --- argument parsing ----------------------------------------------------

VALID_COMMANDS="build build-all run test cover vet fmt fmt-check lint deps check install clean ci version help"

COMMAND="build"
EXTRA_ARGS=()

if [ $# -gt 0 ]; then
    case " ${VALID_COMMANDS} " in
        *" $1 "*) COMMAND="$1"; shift ;;
        -h|--help) COMMAND="help"; shift ;;
    esac
fi

while [ $# -gt 0 ]; do
    case "$1" in
        --debug)      MODE="debug" ;;
        --race)       RACE=1 ;;
        --verbose)    VERBOSE=1 ;;
        -h|--help)    COMMAND="help" ;;
        *)            EXTRA_ARGS+=("$1") ;;
    esac
    shift
done

case "$COMMAND" in
    build)      cmd_build ;;
    build-all)  cmd_build_all ;;
    run)        cmd_run ;;
    test)       cmd_test ;;
    cover)      cmd_cover ;;
    vet)        cmd_vet ;;
    fmt)        cmd_fmt ;;
    fmt-check)  cmd_fmt_check ;;
    lint)       cmd_lint ;;
    deps)       cmd_deps ;;
    check)      cmd_check ;;
    install)    cmd_install ;;
    clean)      cmd_clean ;;
    ci)         cmd_ci ;;
    version)    cmd_version ;;
    help)       cmd_help ;;
    *)
        error "Unknown command: ${COMMAND}"
        echo
        cmd_help
        exit 1
        ;;
esac
