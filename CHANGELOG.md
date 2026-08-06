# Changelog

All notable changes to **Tinoc** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- (nothing yet)

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
