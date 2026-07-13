# Tinoc

> This is the official GitHub repository for the **TinocLang** compiler source code.

## **T**inoc **I**s **No**t **C**

Tinoc is a short form of **T**his **I**s **No**t **C**, commonly known as **T**inoc **I**s **No**t **C**.

**NOTE:** Tinoc respects C and its usage. Tinoc is built on the philosophy that programming should be **Meaningful**, **Accurate**, **Robust**, **Maximum Performance**, and **Simple**.

**Tinoc** transpiles to C99 for maximum system support.

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
	
	printf("%s is creator of %s Programming Language!\n", name, lang);
}
```

### Important Links

- [Website](https://tinoc-lang.vercel.app)
- [LICENSE](LICENSE)
- [Creator GitHub](https://github.com/pbarot2009)