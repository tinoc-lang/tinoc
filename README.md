<p align="center">
  <img src="banner.png" alt="Tinoc Banner" width="100%">
</p>

<h1 align="center">Tinoc</h1>

<p align="center">
  <strong>This Is Not C</strong><br>
  A modern systems programming language that transpiles to C99.
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
- [Example Program](#example-program)
- [Important Links](#important-links)
- [Contributing](#contributing)
- [License](#license)

---

## About

> This is the official GitHub repository for the **TinocLang** compiler source code.

## **T**inoc **I**s **No**t **C**

Tinoc is a short form of **T**his **I**s **No**t **C**, commonly known as **T**inoc **I**s **No**t **C**.

**NOTE:** Tinoc respects C and its usage. Tinoc is built on the philosophy that programming should be **Meaningful**, **Accurate**, **Robust**, **Maximum Performance**, and **Simple**.

**Tinoc** transpiles to C99 for maximum system support.

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
- **License:** [LICENSE](LICENSE)
- **Creator GitHub:** https://github.com/pbarot2009

---

## Contributing

Contributions are welcome. Feel free to open an issue or submit a pull request to help improve Tinoc.

---

## License

This project is licensed under the terms of the MIT License. See the [LICENSE](LICENSE) file for details.