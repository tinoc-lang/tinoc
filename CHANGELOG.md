# Changelog

All notable changes to **Tinoc** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Installers**: `install.sh` / `install.ps1` fetch the latest GitHub release,
  verify it against the release `SHA256SUMS` manifest, and extract it into
  `~/.tinoc/` — with `VERSION` tracking, update checks that ask before
  upgrading, `--local` source builds, `--check`, `--uninstall`, and
  `--version` / `--force` / `--dir` flags.
- **CI workflow** (`.github/workflows/ci.yml`) runs on every push and pull
  request: deps, fmt-check, vet, lint, race tests, and a release build
  (`build.sh ci` / `build.ps1 ci`) across ubuntu/macos/windows, plus an
  installer smoke test in `--local` mode.
- The release workflow now attaches a `SHA256SUMS` manifest to every release,
  so installers can verify downloads.
- **`.gitattributes`**: Go and shell sources are pinned to LF line endings on
  checkout everywhere, so `gofmt -l` no longer false-fails on Windows CI
  (where git used to check out CRLF).

### Fixed

- **Installers**: `--local` no longer prompts to "update" when the built
  version equals the installed version — the fresh source build is installed
  directly. Re-running the installer with the latest version already
  installed now exits before downloading anything.
- **Installers**: `--force`/`--yes` now actually reinstalls over an
  identical installed version instead of reporting "already installed".
- **CLI**: color output is only emitted when stdout is a real terminal, so
  piped commands like `tinoc version | awk ...` return plain text (this was
  breaking the installer's version parsing on systems with TERM set).
- **Version reporting**: `build.sh` / `build.ps1` now inject the version into
  `src.Version` via `-ldflags` (the old `-X main.version` target never
  existed, so release binaries always reported the hardcoded `0.1.0`).
- **Installers**: installed-version comparisons normalize a leading `v`, so a
  `VERSION` file written as `v0.1.0` (e.g. by an earlier installer) is
  correctly treated as equal to `0.1.0` instead of triggering a spurious
  "update available" prompt.
- **Installers**: `--verbose` now prints the download/fetch commands it runs.
- **Installers**: confirmation prompts now decline gracefully when there is no
  interactive terminal (CI, piped input, cron) instead of aborting the whole
  install with an error — the binary still installs and manual PATH
  instructions are printed.

## [0.1.0] - 2026-08-05

First public release. Tinoc transpiles to **C11** and ships a CLI with
`build`, `run`, and `check` subcommands (`-l/--lex`, `-a/--ast`,
`-c/--emit-c` pipeline cutoffs).

### Added

- **Core declarations**: `var`, `const`, `static var`, `static const` with
  explicit or inferred types, const-mutability enforcement, and
  redeclaration/shadowing checks.
- **Functions**: `fn name(params...) Ret { ... }` with forward calls,
  duplicate-parameter detection, argument count/type checking, return-type
  agreement, and missing-return analysis.
- **Structs**: `struct Name { ... }` with typed fields, struct literals
  (`Point { .x = 1.0, .y = 2.0 }`), field access, instance methods
  (`fn m(self ^Name, ...)` mutating through `self^.field`), static methods,
  struct-typed params/returns, and by-value copy semantics. Self-referencing
  structs (`struct Node { next ^Node; }`) work via pointers.
- **Pointers**: `^T` types, `&x` address-of, `x^` dereference, pointer
  params/returns, `null` comparison.
- **Control flow**: `if` / `else if` / `else`, `while`, range `for`
  (`for 0..10 |i| { ... }`), `break`, `continue`.
- **Literals & operators**: integers (decimal/hex/octal/binary with `_`
  separators), floats, strings, chars, bools, `null`; arithmetic,
  comparison, logical (`and`/`or`), and bitwise operators; wrapping
  arithmetic (`+%`, `-%`, `*%`).
- **C interop**:
  - `#importc "header.h" [as alias];` — parses real C headers via clang's
    JSON AST (or gcc's `-aux-info` fallback) for type-safe calls and
    constants.
  - `extern "C" fn name(.symbol)?(...) Ret;` — hand-declared C functions
    with automatic `str` → `char*` argument unwrapping.
- **Runtime header** (`tinoc.h`): C11 type aliases (`u8`..`i128`, `f32`/`f64`,
  `str`, `char32`), slice/optional/error-union helpers, and `_Generic`
  saturating arithmetic dispatch.
- **Samples**: `samples/00_*.tnc` through `samples/13_*.tnc` covering the
  supported feature set (including struct basics, methods, nested/pointer
  structs, and a scoreboard example).
- **Tooling**: `build.sh` / `build.ps1` (build, test, vet, fmt, lint, install,
  cross-compile), embedded `tinoc.h`, and a GitHub Actions release workflow
  that publishes binaries as `.tar.gz` / `.zip` archives on tag push.

### Changed

- Transpile target moved to **C11** (was C99) to enable `_Generic`-based
  runtime features and future type-system growth.
- Postfix `^` dereference now binds at postfix precedence, so expressions
  like `a * self^.x` parse correctly.

### Fixed

- Comparison operators now adapt untyped literals (`f32Val > 0.0`), matching
  the arithmetic branch.

### Security

- No known security issues.

[Unreleased]: https://github.com/tinoc-lang/tinoc/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tinoc-lang/tinoc/releases/tag/v0.1.0
