# Variable

**Variable** can be mutated.

***Syntax:***

```
var <identifier> <type>; // Decl only with explicit type.
var <identifier> <type> = <expression>; // Decl + Init with Explicit type
var <identifier> = <expression>; // Decl + Init with Infered way
```

***Example:***

```
fn main() void {
	var name str = "Lucifer";
	name = "Julius"
	var number = 101; // infer as i32
	number += 10;
	var isAdmin = false; // infer as bool
	isAdmin = true; 
}
```

---

# Constant

**Constant** can not be mutated.

***Syntax:***

```
const <identifier> <type>; // Decl only with explicit type.
const <identifier> <type> = <expression>; // Decl + Init with Explicit type
const <identifier> = <expression>; // Decl + Init with Infered way
```

***Example:***

```
fn main() void {
	const lang str = "Tinoc";
	const codename = "C^";
	// lang = "Tinoc Is Not C" 
	// Won't work, Compile Error.
}
```

---

# Integers 

***Example of Integer Literal:***

```
const decimal_int = 98222;
const hex_int = 0xff; 
const another_hex_int = 0xFF;
const octal_int = 0o755;
const binary_int = 0b11110000; 

// underscores may be placed between two digits as a visual separator 
const one_billion = 1_000_000_000; 
const binary_mask = 0b1_1111_1111; 
const permissions = 0o7_5_5;
const big_address = 0xFF80_0000_0000_0000;
```

Compiler will infer the size of each integer literal, mostly default to `i32` but if literal's size if more than `i32` so it will go for `i64`..`i128`.

For **not comptime** known you must explit add type:

```
fn divide(a i32, b i32) i32 {
    return a / b;
}
```

---

# Floats 

***Float Literals:***

```
const floating_point = 123.0E+77; 
const another_float = 123.0;
const yet_another = 123.0e+77; 
const hex_floating_point = 0x103.70p-5; 
const another_hex_float = 0x103.70; 
const yet_another_hex_float = 0x103.70P-5; 

// underscores may be placed between two digits as a visual separator 
const lightspeed = 299_792_458.000_000; 
const nanosecond = 0.000_000_001; 
const more_hex = 0x1234_5678.9ABC_CDEFp-10;
```

**Tinoc** does not has any special syntax for *NaN*, *Infinity (∞)*, *Negetive Infinity (-∞)*. Use `std.math` library for these.

```
#import std.math;

const inf = math.inf();
const negative_inf = - math.inf();
const nan = math.nan();
```

---

# Operators

See this table to know about what's operator is doing.


| **Name**                   | **Syntax**                       | **Types**                                | Notes              | **Example**  |
| -------------------------- | -------------------------------- | ---------------------------------------- | ------------------ | ------------ |
| Addition                   | `a + b`,<br>`a += b`             | [Integers](#Integers), [Floats](#Floats) | Can cause overflow | `2 + 5 == 7` |
| Wrapping Addition          |                                  |                                          |                    |              |
| Saturating Addition        |                                  |                                          |                    |              |
| Subtraction                |                                  |                                          |                    |              |
| Wrapping Subtraction       |                                  |                                          |                    |              |
| Saturating Subtraction     |                                  |                                          |                    |              |
| Negation                   |                                  |                                          |                    |              |
| Wrapping Negation          |                                  |                                          |                    |              |
| Multiplication             |                                  |                                          |                    |              |
| Wrapping Multiplication    |                                  |                                          |                    |              |
| Saturating Multiplication  |                                  |                                          |                    |              |
| Division                   |                                  |                                          |                    |              |
| Remainder Division         |                                  |                                          |                    |              |
| Bit Shift Left             |                                  |                                          |                    |              |
| Saturating Bit Shift Left  |                                  |                                          |                    |              |
| Bit Shift Right            |                                  |                                          |                    |              |
| Bitwise And                |                                  |                                          |                    |              |
| Bitwise Or                 |                                  |                                          |                    |              |
| Bitwise Xor                |                                  |                                          |                    |              |
| Bitwise Not                |                                  |                                          |                    |              |
| Defaulting Optional Unwrap | `a orelse b`                     |                                          |                    |              |
| Optional Unwrap            | `a?`                             |                                          |                    |              |
| Defaulting Error Unwrap    | `a catch b`, `a catch \|err\| b` |                                          |                    |              |
| Logical And                | `a and b`                        |                                          |                    |              |
| Logical Or                 | `!a`                             |                                          |                    |              |
| Equality                   | `a == b`                         |                                          |                    |              |
| Null Check                 | `a == null`                      |                                          |                    |              |
| Inequality                 | `a != b`                         |                                          |                    |              |
| Non-Null Check             | `a != null`                      |                                          |                    |              |
| Greater Than               |                                  |                                          |                    |              |
| Greater or Equal           |                                  |                                          |                    |              |
| Less Than                  |                                  |                                          |                    |              |
| Lesser or Equal            |                                  |                                          |                    |              |
| Array Multiplication       |                                  |                                          |                    |              |
| Pointer Dereference        | `a^`                             |                                          |                    |              |
| Address Of                 | `&a`                             |                                          |                    |              |
| Error Set Merge            | `a \|\| b`                       |                                          |                    |              |

***Precedence:***

```
x() x[] x.y x^ x? a!b x{} !x -x -%x ~x &x ?x * / % ** *% *| || + - ++ +% -% +| -| << >> <<| & ^ | orelse catch == != < > <= >= and or = *= *%= *|= /= %= += +%= +|= -= -%= -|= <<= <<|= >>= &= ^= |=
```

---

# Function

**Function** is used to make reusable block of code.

***Syntax:***

```
fn <identifier>() <type> {...}
fn <identifier>(<param> <type>, ...) <type> {...}
fn <identifier>:T() <type> {...}
fn <identifier>:T(<param> <type>, ...) <type> {...}
fn <identifier>:(T, ...)() <type> {...}
fn <identifier>:(T, ...)(<param> <type>, ...) <type> {...}

// Call

<identifier>();
<identifier>(value, ...);
<identifier>:T();
// same for other 
```

***Example:***

```
#import std.io;

fn add(a i8, b i8) i8 {
	return a + b;
}

fn greet() void {
	io.println("Hi");
}

fn Identity:T(val T) T {
	return val;
}

fn main() void {
	const result = add(10, 25);
	greet();
	io.println("result = {d}",result);
	io.println("{any}", Identify:str("Tinoc"));
}
```

---

# Types

**Tinoc** has a rich type system covering primitives, compounds, pointer/optional/error types, and heap-allocated collections.

## Primitive Types

### Integer Types

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `u8` | `uint8_t` | Unsigned Integer |
| `u16` | `uint16_t` | Unsigned Integer |
| `u32` | `uint32_t` | Unsigned Integer |
| `u64` | `uint64_t` | Unsigned Integer |
| `u128` | `__uint128_t` *(GCC/Clang only)* | Unsigned Integer |
| `usize` | `size_t` | Unsigned Integer (platform-width) |
| `i8` | `int8_t` | Signed Integer |
| `i16` | `int16_t` | Signed Integer |
| `i32` | `int32_t` | Signed Integer |
| `i64` | `int64_t` | Signed Integer |
| `i128` | `__int128_t` *(GCC/Clang only)* | Signed Integer |
| `isize` | `ptrdiff_t` | Signed Integer (platform-width) |

### Floating-Point Types

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `f32` | `float` | Float |
| `f64` | `double` | Float |
| `f128` | `__float128` *(GCC/Clang only)* | Float |

### Core Primitives

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `bool` | `_Bool` / `stdbool.h bool` | Boolean |
| `char` | `uint32_t` *(Unicode codepoint)* | Character |
| `void` | `void` | Void |

## String Type

`str` has no direct C primitive equivalent. It is represented as a C struct:

```c
typedef struct {
    const char* data;
    size_t len;
} str;
```

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `str` | `struct { const char* data; size_t len; }` | String (struct) |

## Array & Slice Types

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `[N]T` | `T arr[N]` | Fixed-size Array |
| `[_]T` | `T arr[N]` *(size inferred by compiler)* | Inferred-size Array |
| `[]T` | `struct { T* ptr; size_t len; }` | Slice (fat pointer struct) |

`[]T` has no direct C equivalent. Represented as:

```c
typedef struct {
    T* ptr;
    size_t len;
} slice_T;
```

## Pointer Type

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `^T` | `T*` | Pointer |

## Optional Type

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `?T` | `struct { T value; bool has_value; }` | Optional (nullable wrapper) |

No direct C equivalent. Represented as:

```c
typedef struct {
    T value;
    bool has_value;
} tnc_opt_T;
```

An optional wraps a payload of type `T` with a `has_value` flag: a `?T`
is either empty (`null`) or holds a `T`. The compiler emits one named
typedef per optional type used (`?i32` -> `tnc_opt_i32`), so every
declaration of the same optional type shares one C struct type.

***Syntax:***

```
var maybe ?i32 = 42;   // wrap a payload
var none  ?i32 = null; // the empty optional

maybe orelse 0   // payload when some, else the fallback (result type is T)
maybe?           // unwrap: the payload (only after a != null check)
maybe == null    // presence checks
maybe != null
```

***Example:***

```
fn index_of(hay []i32, needle i32) ?i32 {
	var i i32 = 0;
	while i < hay.len {
		if hay[i] == needle {
			return i; // plain i32 auto-wraps into ?i32
		}
		i += 1;
	}
	return null;
}

fn main() void {
	var maybe ?i32 = 42;
	var none  ?i32 = null;

	if none == null {
		var v = maybe orelse 0; // 42 — v is an i32
		var u = maybe?;         // 42
	}
	var nums [4]i32 = [10, 20, 30, 40];
	var s []i32 = nums;
	var d = index_of(s, 99) orelse -1; // -1
}
```

***Notes:***

- A plain `T` value — or `null` — auto-wraps into a `?T` wherever one is
  expected: `var`/`const` initializers, call arguments, return values,
  and assignments. Untyped numeric literals adapt to the payload type
  (`var x ?f64 = 5;`).
- `orelse` short-circuits: the fallback is only evaluated when the
  optional is `null`. Its result type is the payload type `T`, so
  `var x = maybe orelse 0;` infers `x` as `i32`.
- `maybe?` reads the payload; unwrapping an empty optional reads the
  wrapped `value` field (garbage, not a crash) — always check
  `maybe != null` first.
- Comparing an optional with anything but `null` is rejected — use
  `== null` / `!= null`. `orelse` requires an optional on the left and
  `?` unwrap requires an optional. Optionals of arrays or `void` are
  rejected (use `?[]T` for a nullable collection view).

## Error Union Types

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `!T` | `struct { T value; int err; bool is_err; }` | Inferred Error Union |
| `E!T` | `struct { T value; E err; bool is_err; }` | Explicit Error Union |

No direct C equivalent for either. General pattern:

```c
typedef struct {
    T value;
    E err;      // error code/enum
    bool is_err;
} result_T;
```

## Compound Types

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `struct` | `struct` | Compound |
| `enum` | `enum` + `int` | Compound |
| `union` | `union` | Compound |

## Heap-Allocated Types (`std.collections`)

Generic syntax uses `:T` and `:(K, V)`.

| Tinoc Type | C Equivalent | Category |
|---------|-------------|----------|
| `hstr` | None *(heap-managed `str`)* | Heap String |
| `vec:T` | None *(dynamic array)* | Heap Collection |
| `map:(K, V)` | None *(hash map)* | Heap Collection |
| `set:T` | None *(hash set)* | Heap Collection |

These are library types with no single C equivalent. They require manual implementation (dynamic arrays, hash tables, etc.) in plain C.

---

# Struct

**Struct** is a compound type and tells compiler how to handle data.

```
struct <identifier> {
	<field> <type>;
	...
	
	fn <identifier>(self ^<type-of-struct>, ...) <type> {...}
	static fn <identifier>() <type> {...}
	static fn <identifier>(<param> <type>, ...) <type> {...}
}
```

***Example:***

```
#import std.io;

// simple struct with method
struct Point {
    x f32;
    y f32;

    fn translate(self ^Point, dx f32, dy f32) void {
        self^.x += dx;
        self^.y += dy;
    }

    fn length(self ^Point) f32 {
        return sqrt(self^.x * self^.x + self^.y * self^.y);
    }
}

// generic struct with method
struct Pair:T {
    first T;
    second T;

    fn swap(self ^Pair:T) void {
        var tmp T = self^.first;
        self^.first = self^.second;
        self^.second = tmp;
    }
}

// multi-param generic struct
struct Map:(K, V) {
    key K;
    value V;
}

fn main() void {
    var p Point = Point { .x = 1.0, .y = 2.0 };
    p.translate(0.5, 1.5);

    var pair Pair:i32 = Pair:i32 { .first = 10, .second = 20 };
    pair.swap();
    io.println("{any}", p);
}
```

***Struct literals:***

Struct values are constructed with **named-field literals**. Fields may
appear in any order and a trailing comma is allowed; every field must be
initialized exactly once:

```
var p Point = Point { .x = 1.0, .y = 2.0 };
var q Point = Point { .y = 0.5, .x = -1.0, };   // any order, trailing comma
var pair Pair:i32 = Pair:i32 { .first = 10, .second = 20 };  // generic instance
```

- An **empty literal** — `Point {}` — is an explicit zero-initialization:
  every field takes its zero value, so no field list is required.
- Literal fields are type-checked against the declared field types.
  **Unknown fields**, **missing fields**, and **wrong-typed fields** are
  reported with exact source spans; an unknown name gets a
  `did you mean ...?` hint when a close match exists.
- Array-typed fields initialize with plain C brace syntax
  (`Vec { .data = [3]f64 { 1.0, 2.0, 3.0 } }`), and slice fields accept
  a typed literal (`Bag { .items = []i32 {} }`).

***Methods:***

- **Instance methods** take a receiver as their first parameter:
  `self ^Point` (pointer receiver — `self^.x` lowers to `self->x`) or
  `self Point` (by-value receiver, `self.x` works directly). Instance
  methods are called on a value or pointer: `p.translate(0.5, 1.5)`.
- **Static methods** (`static fn name(...)`) have no receiver and are
  called on the type itself, including monomorphized generic instances:
  `Rect.square(3.0)` or `Num:i32.of(21)`. Calling an instance method on
  the type name (or a static method on a value) is rejected with a hint.

***Mutability:***

A `const` binding is immutable all the way down: assigning to one of
its fields — directly, through nested structs, or through array
members — is rejected (`cannot assign to r.w (declared const)`) just
like reassigning the const itself. Writes that go *through a handle*
stay legal: mutating methods write `self^.x` (dereference) and slice
parameters write `s[i]` (through the slice's pointee, i.e. the backing
array). Mutating a by-value `self` copy (`self.x = ...`) is rejected as
a no-op.

***Generic structs:***

- `struct Pair:T { ... }` declares a template; `Pair:i32` instantiates
  it, monomorphized per concrete type-argument set with a mangled C
  name (`Pair:i32` → `tnc_Pair_i32`), cached and shared across module
  boundaries (`math.Pair:f64`). Multi-parameter forms:
  `struct Map:(K, V) { ... }` or the chained `struct Pair:T:U { ... }`.
- Static methods on an instance use the bare form `Num:i32.of(21)`;
  when a type argument is a dotted module path, parenthesize it
  (`Pair:(math.Vec2).make(...)`) so the parser doesn't absorb the
  method name into the type argument.
- Mutually-recursive generic structs (`struct A:T { b B:(A:T) }` …
  `struct B:U { a A:(B:U) }`) hit a monomorphization depth limit and
  fail with a clear `generic instantiation depth limit exceeded`
  diagnostic instead of hanging the compiler.

***Layout diagnostics:***

Circular **by-value** layouts — `struct A { b B; }` with
`struct B { a A; }`, directly or through optionals/arrays — would have
infinite size; Sema detects them at the end of analysis and reports
`circular struct layout: A -> B -> A (make one of the fields a pointer,
e.g. ^A)`. A struct containing itself directly as a field is rejected
at declaration with a pointer hint too.

---

# Enum

***Example:***

```
#import std.{io, collection.*};

enum Direction {
	North, East, South, West,
}

// enum with data and method
enum Shape {
    Circle(f32),       // radius
    Rect(f32, f32),    // width, height
    Point,

    fn area(self ^Shape) f32 {
        // match on self variant
    }
}

enum Literal {
	String(hstr),
	Integer(usize),
	// ...
	// also can make methods and static methods same as struct
}

enum Something:T {
	First,
	Second(T),
}

fn main() void {
	const direction1 = Direction.North;
	io.println("{any}", direction1);
	const literal1 = Literal.String(hstr.from("String Literal"));
	io.println("{any}", literal1); // prints: String("String Literal")
	
	var s Shape = Shape.Circle(5.0);
    var a f32 = s.area();
}
```

---

# Union

***Syntax:***

```
Syntax:
    union <Name> {
        <field_name> <type>;
        ...
        fn <method_name>(self ^<Name>, <param_name> <type>, ...) <return_type> { ... }
    }
    --> simple union with methods.

    union <Name>:<T> {
        <field_name> <type>;
        ...
        fn <method_name>(self ^<Name>:<T>, <param_name> <type>, ...) <return_type> { ... }
    }
    --> generic union with methods.
    union <Name>:(<T>, <U>, ...) {
        <field_name> <type>;
        ...
    }
    --> multi-param generic union.
```

***Example:***

```
// simple union with method
union Data {
    as_int i32;
    as_float f32;
    as_bytes [4]u8;

    fn zero(self ^Data) void {
        self^.as_int = 0;
    }
}

// generic union with method
union Either:T {
    value T;
    raw u64;

    fn clear(self ^Either:T) void {
        self^.raw = 0;
    }
}

// multi-param generic union
union OneOf:(A, B) {
    a A;
    b B;
}

fn main() void {
    var d Data;
    d.as_int = 42;
    d.zero();
    // d.as_float now reads the same memory as f32
}
```

---

# Switch

**Tinoc** uses `switch` for matching on integer or enum values.

***Syntax:***

```
Syntax:
    switch <expression> {
        <value> => { ... }
        <value> => { ... }
        _ => { ... }
    }
    --> switch with default.
```

***Example:***

```
enum TokenKind {
    Int,
    Ident,
    Plus,
    Eof,
}

fn main() void {
    var tok TokenKind = TokenKind.Plus;

    switch tok {
        TokenKind.Int   => { /* handle int */ }
        TokenKind.Ident => { /* handle ident */ }
        TokenKind.Plus  => { /* handle plus */ }
        _               => { /* default */ }
    }

    var x i32 = 2;

    switch x {
        1 => { /* one */ }
        2 => { /* two */ }
        _ => { /* anything else */ }
    }
}
```

***Notes:***

- No parentheses around the expression.
- `_` is the default case.
- No fallthrough; each arm is independent.
- Braces are required per arm.

---

# If / Else

**Tinoc** uses `if` and `else` for conditional branching. No parentheses around the condition.

***Syntax:***

```
    if <condition> { ... }
    --> simple if.

    if <condition> { ... } else { ... }
    --> if / else.

    if <condition> { ... } else if <condition> { ... } else { ... }
    --> if / else if chain.
```

***Example:***

```
fn main() void {
    var x i32 = 10;

    if x > 0 {
        // positive
    }

    if x > 0 {
        // positive
    } else {
        // zero or negative
    }

    if x > 0 {
        // positive
    } else if x == 0 {
        // zero
    } else {
        // negative
    }
}
```

***Notes:***

- No parentheses around the condition.
- Braces are required; no single-line braceless form.

---

# For

**Tinoc** uses `for` with a range and a capture binding for iteration.

***Syntax:***

```
Syntax:
    for <start>..<end> |<i>| { ... }
    --> iterate over a range, capturing index.

    for <collection> |<item>| { ... }
    --> iterate over a collection.
```

***Example:***

```
fn main() void {
    // range loop — 0 to 9
    for 0..10 |i| {
        // i goes 0, 1, 2, ... 9
    }

    // range with step is done via while if needed
    var i i32 = 0;
    while i < 10 {
        i += 2;
    }

    // iterate over a slice
    var nums [5]i32 = [1, 2, 3, 4, 5];
    for nums |n| {
        // n is each element
    }
}
```

***Notes:***

- No parentheses around the range or collection.
- `|i|` captures the loop variable, its not a closure, just binding syntax.
- Range `0..10` is exclusive on the right, iterates 0 through 9.
- `break` exits the loop, `continue` skips to the next iteration.

---

# While

**Tinoc** uses `while` for condition-based loops. `while true` covers the infinite loop / do-while pattern.

***Syntax:***

```
Syntax:
    while <condition> { ... }
    --> loops while condition is true.

    while true { ... }
    --> infinite loop.
```

***Example:***

```
fn main() void {
    var i i32 = 0;

    // normal while
    while i < 10 {
        i += 1;
    }

    // infinite loop — break to exit
    while true {
        if i == 20 {
            break;
        }
        i += 1;
    }

    // do-while equivalent — run body first, check at end
    while true {
        i += 1;
        if i >= 5 {
            break;
        }
    }
}
```

***Notes:***

- No parentheses around the condition.
- Braces are required.
- `while true` replaces both infinite loops and do-while patterns from C.
- `break` exits the loop, `continue` skips to the next iteration.

---

# Modules

**Tinoc** uses `#import` for module resolution. The `#` prefix marks module and preprocessor directives. `#import` replaces C `#include` with *semantic* module resolution: modules are parsed and type-checked once, only `pub` exports are visible to importers, and every loaded module compiles into a single merged C translation unit — no text insertion, no header guards.

## The `module` keyword

`module <name>;` names the file's module. A file without a declaration gets its module name from its file stem (`vec.tnc` → module `vec`), and convention files `mod.tnc` / `index.tnc` take their containing directory's name (`shapes/mod.tnc` → module `shapes`) — **directories become modules automatically**.

```tinoc
module math;   // this file is the module `math`; importers say: #import math;

pub const PI f64 = 3.14159;
const TAU f64 = 6.28318;   // private: only visible inside this module

pub fn square(x f64) f64 {
	return x * x;
}
```

`module <name> { ... }` groups declarations into an **in-file namespace** — the members are reachable as `name.member` from the rest of the file. Blocks nest (`module a { module b { ... } }` → `a.b.member`) and accept dotted names (`module a.b { ... }`).

```tinoc
module physics {
	pub const g f64 = 9.81;

	pub fn energy(m f64, v f64) f64 {
		return 0.5 * m * v * v;
	}
}

fn main() void {
	// physics.energy / physics.g usable here
}
```

## Import forms

```tinoc
#import module;                  // namespace import -> module.symbol
#import module.sub;              // nested path / submodule
#import module.*;                // wildcard: every pub symbol, usable bare
#import module.symbol;           // single symbol, usable directly
#import module.{a, b as c};      // selected symbols, per-symbol aliases
#import module as alias;         // rename the namespace
#import "rel/path.tnc" as alias; // file import (module name from file)
```

Whether a bare dotted tail (`math.PI`) is a submodule or a single symbol is decided by the module loader: it tries a module file first (`math/PI.tnc`, `math/PI/mod.tnc`), then a pub symbol of the prefix module (`math`). The `{...}` and `.*` forms are unambiguous. `module.symbol as name` renames a single-symbol import; `#import module as m;` renames the namespace.

***Example:***

`shapes/vec.tnc` (the submodule `shapes.vec`):
```tinoc
pub struct Vec2 {
	x f32;
	y f32;
}

pub fn dot(a Vec2, b Vec2) f32 {
	return a.x * b.x + a.y * b.y;
}
```

`main.tnc`:
```tinoc
#import math;
#import math.PI;                 // single symbol -> PI
#import math.E as EULER;         // single symbol, renamed
#import shapes.vec;              // namespace -> vec.Vec2 / vec.dot
#import shapes.vec.{Vec2, dot};  // selected symbols, usable directly
#import "shapes/vec.tnc" as vf; // file import under an alias
#import shapes.*;                // wildcard -> circumference(...)
```

## Visibility

`pub` marks a function, struct, enum, union, const, var, or alias as exported from its module; everything else is private and rejected when imported — `symbol x is private to module math` for a private member, `module math has no public symbol x` for a missing one. In-file `module name { ... }` blocks expose every member (they are the same file's namespace).

## Modules & generics

Generic declarations (`fn name:T`, `struct Name:T`, `alias Name:T = ...;`, and the multi-param `:(K, V)` forms) cross module boundaries like any other pub symbol:

```tinoc
#import math;                  // qualified use: math.identity:i32(42)
#import math.identity;         // bare use: identity:i32(42) or inferred identity(7)
#import math.identity as id;   // renamed: id:str("hi")
#import math.{Pair};           // bare struct template: Pair:f64 { .first = 1.0, ... }
#import box.Opt;               // bare alias template: Opt:i32
#import math.*;                // wildcard binds every pub generic bare
```

A bare instantiation and a qualified call of the same generic produce **one** shared mangled C instance (`identity:i32(42)` and `math.identity:i32(42)` emit a single `tnc_math_identity_i32`), so an imported generic is never duplicated. Private generics are rejected like private functions — `symbol x is private to module math` — whether imported by name or reached qualified. Each concrete instantiation is monomorphized once per type-argument set.

Generic bodies are checked in the **defining module's scope**: an instantiated body resolves that module's private helpers and consts exactly as the template author wrote them, and may itself build the module's own generic types (`makePair:(K, V)` returning `Pair:(K, V) { ... }`) — generics compose across boundaries.

## Module resolution rules

- Imports resolve **relative to the importing file's directory**: `#import a.b.c;` looks for `a/b/c.tnc`, then `a/b/c/mod.tnc`, then `a/b/c/c.tnc`.
- Modules load **recursively** (imports of imports) with **cycle detection** (`import cycle detected: a -> b -> a`), and are **cached by absolute path** — diamond imports (`main` → `a` → `base`, `main` → `b` → `base`) share one instance.
- Re-importing the same module under the same local name is idempotent (`#import vec; #import vec.{Vec2};` binds `vec` once); binding a *different* module under a taken name is an error.
- Two files claiming the same module name collide (`module math is already defined by ...`), since their items would mangle to the same C symbols.
- The **standard library is not available yet**: `#import std.io;` is rejected with `standard library modules are not yet available (std.io)` — user modules only for now.
- Every module's code is merged into one C translation unit; module items get mangled C names (`math.abs` → `tnc_math_abs`) so same-named items across modules never collide.

---

# Preprocessor

- `#import`: ***Comptime*** module resolution. Pass the entry file (i.e. `main.tnc`) and the compiler loads, checks, and merges every transitively imported module (see [Modules](#modules)).
- `#importc`: ***C header import***. `#importc "stdio.h" "math.h" as c;` parses real C headers (clang's JSON AST, gcc's `-aux-info` fallback) and exposes their functions, extern variables, enum constants, typedefs, and simple macros under the alias with full type checking — `c.printf(...)`, `c.EOF`, `c.sqrt(16.0)`. Without `as alias` the alias defaults to the header's file stem (`#importc "stdio.h";` → `stdio.printf(...)`). Codegen emits a matching `#include` per header. See [C Interop](#c-interop).
- `extern "C" fn name(.symbol)?(params...) Ret;`: declares a C function by hand (no header parsing) — `extern "C" fn printf(fmt *const char, ...) i32;` — callable by its Tinoc name with automatic `str` → `char*` argument unwrapping.
- `#run`: ***Comptime*** execution as expression or block, useful for Meta Programming.
- `#partial`: Tells compiler about a partial implementation of switch on enum.

Other useful Preprocessor directives may be added in future versions.

---

# C Interop

**Tinoc** talks to C two ways: `#importc` parses real headers, `extern "C" fn` declares functions by hand. Both give you type-checked calls into libc and any other C library.

## `#importc` — import C headers

```tinoc
#importc "stdio.h";                 // alias defaults to the file stem: stdio
#importc "stdio.h" "math.h" as c;  // multiple headers, explicit alias
#importc "mylib.h" as mylib;        // local header next to the source
```

The compiler parses each header and registers every function, extern variable, enum constant, typedef, and object-like macro under the alias:

```tinoc
#importc "stdio.h" as cio;

fn main() void {
	cio.printf("%.1f\n", 3.14);   // full argument type/count checking
	var code i32 = cio.EOF;        // -1 (macro constant)
}
```

Codegen emits one `#include` per header into the merged output, so the declarations resolve at C compile time. Type safety is enforced at the Tinoc level first — unknown members (`undefined: cio.doesNotExist`) and wrong argument counts/types are caught before C ever runs.

## `extern "C" fn` — hand-declared C functions

```tinoc
extern "C" fn printf(fmt *const char, ...) i32;
extern "C" fn strlen(s *const char) usize;
extern "C" fn my_puts.puts(s *const char) i32;   // call my_puts, C symbol puts
```

Declarations must end with `;` (no body); variadic declarations need at least one named parameter before `...`. `str` arguments are unwrapped to their underlying `char*` automatically, so `printf("%s\n", lang)` passes the string's data pointer.

---

# Array

**Array** is a fixed-size, contiguous sequence of elements of type `T`.
Its length is part of the type and known at compile time.

***Syntax:***

```
[<length>]<T>        // fixed-size array, e.g. [5]u8
[_]<T>               // size inferred from the initializer, e.g. [_]f32
[<length>:<sentinel>]<T>  // sentinel-terminated array, e.g. [_:0]u8

[<elem>, <elem>, ...]     // array literal; element type is inferred

[<T>] { <elem>, ... }     // typed slice literal, e.g. []i32 { 1, 2 } or []i32 {}
[<N><T>] { <elem>, ... }  // typed fixed-size literal, e.g. [3]i32 { 1, 2, 3 }
```

***Example:***

```
// array literal with inferred element type
const message = ['h', 'e', 'l', 'l', 'o'];

// alternative initialization using result location
const alt_message [5]u8 = ['h', 'e', 'l', 'l', 'o'];

// explicit element type, size inferred from the literal
var floats [_]f32 = [1.5, 2.5, 3.5];

// element access and assignment
if message[0] != 'h' { ... }
message[4] = 'x';

// .len is the element count (compile-time constant for arrays)
if message.len != 5 { ... }

// iterate over the elements
var total i32 = 0;
for message |ch| {
    total += ch;
}
```

***Notes:***

- Array literals in a `var`/`const` initializer use plain C brace
  initialization; the literal must match the declared length exactly.
- `[N]T` and `[_]T` values store exactly `N` elements; `[N:x]T` stores
  `N + 1` (the sentinel sits at index `N`), matching C string
  conventions.
- Elements are read/write accessible via `arr[i]`; indexing a
  multidimensional array chains: `mat[i][j]`.
- A type-annotated literal — `[]i32 {}`, `[]f64 { 1.5, 2.5 }`, or
  `[3]i32 { 1, 2, 3 }` — spells the element type explicitly and is
  useful wherever the result location isn't a plain declaration (struct
  literal fields, return statements). The annotation is syntax sugar:
  Sema retypes the literal from the result location as usual.

## Slices

A **slice** `[]T` is a fat pointer — a `{ ptr: ^T, len: usize }` pair
that views a contiguous region of memory, typically an array.

```
var nums [5]i32 = [1, 2, 3, 4, 5];
var s []i32 = nums;   // array converts implicitly to a slice view

if s.len != 5 { ... } // runtime length
s[2] = 30;            // writes through to the backing array

fn total(s []i32) i32 { ... }  // functions take slices, not arrays
```

***Notes:***

- Arrays convert implicitly to slices at call sites and in
  assignments; the slice aliases the array's storage.
- Functions cannot take or return array values (arrays are storage,
  not handles) — the compiler rejects them and points you to slices:
  `array parameter a is not supported ([3]i32) — use a slice ([]i32) instead`.
- Assigning a whole array is rejected for the same reason; copy
  element-wise instead.
- Iterate with the collection form: `for s |v| { ... }`.

## Multidimensional arrays

Nested `[N]T` types build row-major matrices: `[4][5]f32` is 4 rows of
5 `f32`. A row (`mat[i]`) is itself an array, so iterate row-by-row:

```
const mat4x5 [4][5]f32 = [
    [1.0, 0.0, 0.0, 0.0, 0.0],
    [0.0, 1.0, 0.0, 1.0, 0.0],
    [0.0, 0.0, 1.0, 0.0, 0.0],
    [0.0, 0.0, 0.0, 1.0, 9.9],
];

if mat4x5[2][3] != 0.0 { ... }

var total f32 = 0.0;
for 0..mat4x5.len |i| {
    for mat4x5[i] |cell| {
        total += cell;
    }
}
```

***Notes:***

- `.len` on a multidimensional array is the number of rows.
- The compiler rejects iterating a multidimensional array directly
  (its element is itself an array) — iterate a row instead.

## Sentinel-Terminated Array

The syntax `[N:x]`T describes an array which has a sentinel element of value `x` at the index corresponding to the length `N`.

```
#import std.testing.expectEqual;

test "0-terminated sentinel array" { 
	const array [_:0]u8 = [1, 2, 3, 4]; 
	try expectEqual([4:0]u8, @TypeOf(array)); 
	try expectEqual(4, array.len); 
	try expectEqual(0, array[4]);
}
```

## Destructing Array 

{Placeholder}

---