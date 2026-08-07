# Changelog

All notable changes to **Tinoc** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Enums**: `enum Name { Variant, Other(type), ... }` — fieldless
  variants compile to plain C enums; variants with payloads become
  tagged unions (tag + per-variant anonymous struct), so multi-field
  payloads never overlap. Enums support instance methods (`self
  ^Name`), static methods, equality, and being passed/returned like any
  other type.
- **Enum `switch`**: exhaustive enum switches (all variants listed, no
  `_` arm needed) with pattern binding — `Shape.Rect(w, h) => { ... }`
  binds the payload fields directly; `_` wildcards discard a payload
  slot. Missing-return analysis understands exhaustive enum switches.
- **Unions**: `union Name { field type; ... }` with C-style shared-memory
  fields — writing `as_int` and reading `as_float` reinterprets the same
  bytes (IEEE-754 punning works end-to-end). Unions reuse struct field
  syntax, support instance methods (`self ^Name`) and static methods,
  and reject comparisons/arithmetic with clear diagnostics. Generic
  unions (`union Pair:T`) are rejected with a "not yet supported"
  diagnostic, matching structs/enums.
- **Samples**: `samples/14_enum_basics.tnc` through
  `samples/17_str_strings.tnc` covering fieldless enums, tagged unions
  with pattern matching, enum methods, and `str` semantics, plus
  `samples/18_combo.tnc`, which exercises structs, enums (pattern
  matching), unions (type punning), `str`, `switch`, and methods all in
  one program.

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

- **Compiler**: calling an instance method that was declared without a
  `self` parameter (e.g. `fn make() Data { ... }` inside a struct/union
  body, invoked as `d.make()`) previously panicked with an
  index-out-of-range error; it now reports a proper "needs a self
  parameter" diagnostic.
- **str**: `<`, `>`, `<=`, `>=` on `str` are now rejected with a clear
  diagnostic (only `==`/`!=` are defined, via content comparison) instead
  of emitting invalid C. Switching on a `str` is likewise a proper
  semantic error. `str ==`/`!=` compare by content through the
  `tinoc_str_eq` runtime helper.
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
- **Installers**: the update prompt now appears *before* the download starts,
  so a declined update never wastes bandwidth on a full release download.
- **install.ps1** now detects the real OS and architecture (linux/darwin/
  windows × amd64/arm64) and installs the matching binary name, making it
  fully cross-platform under PowerShell 7+ — matching `install.sh`. It also
  prefers the repo's `build.sh` when running `-Local` on macOS/Linux.
- **build.ps1**: OS/architecture detection now matches `build.sh`, so
  `install.ps1 -Local` on macOS/Linux cross-targets correctly.
- **install.sh**: archive binary discovery no longer relies on the GNU-only
  `find -maxdepth`, which errored on macOS's BSD find; it also offers to
  create a missing shell rc file when adding `~/.tinoc/bin` to PATH.

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
