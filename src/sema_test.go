package src

import "testing"

// === Sema Tests ===
//
// These exercise semantic analysis for var/const/static var/static
// const/fn: type inference, explicit-type checking, const-mutability
// enforcement, function signature/return checking, and name resolution.

func checkNoErrors(t *testing.T, source string) *Sema {
	t.Helper()
	_, sema, diags := RunSema("test.tnc", source)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("unexpected diagnostic: %s", d.String())
		}
	}
	return sema
}

func checkHasError(t *testing.T, source, wantSubstr string) {
	t.Helper()
	_, _, diags := RunSema("test.tnc", source)
	if !diags.HasErrors() {
		t.Fatalf("expected an error containing %q, got none", wantSubstr)
	}
	for _, d := range diags.All() {
		if d.Severity == SeverityError && contains(d.Message, wantSubstr) {
			return
		}
	}
	var got []string
	for _, d := range diags.All() {
		got = append(got, d.Message)
	}
	t.Fatalf("expected an error containing %q, got: %v", wantSubstr, got)
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestVarInferredType(t *testing.T) {
	sema := checkNoErrors(t, `
fn main() void {
	var x = 5;
	return;
}
`)
	_ = sema
}

func TestVarExplicitTypeMatches(t *testing.T) {
	checkNoErrors(t, `
fn main() void {
	var x i64 = 5;
	return;
}
`)
}

func TestVarExplicitTypeMismatch(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x i32 = "hello";
	return;
}
`, "cannot use")
}

func TestVarDeclOnlyNoTypeNoValue(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x;
	return;
}
`, "cannot infer type")
}

func TestConstRequiresInitializer(t *testing.T) {
	checkHasError(t, `
fn main() void {
	const x i32;
	return;
}
`, "missing initializer")
}

func TestConstCannotBeReassigned(t *testing.T) {
	checkHasError(t, `
fn main() void {
	const x = 5;
	x = 10;
	return;
}
`, "declared const")
}

func TestVarCanBeReassigned(t *testing.T) {
	checkNoErrors(t, `
fn main() void {
	var x = 5;
	x = 10;
	return;
}
`)
}

func TestStaticVarAndConst(t *testing.T) {
	checkNoErrors(t, `
fn main() void {
	static var counter i32 = 0;
	static const limit i32 = 100;
	counter += 1;
	return;
}
`)
}

func TestStaticConstCannotBeReassigned(t *testing.T) {
	checkHasError(t, `
fn main() void {
	static const limit i32 = 100;
	limit = 200;
	return;
}
`, "declared const")
}

func TestRedeclarationInSameScope(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x = 5;
	var x = 10;
	return;
}
`, "redeclared")
}

func TestShadowingInNestedScopeAllowed(t *testing.T) {
	checkNoErrors(t, `
fn main() void {
	var x = 5;
	if x > 0 {
		var x = 10;
		x = 20;
	}
	return;
}
`)
}

func TestUndefinedIdentifier(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x = y;
	return;
}
`, "undefined: y")
}

func TestFunctionCallForwardReference(t *testing.T) {
	// Tinoc allows main() to call a function defined later in the file.
	checkNoErrors(t, `
fn main() void {
	var x = add(1, 2);
	return;
}

fn add(a i32, b i32) i32 {
	return a + b;
}
`)
}

func TestFunctionArgCountMismatch(t *testing.T) {
	checkHasError(t, `
fn add(a i32, b i32) i32 {
	return a + b;
}

fn main() void {
	var x = add(1);
	return;
}
`, "not enough arguments")
}

func TestFunctionArgTypeMismatch(t *testing.T) {
	checkHasError(t, `
fn add(a i32, b i32) i32 {
	return a + b;
}

fn main() void {
	var x = add(1, "two");
	return;
}
`, "argument 2")
}

func TestUndefinedFunctionCall(t *testing.T) {
	checkHasError(t, `
fn main() void {
	foo(1, 2);
	return;
}
`, "undefined: foo")
}

func TestMissingReturn(t *testing.T) {
	checkHasError(t, `
fn add(a i32, b i32) i32 {
	var x = a + b;
}
`, "missing return")
}

func TestReturnTypeMismatch(t *testing.T) {
	checkHasError(t, `
fn add(a i32, b i32) i32 {
	return "not a number";
}
`, "return value")
}

func TestReturnValueFromVoidFunction(t *testing.T) {
	checkHasError(t, `
fn main() void {
	return 5;
}
`, "too many return values")
}

func TestIfElseBothReturnSatisfiesMissingReturn(t *testing.T) {
	checkNoErrors(t, `
fn sign(x i32) i32 {
	if x > 0 {
		return 1;
	} else if x < 0 {
		return -1;
	} else {
		return 0;
	}
}
`)
}

func TestIfWithoutElseDoesNotSatisfyMissingReturn(t *testing.T) {
	checkHasError(t, `
fn sign(x i32) i32 {
	if x > 0 {
		return 1;
	}
}
`, "missing return")
}

func TestWhileConditionMustBeBool(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x i32 = 5;
	while x {
		x = x - 1;
	}
	return;
}
`, "non-boolean")
}

func TestUntypedLiteralAdaptsToDeclaredType(t *testing.T) {
	sema := checkNoErrors(t, `
fn main() void {
	var big i64 = 5;
	return;
}
`)
	_ = sema
}

func TestDuplicateParamName(t *testing.T) {
	checkHasError(t, `
fn add(a i32, a i32) i32 {
	return a;
}
`, "duplicate parameter")
}

func TestMismatchedOperandTypes(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x i32 = 5;
	var y f64 = 1.5;
	var z = x + y;
	return;
}
`, "mismatched types")
}

func TestLogicalOperatorRequiresBool(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var x i32 = 5;
	var y = x and true;
	return;
}
`, "non-boolean")
}

func TestForRangeLoop(t *testing.T) {
	checkNoErrors(t, `
fn main() void {
	var total i32 = 0;
	for 0..10 |i| {
		total += i;
	}
	return;
}
`)
}

// === Struct Sema Tests ===

func TestStructDeclarationAndLiteral(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .y = 2.0 };
	p.x = 5.0;
	return;
}
`)
}

func TestStructInferredLiteral(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p = Point { .x = 1.0, .y = 2.0 };
	return;
}
`)
}

func TestStructDeclOnly(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point;
	return;
}
`)
}

func TestStructUnknownFieldInLiteral(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .z = 2.0 };
	return;
}
`, "unknown field z in struct Point")
}

func TestStructMissingFieldInLiteral(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0 };
	return;
}
`, "missing field(s)")
}

func TestStructDuplicateFieldInLiteral(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .x = 2.0, .y = 3.0 };
	return;
}
`, "duplicate field x")
}

func TestStructLiteralTypeMismatch(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .y = "nope" };
	return;
}
`, "field y")
}

func TestStructLiteralOnNonStruct(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var p i32 = i32 { .x = 1 };
	return;
}
`, "is not a struct type")
}

func TestStructDuplicateFields(t *testing.T) {
	checkHasError(t, `
struct Bad {
	a i32;
	a i32;
}
fn main() void {
	return;
}
`, "duplicate field a")
}

func TestStructRedeclared(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
}
struct Point {
	y f32;
}
fn main() void {
	return;
}
`, "redeclared")
}

func TestStructSelfContainmentByValueRejected(t *testing.T) {
	checkHasError(t, `
struct Bad {
	next Bad;
}
fn main() void {
	return;
}
`, "contains itself by value")
}

func TestStructSelfPointerAllowed(t *testing.T) {
	checkNoErrors(t, `
struct Node {
	value i32;
	next ^Node;
}
fn main() void {
	return;
}
`)
}

func TestStructFieldAccess(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .y = 2.0 };
	var x = p.x;
	p.y = x;
	return;
}
`)
}

func TestStructUnknownFieldAccess(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .y = 2.0 };
	var z = p.z;
	return;
}
`, "has no field or method z")
}

func TestStructFieldTypeMismatchOnAssign(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var p Point = Point { .x = 1.0, .y = 2.0 };
	p.x = "hello";
	return;
}
`, "cannot use")
}

func TestStructCannotCompare(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;
	y f32;
}

fn main() void {
	var a Point = Point { .x = 1.0, .y = 2.0 };
	var b Point = Point { .x = 1.0, .y = 2.0 };
	if a == b {
		return;
	}
	return;
}
`, "cannot compare")
}

func TestStructAsParamAndReturn(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;
}

fn make(x f32) Point {
	return Point { .x = x, .y = 0.0 };
}

fn sum(a Point, b Point) f32 {
	return a.x + b.y;
}

fn main() void {
	var p = make(1.0);
	var s = sum(p, p);
	return;
}
`)
}

func TestStructInstanceMethod(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;

	fn translate(self ^Point, dx f32, dy f32) void {
		self^.x += dx;
		self^.y += dy;
	}

	fn length(self ^Point) f32 {
		return self^.x * self^.x + self^.y * self^.y;
	}
}

fn main() void {
	var p Point = Point { .x = 1.0, .y = 2.0 };
	p.translate(0.5, 1.5);
	var l = p.length();
	return;
}
`)
}

func TestStructStaticMethod(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;
	y f32;

	static fn origin() Point {
		return Point { .x = 0.0, .y = 0.0 };
	}
}

fn main() void {
	var o = Point.origin();
	return;
}
`)
}

func TestStructMethodMissingReturn(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn doubled(self ^Point) f32 {
		var r = self^.x * 2.0;
	}
}
fn main() void {
	return;
}
`, "missing return at end of method")
}

func TestStructStaticCalledOnValueRejected(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	static fn make() Point {
		return Point { .x = 1.0 };
	}
}

fn main() void {
	var p Point = Point { .x = 1.0 };
	p.make();
	return;
}
`, "must be called on the type name")
}

func TestStructInstanceCalledOnTypeRejected(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn get(self ^Point) f32 {
		return self^.x;
	}
}

fn main() void {
	var v = Point.get();
	return;
}
`, "is not static")
}

func TestStructUnknownMethod(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn get(self ^Point) f32 {
		return self^.x;
	}
}

fn main() void {
	var p Point = Point { .x = 1.0 };
	p.warp(2.0);
	return;
}
`, "no method warp")
}

func TestStructMethodArgCountMismatch(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn scale(self ^Point, k f32) void {
		self^.x *= k;
	}
}

fn main() void {
	var p Point = Point { .x = 1.0 };
	p.scale();
	return;
}
`, "not enough arguments")
}

func TestStructMethodArgTypeMismatch(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn scale(self ^Point, k f32) void {
		self^.x *= k;
	}
}

fn main() void {
	var p Point = Point { .x = 1.0 };
	p.scale("big");
	return;
}
`, "argument 1")
}

func TestStructInstanceMethodWithoutSelfRejected(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn bad() f32 {
		return 1.0;
	}
}
fn main() void {
	return;
}
`, "needs a self parameter")
}

func TestStructSelfWrongTypeRejected(t *testing.T) {
	checkHasError(t, `
struct Point {
	x f32;

	fn bad(self ^Other) f32 {
		return 1.0;
	}
}
fn main() void {
	return;
}
`, "self parameter")
}

func TestStructNested(t *testing.T) {
	checkNoErrors(t, `
struct Inner {
	v f32;
}

struct Outer {
	inner Inner;
	tag i32;
}

fn main() void {
	var o Outer = Outer { .inner = Inner { .v = 1.5 }, .tag = 7 };
	var v = o.inner.v;
	o.inner.v = v;
	return;
}
`)
}

func TestStructMethodCallOnPointerReceiver(t *testing.T) {
	checkNoErrors(t, `
struct Point {
	x f32;

	fn set(self ^Point, v f32) void {
		self^.x = v;
	}
}

fn main() void {
	var p Point = Point { .x = 1.0 };
	var pp ^Point = &p;
	pp.set(9.0);
	return;
}
`)
}

// === Enums ===

func TestEnumFieldlessVariants(t *testing.T) {
	checkNoErrors(t, `
enum Direction {
	North, East, South, West,
}

fn main() void {
	var d Direction = Direction.East;
	if d == Direction.East {
		return;
	}
}
`)
}

func TestEnumPayloadConstruction(t *testing.T) {
	checkNoErrors(t, `
enum Shape {
	Circle(f32),
	Rect(f32, f32),
	Point,
}

fn main() void {
	var c Shape = Shape.Circle(5.0);
	var r Shape = Shape.Rect(1.0, 2.0);
	var p Shape = Shape.Point;
	return;
}
`)
}

func TestEnumConstructorWrongArity(t *testing.T) {
	checkHasError(t, `
enum Shape { Rect(f32, f32) }
fn main() void {
	var s Shape = Shape.Rect(1.0);
}
`, "not enough arguments")
}

func TestEnumConstructorWrongArgType(t *testing.T) {
	checkHasError(t, `
enum Shape { Circle(f32) }
fn main() void {
	var s Shape = Shape.Circle("nope");
}
`, "argument 1 to Shape.Circle")
}

func TestEnumUnknownVariant(t *testing.T) {
	checkHasError(t, `
enum Color { Red, Green }
fn main() void {
	var c Color = Color.Purple;
}
`, "no variant or method")
}

func TestEnumDuplicateVariant(t *testing.T) {
	checkHasError(t, `
enum Color { Red, Red }
fn main() void { }
`, "duplicate variant")
}

func TestEnumExhaustiveSwitchNoMissingReturn(t *testing.T) {
	checkNoErrors(t, `
enum Color { Red, Green, Blue }
fn name(c Color) str {
	switch c {
		Color.Red => { return "red"; }
		Color.Green => { return "green"; }
		Color.Blue => { return "blue"; }
	}
}
fn main() void { }
`)
}

func TestEnumSwitchNonExhaustiveMissingReturn(t *testing.T) {
	checkHasError(t, `
enum Color { Red, Green, Blue }
fn name(c Color) str {
	switch c {
		Color.Red => { return "red"; }
		Color.Green => { return "green"; }
	}
}
fn main() void { }
`, "missing return")
}

func TestEnumSwitchWrongEnumRejected(t *testing.T) {
	checkHasError(t, `
enum A { X }
enum B { Y }
fn main() void {
	var a A = A.X;
	switch a {
		B.Y => { }
	}
}
`, "mismatched types in switch")
}

func TestEnumSwitchPatternBinding(t *testing.T) {
	checkNoErrors(t, `
enum Shape {
	Circle(f32),
	Rect(f32, f32),
	Point,
}

fn area(s Shape) f32 {
	switch s {
		Shape.Rect(w, h) => { return w * h; }
		Shape.Circle(r) => { return 3.0 * r; }
		Shape.Point => { return 0.0; }
	}
}
fn main() void { }
`)
}

func TestEnumSwitchPatternBindingCount(t *testing.T) {
	checkHasError(t, `
enum Shape { Rect(f32, f32) }
fn main() void {
	var s Shape = Shape.Rect(1.0, 2.0);
	switch s {
		Shape.Rect(w) => { }
	}
}
`, "pattern Shape.Rect expects 2 binding(s)")
}

func TestEnumInstanceMethod(t *testing.T) {
	checkNoErrors(t, `
enum Shape {
	Circle(f32),
	Point,

	fn isRound(self ^Shape) bool {
		switch self^ {
			Shape.Circle(r) => { return r > 0.0; }
			Shape.Point => { return false; }
		}
	}
}

fn main() void {
	var c Shape = Shape.Circle(5.0);
	if c.isRound() {
		return;
	}
}
`)
}

func TestEnumStaticMethod(t *testing.T) {
	checkNoErrors(t, `
enum Color {
	Red, Green,

	static fn count() i32 {
		return 2;
	}
}

fn main() void {
	var n i32 = Color.count();
	return;
}
`)
}

// === str type ===

func TestStrEqualityValid(t *testing.T) {
	checkNoErrors(t, `
fn main() void {
	var a str = "hello";
	var b str = "hello";
	if a == b { return; }
	if a != "world" { return; }
}
`)
}

func TestStrOrderingRejected(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var a str = "apple";
	if a < "banana" { return; }
}
`, "not defined on str (str supports only == and !=)")
}

func TestStrArithmeticRejected(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var a str = "a";
	var b str = a + "b";
}
`, "operator + not defined on a (type str)")
}

func TestSwitchOnStrRejected(t *testing.T) {
	checkHasError(t, `
fn main() void {
	var a str = "apple";
	switch a {
		"apple" => { }
		_ => { }
	}
}
`, "cannot switch on value of type str")
}
