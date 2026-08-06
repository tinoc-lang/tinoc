#!/usr/bin/env bash
#
# Tinoc installer.                             # Usage: ./install.sh [flags]
# Fetches the latest release from GitHub and installs it into ~/.tinoc
# (override with $TINOC_HOME or --dir). Run './install.sh --help' for flags.
#
# The installer is a companion to build.sh/build.ps1: pass --local to build
# from source with ./build.sh build instead of downloading a release.

set -euo pipefail

# --- defaults --------------------------------------------------------------
TINOC_REPO="${TINOC_REPO:-tinoc-lang/tinoc}"
TINOC_API_BASE="${TINOC_API_BASE:-https://api.github.com}"
TINOC_DOWNLOAD_BASE="${TINOC_DOWNLOAD_BASE:-https://github.com}"
TINOC_HOME="${TINOC_HOME:-$HOME/.tinoc}"

LOCAL=0
FORCE=0
CHECK=0
UNINSTALL=0
VERBOSE=0
SPEC_VERSION=""

# --- colors ------------------------------------------------------------
# Same palette and helpers as build.sh. Disabled when stdout is not a
# terminal or NO_COLOR is set (see https://no-color.org).
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

# --- platform detection (mirrors build.sh) --------------------------------
detect_os() {
    if [ -n "${GOOS:-}" ]; then echo "$GOOS"; return; fi
    case "$(uname -s 2>/dev/null || echo unknown)" in
        Linux*)               echo "linux" ;;
        Darwin*)              echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)                    echo "linux" ;;
    esac
}

detect_arch() {
    if [ -n "${GOARCH:-}" ]; then echo "$GOARCH"; return; fi
    case "$(uname -m 2>/dev/null || echo unknown)" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             echo "amd64" ;;
    esac
}

# --- small helpers ---------------------------------------------------------
has() { command -v "$1" >/dev/null 2>&1; }

norm_version() { printf '%s\n' "$1" | sed 's/^[vV]//'; }

installed_version() {
    [ -f "$TINOC_HOME/VERSION" ] && cat "$TINOC_HOME/VERSION" || echo ""
}

binary_name() { if [ "$1" = "windows" ]; then echo "tinoc.exe"; else echo "tinoc"; fi; }

# Asset published by the release workflow: tinoc-<os>-<arch>.tar.gz / .zip,
# containing a binary named tinoc-<os>-<arch>[.exe].
asset_name() {
    local os="$1" arch="$2"
    if [ "$os" = "windows" ]; then
        echo "tinoc-${os}-${arch}.zip"
    else
        echo "tinoc-${os}-${arch}.tar.gz"
    fi
}

supported_target() {
    local os="$1" arch="$2"
    case "${os}/${arch}" in
        linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) return 0 ;;
    esac
    return 1
}

download_file() { # url dest
    local url="$1" dest="$2"
    if has curl; then
        curl --fail --silent --show-error --location --output "$dest" "$url"
    elif has wget; then
        wget --quiet --output-document="$dest" "$url"
    else
        error "No HTTP download tool found (need curl or wget)"
        exit 1
    fi
}

fetch_stdout() { # url
    if has curl; then
        curl --fail --silent --show-error --location "$1"
    elif has wget; then
        wget --quiet --output-document=- "$1"
    else
        error "No HTTP download tool found (need curl or wget)"
        exit 1
    fi
}

sha256_of() { # file
    if has sha256sum; then
        sha256sum "$1" | awk '{print $1}'
    elif has shasum; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        echo ""
    fi
}

# Ask the user to confirm; --yes/--force skips the prompt (Starship pattern:
# read from /dev/tty so piping the script to bash still prompts correctly).
confirm() {
    if [ "$FORCE" = "1" ]; then return 0; fi
    printf "${C_BOLD}?${C_RESET} %s ${C_BOLD}[y/N]${C_RESET} " "$1"
    local yn
    if ! read -r yn < /dev/tty; then
        echo
        error "Cannot read confirmation (re-run with --yes)"
        exit 1
    fi
    case "$yn" in
        y|Y|yes|YES) return 0 ;;
        *)            return 1 ;;
    esac
}

path_has() { # dir
    local dir="$1" p
    local IFS=:
    for p in $PATH; do
        if [ "$p" = "$dir" ]; then return 0; fi
    done
    return 1
}

# --- commands --------------------------------------------------------------
usage() {
    printf '%b\n' "${C_BOLD}Tinoc installer${C_RESET}"
    printf '\n'
    printf '%b\n' "${C_BOLD}Usage:${C_RESET}"
    printf '  ./install.sh [flags]\n'
    printf '\n'
    printf '%b\n' "${C_BOLD}Flags:${C_RESET}"
    printf '  %-16s %s\n' \
        "--local"    "Build with ./build.sh build and install the local binary" \
        "--check"    "Compare installed version with the latest release, make no changes" \
        "--uninstall" "Remove the entire \$TINOC_HOME install" \
        "--version"  "Install a specific release version (e.g. --version 0.1.0)" \
        "--force, --yes" "Skip all confirmation prompts" \
        "--dir"      "Override install directory (default: \$HOME/.tinoc)" \
        "--repo"     "Override the GitHub repository (default: ${TINOC_REPO})" \
        "--verbose"  "Print the underlying commands being run" \
        "-h, --help" "Show this message"
    printf '\n'
    printf '%b\n' "${C_BOLD}Environment:${C_RESET}"
    printf '  %-16s %s\n' \
        "TINOC_HOME"     "Install directory (default: \$HOME/.tinoc)" \
        "TINOC_REPO"     "GitHub repository (default: tinoc-lang/tinoc)" \
        "TINOC_API_BASE" "GitHub API base URL (default: https://api.github.com)" \
        "NO_COLOR"       "Disable colored output"
    printf '\n'
    printf '%b\n' "${C_BOLD}Examples:${C_RESET}"
    printf '  ./install.sh\n'
    printf '  ./install.sh --check\n'
    printf '  ./install.sh --local\n'
    printf '  ./install.sh --version 0.1.0 --yes\n'
    printf '  ./install.sh --uninstall\n'
}

# Shared install step: put $1 (the binary) at $BIN_DIR/tinoc, write VERSION,
# then verify it runs and offer PATH setup. $3 (optional) is a temp dir to
# clean up on the early-exit paths (already installed / aborted).
install_binary() { # src_binary version [cleanup_dir]
    local src="$1" version="$2" exe
    exe="$(binary_name "$(detect_os)")"
    mkdir -p "$TINOC_HOME" "$TINOC_HOME/bin"

    local prev
    prev="$(installed_version)"
    if [ -n "$prev" ]; then
        if [ "$prev" = "$version" ]; then
            rm -rf "${3:-}"
            ok "tinoc v${version} is already installed (${TINOC_HOME}/bin/${exe})"
            info "Nothing to do — run './install.sh --force' to reinstall"
            exit 0
        fi
        warn "New version available: v${version} (installed v${prev})"
        if ! confirm "Update tinoc from v${prev} to v${version}?"; then
            rm -rf "${3:-}"
            info "Aborted — keeping v${prev}"
            exit 0
        fi
    fi

    info "Installing tinoc v${version}"
    local tmp
    tmp="$(mktemp -d "${TINOC_HOME}/.install.XXXXXX")"
    cp "$src" "${tmp}/${exe}"
    chmod +x "${tmp}/${exe}"

    # Atomic same-filesystem move, then record the version.
    if mv -f "${tmp}/${exe}" "${TINOC_HOME}/bin/${exe}"; then
        printf '%s\n' "$version" > "$TINOC_HOME/VERSION"
        rm -rf "$tmp"
    else
        rm -rf "$tmp"
        error "Failed to install binary to ${TINOC_HOME}/bin/${exe}"
        exit 1
    fi

    # Sanity check: the installed binary must run.
    if "${TINOC_HOME}/bin/${exe}" version >/dev/null 2>&1; then
        ok "Installed tinoc v${version} -> ${TINOC_HOME}/bin/${exe}"
    else
        error "Installed binary failed to run — the release may be corrupt"
        exit 1
    fi
    maybe_setup_path
}

maybe_setup_path() {
    local bin_dir="$TINOC_HOME/bin"
    if path_has "$bin_dir"; then return; fi

    step "${bin_dir} is not on your PATH"
    local rc=""
    case "${SHELL:-}" in
        *fish) rc="$HOME/.config/fish/config.fish" ;;
        *zsh)  rc="$HOME/.zshrc" ;;
        *)     rc="$HOME/.bashrc" ;;
    esac

    if [ -w "$rc" ] && confirm "Add '${bin_dir}' to your PATH in ${rc}?"; then
        {
            printf '\n# tinoc\n'
            if [[ "$rc" == *fish ]]; then
                printf 'set -gx PATH %s $PATH\n' "$bin_dir"
            else
                printf 'export PATH="%s:$PATH"\n' "$bin_dir"
            fi
        } >> "$rc"
        ok "PATH updated in ${rc} — restart your shell or run: source ${rc}"
    else
        info "Add it manually to your shell config:"
        kv "export" "PATH=\"${bin_dir}:\$PATH\""
    fi
}

fetch_latest_tag() { # -> tag like v0.1.0
    local url="${TINOC_API_BASE%/}/repos/${TINOC_REPO}/releases/latest"
    local json tag
    if ! json="$(fetch_stdout "$url" 2>/dev/null)"; then
        error "Could not fetch the latest release from GitHub"
        error "  ${url}"
        error "Check your network connection, or install a specific version with --version"
        exit 1
    fi
    tag="$(printf '%s\n' "$json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    if [ -z "$tag" ]; then
        error "GitHub returned no release for ${TINOC_REPO} — no releases published yet?"
        error "Build from source instead: ./install.sh --local"
        exit 1
    fi
    printf '%s\n' "$tag"
}

cmd_remote() {
    if ! has tar && ! has unzip; then
        error "Neither tar nor unzip found — one is required to extract releases"
        exit 1
    fi

    local os arch ext tag version asset
    os="$(detect_os)"; arch="$(detect_arch)"
    if ! supported_target "$os" "$arch"; then
        error "No prebuilt release for ${os}/${arch}"
        error "Supported: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64"
        error "Build from source instead: ./install.sh --local"
        exit 1
    fi

    if [ -n "$SPEC_VERSION" ]; then
        version="$(norm_version "$SPEC_VERSION")"
        tag="v${version}"
    else
        tag="$(fetch_latest_tag)"
        version="$(norm_version "$tag")"
    fi

    ext="tar.gz"; [ "$os" = "windows" ] && ext="zip"
    asset="$(asset_name "$os" "$arch")"
    base="${TINOC_DOWNLOAD_BASE%/}/${TINOC_REPO}/releases/download/${tag}"

    info "Fetching tinoc v${version} from GitHub"
    kv "Version" "v${version}"
    kv "Platform" "${os}/${arch}"
    kv "Install"  "$TINOC_HOME"
    kv "Asset"    "${base}/${asset}"

    mkdir -p "$TINOC_HOME"
    local tmp
    tmp="$(mktemp -d "${TINOC_HOME}/.install.XXXXXX")"

    step "Downloading ${asset}"
    if ! download_file "${base}/${asset}" "${tmp}/archive"; then
        rm -rf "$tmp"
        error "Download failed — does release ${tag} contain '${asset}'?"
        exit 1
    fi

    # Verify against the release SHA256SUMS manifest when present.
    local want got
    if download_file "${base}/SHA256SUMS" "${tmp}/sha" 2>/dev/null; then
        want="$(awk -v f="$asset" '$2 == f {print $1}' "${tmp}/sha")"
        got="$(sha256_of "${tmp}/archive")"
        if [ -n "$want" ] && [ -n "$got" ] && [ "$want" = "$got" ]; then
            ok "Checksum verified (sha256)"
        else
            rm -rf "$tmp"
            error "Checksum mismatch for ${asset}"
            error "  expected: ${want:-<missing>}"
            error "  actual:   ${got:-<unknown>}"
            exit 1
        fi
    else
        warn "No SHA256SUMS manifest on this release — skipping checksum verification"
    fi

    step "Extracting ${asset}"
    local bin_dir x
    mkdir -p "${tmp}/x"
    case "$ext" in
        zip)    unzip -qo "${tmp}/archive" -d "${tmp}/x" ;;
        tar.gz) tar -xzf "${tmp}/archive" -C "${tmp}/x" ;;
    esac

    # The archive contains a binary named tinoc-<os>-<arch>[.exe]; installed
    # as a plain tinoc/tinoc.exe under $TINOC_HOME/bin.
    bin_dir="$(find "${tmp}/x" -maxdepth 1 -type f -name 'tinoc-*' 2>/dev/null | head -n 1)"
    if [ -z "$bin_dir" ]; then
        bin_dir="$(find "${tmp}/x" -maxdepth 2 -type f -name 'tinoc*' 2>/dev/null | head -n 1)"
    fi
    if [ -z "$bin_dir" ]; then
        rm -rf "$tmp"
        error "No tinoc binary found inside ${asset}"
        exit 1
    fi

    install_binary "$bin_dir" "$version" "$tmp"
    rm -rf "$tmp"
}

cmd_local() {
    if [ ! -f "build.sh" ] && [ ! -f "build.ps1" ]; then
        error "No build.sh/build.ps1 found in $(pwd)"
        error "Run './install.sh --local' from the repository root"
        exit 1
    fi

    local exe bin version
    info "Building tinoc from source"
    if [ -f "build.sh" ]; then
        ./build.sh build
    else
        warn "Windows: run './install.ps1 --local' instead for the PowerShell flow"
        ./build.ps1 build
    fi

    exe="$(binary_name "$(detect_os)")"
    bin="build/${exe}"
    if [ ! -f "$bin" ]; then
        error "Build did not produce ${bin}"
        exit 1
    fi

    version="$("$bin" version 2>/dev/null | awk '{print $3}')"
    [ -z "$version" ] && version="dev"
    install_binary "$bin" "$version"
}

cmd_check() {
    local os arch latest installed
    os="$(detect_os)"; arch="$(detect_arch)"
    installed="$(installed_version)"
    if [ -z "$installed" ]; then
        info "tinoc is not installed (${TINOC_HOME})"
        info "Install the latest release with: ./install.sh"
        return 0
    fi
    info "Checking for updates (${os}/${arch})"
    kv "Installed" "v${installed}"

    local tag
    tag="$(fetch_latest_tag)"
    latest="$(norm_version "$tag")"
    kv "Latest" "v${latest}"

    if [ "$installed" = "$latest" ]; then
        ok "tinoc is up to date (v${latest})"
    else
        warn "Update available: v${latest} (installed v${installed})"
        info "Run './install.sh' to update"
    fi
}

cmd_uninstall() {
    if [ ! -d "$TINOC_HOME" ]; then
        info "Nothing to uninstall — ${TINOC_HOME} does not exist"
        return 0
    fi
    info "This will remove ${TINOC_HOME} (binaries, VERSION, and data)"
    if confirm "Remove tinoc from ${TINOC_HOME}?"; then
        rm -rf "$TINOC_HOME"
        ok "Uninstalled tinoc (${TINOC_HOME})"
        info "Remove the PATH entry from your shell config if you added it"
    else
        info "Aborted — nothing was removed"
    fi
}

# --- argument parsing (build.sh style) ------------------------------------
while [ $# -gt 0 ]; do
    case "$1" in
        --local)     LOCAL=1 ;;
        --force|--yes|-f|-y) FORCE=1 ;;
        --check)     CHECK=1 ;;
        --uninstall) UNINSTALL=1 ;;
        --verbose)   VERBOSE=1 ;;
        --version|-v) SPEC_VERSION="${2:-}"; shift ;;
        --dir|--prefix) TINOC_HOME="${2:-}"; shift ;;
        --repo)      TINOC_REPO="${2:-}"; shift ;;
        -h|--help)   usage; exit 0 ;;
        *)
            error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
    shift
done

if [ -z "$TINOC_HOME" ]; then
    error "--dir requires a path"
    exit 1
fi

if [ "$UNINSTALL" = "1" ]; then
    cmd_uninstall
elif [ "$CHECK" = "1" ]; then
    cmd_check
elif [ "$LOCAL" = "1" ]; then
    if [ -n "$SPEC_VERSION" ]; then
        error "--version cannot be combined with --local"
        exit 1
    fi
    cmd_local
else
    cmd_remote
fi
