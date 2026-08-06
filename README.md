<p align="center">
  <img src="banner.png" alt="Tinoc Banner" width="100%">
</p>

<h1 align="center">Tinoc</h1>

<p align="center">
  <strong>This Is Not C</strong><br>
  A modern systems programming language that transpiles to C11.
</p>

<p align="center">
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
- [Contributing](#contributing)
- [License](#license)

---

## About

> This is the official GitHub repository for the **TinocLang** compiler source code.

## **T**inoc **I**s **No**t **C**

Tinoc is a short form of **T**his **I**s **No**t **C**, commonly known as **T**inoc **I**s **No**t **C**.

**NOTE:** Tinoc respects C and its usage. Tinoc is built on the philosophy that programming should be **Meaningful**, **Accurate**, **Robust**, **Maximum Performance**, and **Simple**.

**Tinoc** transpiles to C11 for maximum system support — C11 unlocks features like `_Generic` dispatch in the `tinoc.h` runtime (saturating arithmetic) and gives the compiler room to grow (optionals, error unions, typed enums) without fighting the target language.

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
- Offer to add `~/.tinoc/bin` to your `PATH` after installing.

Common flags: `--check` (compare installed vs latest), `--uninstall`,
`--version 0.1.0` (specific release), `--force`/`--yes` (skip prompts),
`--dir <path>` (override install dir). Run `install.sh --help` for the full list.

---

## Language Support

What the compiler supports end-to-end in the current release:

| Feature | Status | Notes |
| --- | --- | --- |
| `var` / `const` (incl. `static`) | ✅ | Explicit or inferred types; const-mutability enforced |
| `fn` functions | ✅ | Forward calls, argument/return type checking, missing-return checks |
| `struct` + methods | ✅ | Fields, struct literals (`Point { .x = 1.0 }`), instance methods (`self ^Point`), static methods, by-value copy/params/returns |
| Pointers | ✅ | `^T`, `&x`, `x^` deref, pointers to structs, `self^.x` → `self->x` |
| Control flow | ✅ | `if` / `else if` / `else`, `while`, `for 0..10 |i|`, `break`, `continue` |
| Literals & operators | ✅ | Integer (all bases, `_` separators), float, string, char, bool; arithmetic/comparison/logical/bitwise |
| C interop | ✅ | `#importc "header.h" as alias;` (clang/gcc parsing) and `extern "C" fn` declarations |
| Generic functions / structs | ❌ | `fn foo:T(...)`, `struct Pair:T` — rejected with a clear "not yet supported" diagnostic |
| `enum` / `union` | ❌ | Planned after structs |
| `switch` | ❌ | Planned |
| Arrays / slices | ❌ | `[N]T`, `[]T`, `for coll |x|` — planned |
| Optionals / error unions | ❌ | `?T`, `!T`, `orelse`, `catch` — planned |
| Standard library (`std.io`, ...) | ❌ | Module system is next after the type system |

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

## Important Links

- **Website:** https://tinoc-lang.vercel.app
- **Creator GitHub:** https://github.com/pbarot2009

---

## Contributing

Contributions are welcome. Feel free to open an issue or submit a pull request to help improve Tinoc.

---

## License

This project is licensed under the terms of the Apache 2.0 License. See the [LICENSE](LICENSE) file for details.
