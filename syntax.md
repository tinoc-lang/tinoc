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
} optional_T;
```

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

**Tinoc** uses `#import` for module resolution. The `#` prefix marks module and preprocessor directives.

***Syntax:***

```
Syntax:
    #import <module.path>;
    --> import a specific module.

    #import <module.path>.*;
    --> import all exports from a module.
```

***Example:***

`util.tnc`:
```tinoc
pub fn add(a i32, b i32) i32 {
	return a + b;
}
```

`main.tnc`:
```
#import std.io;
#import util;
#import std.collections.*;

fn main() void {
    // std.io symbols accessed via module name
    io.println("Hello, Tinoc");

    // std.collections.* symbols available directly
    var v vec:i32;
    
    const result = util.add(125, 2022);
    io.println("{any}",result);
}
```

***Notes:***

- `#import` replaces C `#include`; it uses semantic module resolution, not file text insertion.
- `.*` wildcard imports all public exports from a module into the current scope.
- `pub` keyword is used to declare a function/struct/enum/union/const etc. public for import.
- Without `.*`, symbols are accessed via the last path segment (e.g. `std.io` → `io.println`).

---

# Preprocessor

- `#import`: ***Comptime*** module resolution, you have to pass main file(i.e. `main.tnc`) and compiler will handle the rest.
- `#run`: ***Comptime*** execution as expression or block, useful for Meta Programming.
- `#partial`: Tells compiler about a partial implementation of switch on enum.

Other useful Preprocessor may be add in future versions.

---

# Array 

{placeholder}

***Example:***

```
// array literal
const message = ['h', 'e', 'l', 'l', 'o'];

// alternative initialization using result location
const alt_message [5]u8 = [ 'h', 'e', 'l', 'l', 'o'];
```

## Multidimensional arrays

{Placeholder}

```
const mat4x5 [4][5]f32 = [
    [1.0, 0.0, 0.0, 0.0, 0.0],
    [0.0, 1.0, 0.0, 1.0, 0.0],
    [0.0, 0.0, 1.0, 0.0, 0.0],
    [0.0, 0.0, 0.0, 1.0, 9.9],
];
```

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