<p align="center">
  <img src="banner.png" alt="Tinoc Banner" width="100%">
</p>

<h1 align="center">Tinoc</h1>

<p align="center">
  <strong>This Is Not C</strong><br>
  A modern systems programming language that transpiles to C11.
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/tinoc-lang/tinoc?style=flat-square" alt="Release">
  <img src="https://img.shields.io/github/license/tinoc-lang/tinoc?style=flat-square" alt="License">
  <img src="https://img.shields.io/github/stars/tinoc-lang/tinoc?style=flat-square" alt="Stars">
  <img src="https://img.shields.io/github/forks/tinoc-lang/tinoc?style=flat-square" alt="Forks">
  <img src="https://img.shields.io/github/issues/tinoc-lang/tinoc?style=flat-square" alt="Issues">
  <img src="https://img.shields.io/github/last-commit/tinoc-lang/tinoc?style=flat-square" alt="Last Commit">
  <img src="https://img.shields.io/github/languages/top/tinoc-lang/tinoc?style=flat-square" alt="Top Language">
</p>

---

## Table of Contents

- [About](#about)
- [Language Support](#language-support)
- [Example Program](#example-program)
- [Changelog](#changelog)
- [Release Checklist](#release-checklist)
- [Important Links](#important-links)
- [Nix (reproducible dev & CI)](#nix-reproducible-dev--ci)
- [Contributing](#contributing)
- [License](#license)

---

## About

> This is the official GitHub repository for the **TinocLang** compiler source code.

## **T**inoc **I**s **No**t **C**

Tinoc is a short form of **T**his **I**s **No**t **C**, commonly known as **T**inoc **I**s **No**t **C**.

**NOTE:** Tinoc respects C and its usage. Tinoc is built on the philosophy that programming should be **Meaningful**, **Accurate**, **Robust**, **Maximum Performance**, and **Simple**.

**Tinoc** transpiles to C11 for maximum system support — C11 unlocks features like `_Generic` dispatch in the `tinoc.h` runtime (saturating arithmetic) and gives the compiler room to grow (error unions, typed enums, generics) without fighting the target language.

---

## Installation

Install the **latest release** binary into `~/.tinoc` — no root needed:

```bash
curl -fsSL https://raw.githubusercontent.com/tinoc-lang/tinoc/main/install.sh | bash
```

PowerShell:

```powershell
irm https://raw.githubusercontent.com/tinoc-lang/tinoc/main/install.ps1 | iex
```

The installers (`install.sh` / `install.ps1`) are companions to
[`build.sh`](build.sh) / [`build.ps1`](build.ps1):

- Fetch the **latest release** from GitHub, verify it against the release
  `SHA256SUMS` manifest, and extract it into `~/.tinoc/` (or `$TINOC_HOME`).
- Write a `VERSION` file next to the install. Re-running the installer checks
  for updates and **asks before upgrading**.
- `--local` builds from source with `./build.sh build` (or `./build.ps1 build`)
  and installs the local binary instead of downloading.
- `install.ps1` is fully cross-platform too: it detects the OS/architecture
  just like `install.sh`, so it works on macOS/Linux with PowerShell 7+ as
  well as on Windows.
- Offer to add `~/.tinoc/bin` to your `PATH` after installing.

Common flags: `--check` (compare installed vs latest), `--uninstall`,
`--version 0.1.1` (specific release), `--force`/`--yes` (skip prompts),
`--dir <path>` (override install dir). Run `install.sh --help` for the full list.

---

## Language Support

What the compiler supports end-to-end in the current release:

| Feature | Status | Notes |
| --- | --- | --- |
| `var` / `const` (incl. `static`) | ✅ | Explicit or inferred types; const-mutability enforced |
| `fn` functions | ✅ | Forward calls, argument/return type checking, missing-return checks |
| `struct` + methods | ✅ | Fields, struct literals (`Point { .x = 1.0 }`), instance methods (`self ^Point`), static methods, by-value copy/params/returns |
| `enum` + methods | ✅ | Fieldless variants (`Direction.North`) and tagged unions with payloads (`Shape.Circle(r)`); instance/static methods; exhaustive `switch` with pattern binding (`Shape.Rect(w, h) =>`) |
| `switch` | ✅ | Enum variants (exhaustive, no `_` needed), integer/char literals, `_` default arm; pattern-binding arms for tagged-union payloads |
| Pointers | ✅ | `^T`, `&x`, `x^` deref, pointers to structs/enums/unions, `self^.x` → `self->x` |
| Control flow | ✅ | `if` / `else if` / `else`, `while`, `for 0..10 |i|`, `break`, `continue` |
| Literals & operators | ✅ | Integer (all bases, `_` separators), float, string, char, bool; arithmetic/comparison/logical/bitwise; content-based `str ==`/`!=` |
| C interop | ✅ | `#importc "header.h" as alias;` (clang/gcc parsing) and `extern "C" fn` declarations |
| Modules & `#import` | ✅ | `module name;` / `module name { ... }` blocks, `pub` visibility, namespace/wildcard/selected/single-symbol/file imports with aliases, submodule paths, directories-as-modules (`mod.tnc`), import caching + cycle detection, single merged C output with mangled names |
| Generic functions / structs / aliases | ✅ | `fn foo:T(...)`, `struct Pair:T`, `alias Opt:T = ?T;` — monomorphized per concrete type-argument set, including across module boundaries (`math.Pair:i32`) |
| `union` + methods | ✅ | C-style shared-memory fields (`as_int`/`as_float` reinterpret the same bytes); instance methods (`self ^Name`), static methods |
| Arrays / slices | ✅ | `[N]T`, `[_]T` (inferred size), `[]T` slices, `[N:x]T` sentinel arrays, array literals, indexing, `.len`, `for coll |x|`, implicit array→slice conversion at calls/assignments; array params/returns/whole-array assignment rejected with a slice hint |
| Optionals | ✅ | `?T`, `null`, `orelse` defaulting, `x?` unwrap, `== null` / `!= null` checks; plain values (and `null`) auto-wrap where a `?T` is expected |
| Error unions | ❌ | `!T`, `catch` — planned |
| Standard library (`std.io`, ...) | ❌ | User modules are in; the std library ships when the language matures |

See [`CHANGELOG.md`](CHANGELOG.md) for what landed in each version and [`CHECKLIST.md`](CHECKLIST.md) for the release process.

---

## Example Program

```c
#import std.io;

// Main function
fn main() void {
	var name str = "Prathmesh";
	const lang = "Tinoc";

	io.println("{s} is creator of {s} Programming Language!", name, lang);
}
```

**Transpiled C Code:**

```c
#include <stdio.h>
#include <tinoc.h>

int main() {
	str name = {"Prathmesh", 9};
	const str lang = {"Tinoc", 5};

	printf("%s is creator of %s Programming Language!\n", name.data, lang.data);
}
```

---

## Changelog

All notable changes are documented in [`CHANGELOG.md`](CHANGELOG.md), which
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Releases are
tagged `vX.Y.Z` on `main`; pushing a tag triggers the [release
workflow](.github/workflows/release.yml), which cross-compiles, packages, and
attaches binaries for linux/darwin/windows × amd64/arm64 plus a `SHA256SUMS`
manifest.

**What's new in v0.1.1** (2026-08-17):

- **Structs, end-to-end** — generic structs (`Box:T`, `Pair:(K, V)`) with
  monomorphized C11 `typedef` emission, instance (`self ^T` / `self T`) and
  static methods, named-field and zero-init literals, and const-mutability
  enforced all the way down (`o.inner.v = 1` on a `const` struct is rejected).
- **Typed array/slice literals** — `[]i32 { ... }`, `[3]i32 { ... }` in
  struct fields and return statements.
- **Nix flake + CI** — hermetic dev shell (`nix develop`), `nix flake check`,
  and a Linux/macOS CI job running the same flake checks.
- **Cross-module generic structs with methods** now instantiate correctly
  (`containers.Box:i32` with a working `fn get(self ^Box:T)`).
- **Samples 24–28** exercise every new feature end-to-end.

## Release Checklist

The exact release process lives in [`CHECKLIST.md`](CHECKLIST.md): code
health → release prep (version bump, changelog) → tag & CI release →
post-release verification. In short, `git tag v0.1.1 && git push origin
v0.1.1` triggers the release workflow, which builds the archives, generates
release notes, and creates the GitHub Release.

---

## Important Links

- **Website:** https://tinoc-lang.vercel.app
- **Creator GitHub:** https://github.com/pbarot2009

---

## Nix (reproducible dev & CI)

The repository ships a [Nix flake](flake.nix) for hermetic, zero-drift
development on Linux and macOS (`x86_64-linux`, `aarch64-linux`,
`x86_64-darwin`, `aarch64-darwin`):

```bash
# Enter the dev shell: the pinned Go toolchain (matching go.mod),
# golangci-lint, gopls, a C11 compiler (gcc + clang on Linux, clang on
# macOS), gdb + valgrind (lldb on macOS), and a flake-built `tinoc`.
nix develop

# Or with nix-direnv (see .envrc):
#   direnv allow

# Build the compiler (installs the C11 runtime header into $out/include
# and $out/share/tinoc):
nix build .#tinoc

# Full gate — gofmt, go vet, go test -race, and the end-to-end samples
# suite (generated C compiled and executed):
nix flake check
```

CI runs the same flake checks on Linux and macOS with Determinate Nix and
Magic Nix Cache (see [`.github/workflows/ci.yml`](.github/workflows/ci.yml)).

---

## Contributing

Contributions are welcome. Feel free to open an issue or submit a pull request to help improve Tinoc.

---

## License

This project is licensed under the terms of the Apache 2.0 License. See the [LICENSE](LICENSE) file for details.
