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
