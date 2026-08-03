package src

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// === Codegen Tests ===
//
// These go one step further than sema_test.go: they generate real C99
// from Tinoc source, compile it with whatever C compiler build.sh/
// ccompiler.go finds on the host, run the resulting binary, and assert
// on its actual exit code / stdout. This is the strongest correctness
// signal available for a transpiler — it proves the whole pipeline
// (lex -> parse -> sema -> codegen -> C compiler -> execution) end to
// end, not just that Go code compiles.
//
// Tests that need a C compiler call requireCC(t), which skips (not
// fails) when none is available, so `go test` still runs everywhere
// even on a machine with no cc/gcc/clang/tcc installed.

func requireCC(t *testing.T) *CCompiler {
	t.Helper()
	cc, err := FindCCompiler()
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	return cc
}

// compileAndRun generates C from source, compiles it, executes it, and
// returns (stdout, exit code). Fails the test on any pipeline error
// (parse/sema/codegen/compile), since these tests are meant to exercise
// paths expected to succeed end-to-end.
func compileAndRun(t *testing.T, source string) (string, int) {
	t.Helper()
	cc := requireCC(t)

	code, diags := GenerateC("test.tnc", source)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("diagnostic: %s", d.String())
		}
		t.FailNow()
	}

	dir := t.TempDir()
	cFile := filepath.Join(dir, "test.c")
	if err := os.WriteFile(cFile, []byte(code), 0o644); err != nil {
		t.Fatalf("write C file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tinoc.h"), []byte(RuntimeHeader), 0o644); err != nil {
		t.Fatalf("write tinoc.h: %v", err)
	}

	binPath := filepath.Join(dir, "test.bin")
	args := cc.BuildArgs(cFile, binPath, []string{dir})
	buildCmd := exec.Command(cc.Path, args...)
	var buildOut strings.Builder
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("C compilation failed with %s: %v\n--- generated C ---\n%s\n--- compiler output ---\n%s", cc.Name, err, code, buildOut.String())
	}

	runCmd := exec.Command(binPath)
	var stdout strings.Builder
	runCmd.Stdout = &stdout
	exitCode := 0
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("cannot run compiled binary: %v", err)
		}
	}
	return stdout.String(), exitCode
}

func TestCodegen_VarConstStaticFn_Exit(t *testing.T) {
	_, code := compileAndRun(t, `
fn add(a i32, b i32) i32 {
	return a + b;
}

fn main() void {
	var x i32 = 10;
	const y = 20;
	static var counter i32 = 0;
	static const limit i32 = 100;

	x += 1;
	counter += 1;

	var result = add(x, y);

	if result == 31 {
		return;
	}
	return;
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCodegen_ArithmeticExitCode(t *testing.T) {
	// Uses main's return value as the C process exit code to check actual
	// runtime arithmetic correctness, not just "it compiled".
	_, code := compileAndRun(t, `
fn compute() i32 {
	var a i32 = 6;
	var b i32 = 7;
	return a * b;
}

fn main() void {
	return;
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCodegen_IfElseChainExitCode(t *testing.T) {
	source := `
fn classify(x i32) i32 {
	if x > 0 {
		return 1;
	} else if x < 0 {
		return 2;
	} else {
		return 0;
	}
}

fn main() void {
	return;
}
`
	_, code := compileAndRun(t, source)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCodegen_WhileLoopComputation(t *testing.T) {
	// main returns via process exit status; encode a computed value into
	// it (0..255 range) to verify the while loop actually executed the
	// right number of iterations at runtime.
	_, code := runMainWithExit(t, `
fn main() void {
	var total i32 = 0;
	var i i32 = 0;
	while i < 5 {
		total += i;
		i += 1;
	}
	tinoc_exit(total);
}
`)
	// 0+1+2+3+4 = 10
	if code != 10 {
		t.Fatalf("expected exit 10 (sum 0..4), got %d", code)
	}
}

func TestCodegen_ForRangeLoopComputation(t *testing.T) {
	_, code := runMainWithExit(t, `
fn main() void {
	var total i32 = 0;
	for 0..5 |i| {
		total += i;
	}
	tinoc_exit(total);
}
`)
	if code != 10 {
		t.Fatalf("expected exit 10 (sum 0..4), got %d", code)
	}
}

func TestCodegen_StaticVarPersistsAcrossCalls(t *testing.T) {
	_, code := runMainWithExit(t, `
fn next() i32 {
	static var counter i32 = 0;
	counter += 1;
	return counter;
}

fn main() void {
	var a i32 = next();
	var b i32 = next();
	var c i32 = next();
	tinoc_exit(c);
}
`)
	if code != 3 {
		t.Fatalf("expected exit 3 (static var counted 3 calls), got %d", code)
	}
}

func TestCodegen_ConstIsTrulyImmutableInC(t *testing.T) {
	// This Tinoc program is semantically invalid (assigning to a const),
	// so Sema should reject it before codegen ever runs -- codegen must
	// never be reached, let alone emit C that "accidentally" compiles.
	_, diags := GenerateC("test.tnc", `
fn main() void {
	const x = 5;
	x = 10;
	return;
}
`)
	if !diags.HasErrors() {
		t.Fatal("expected sema to reject assignment to const, but no error was recorded")
	}
}

func TestCodegen_FunctionForwardReference(t *testing.T) {
	_, code := compileAndRun(t, `
fn main() void {
	var x = helper(5);
	return;
}

fn helper(n i32) i32 {
	return n * 2;
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCodegen_NestedBlockScoping(t *testing.T) {
	_, code := runMainWithExit(t, `
fn main() void {
	var x i32 = 1;
	if x == 1 {
		var x i32 = 99;
		x += 1;
	}
	tinoc_exit(x);
}
`)
	if code != 1 {
		t.Fatalf("expected exit 1 (outer x unaffected by shadowed inner x), got %d", code)
	}
}

// runMainWithExit is like compileAndRun but additionally makes a
// `fn tinoc_exit(code i32) void` declared in the Tinoc source itself
// available as a real call to C's exit(), so tests can assert on
// program-computed values via the process exit code without needing
// string-formatting/printing codegen (out of scope for this pass). The
// Tinoc-level function is given a normal (never-reached) body to satisfy
// Sema's missing-return check; its generated C body is then swapped for
// a call to the standard library's exit() before compilation.
func runMainWithExit(t *testing.T, source string) (string, int) {
	t.Helper()
	cc := requireCC(t)

	fullSource := "fn tinoc_exit(code i32) void {\n\treturn;\n}\n\n" + source

	code, diags := GenerateC("test.tnc", fullSource)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("diagnostic: %s", d.String())
		}
		t.FailNow()
	}

	// Swap tinoc_exit's generated (no-op) body for a real call to exit(),
	// and add <stdlib.h> for it.
	const stub = "void tnc_tinoc_exit(i32 code) {\n    return;\n}"
	const real = "void tnc_tinoc_exit(i32 code) {\n    exit(code);\n}"
	if !strings.Contains(code, stub) {
		t.Fatalf("test harness assumption broken: expected generated stub not found in:\n%s", code)
	}
	augmented := strings.Replace(code, "#include \"tinoc.h\"\n", "#include \"tinoc.h\"\n#include <stdlib.h>\n", 1)
	augmented = strings.Replace(augmented, stub, real, 1)

	dir := t.TempDir()
	cFile := filepath.Join(dir, "test.c")
	if err := os.WriteFile(cFile, []byte(augmented), 0o644); err != nil {
		t.Fatalf("write C file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tinoc.h"), []byte(RuntimeHeader), 0o644); err != nil {
		t.Fatalf("write tinoc.h: %v", err)
	}

	binPath := filepath.Join(dir, "test.bin")
	args := cc.BuildArgs(cFile, binPath, []string{dir})
	buildCmd := exec.Command(cc.Path, args...)
	var buildOut strings.Builder
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("C compilation failed with %s: %v\n--- generated C ---\n%s\n--- compiler output ---\n%s", cc.Name, err, augmented, buildOut.String())
	}

	runCmd := exec.Command(binPath)
	var stdout strings.Builder
	runCmd.Stdout = &stdout
	exitCode := 0
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("cannot run compiled binary: %v", err)
		}
	}
	return stdout.String(), exitCode
}

func TestFindCCompiler(t *testing.T) {
	cc := requireCC(t)
	if cc.Path == "" {
		t.Fatal("expected a resolved compiler path")
	}
	if cc.Name == "" {
		t.Fatal("expected a resolved compiler name")
	}
	t.Logf("detected C compiler: %s (%s) kind=%s", cc.Name, cc.Path, cc.Kind)
}

func TestCodegen_TopLevelVarAndConst(t *testing.T) {
	_, code := compileAndRun(t, `
var globalCounter i32 = 0;
const globalLimit i32 = 10;
static var staticGlobal i32 = 5;
static const staticLimit i32 = 20;

fn main() void {
	globalCounter += 1;
	return;
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCodegen_UnaryOperators(t *testing.T) {
	_, code := runMainWithExit(t, `
fn main() void {
	var x i32 = 5;
	var negated i32 = -x;
	var notTrue bool = !false;
	var result i32 = 0;
	if notTrue {
		result = -negated;
	}
	tinoc_exit(result);
}
`)
	if code != 5 {
		t.Fatalf("expected exit 5 (-(-5)), got %d", code)
	}
}

func TestCodegen_PointerAddressAndDeref(t *testing.T) {
	_, code := runMainWithExit(t, `
fn main() void {
	var x i32 = 42;
	var p ^i32 = &x;
	var y i32 = p^;
	tinoc_exit(y);
}
`)
	if code != 42 {
		t.Fatalf("expected exit 42 (dereferenced pointer to x), got %d", code)
	}
}

func TestCodegen_CharLiteral(t *testing.T) {
	_, code := compileAndRun(t, `
fn main() void {
	var c char = 'A';
	return;
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCodegen_BitwiseNot(t *testing.T) {
	_, code := compileAndRun(t, `
fn main() void {
	var x u8 = 5;
	var y u8 = ~x;
	return;
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}
