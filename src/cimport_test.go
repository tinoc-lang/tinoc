package src

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// === C-Interop Tests (#importc + extern "C" fn) ===
//
// End-to-end tests for the two C-interop mechanisms:
//
//  1. `#importc "stdio.h" as cio;` — parse a real C header (clang JSON
//     AST when clang is installed, gcc -aux-info otherwise) and call its
//     functions / read its constants through a type-checked alias.
//  2. `extern "C" fn printf(fmt *const char, ...) i32;` — hand-declared
//     C functions, no header parsing involved.
//
// These tests generate real C99, compile it with the host C compiler,
// run the resulting binary, and assert on actual stdout / exit codes,
// proving the whole pipeline (lex -> parse -> sema -> codegen -> C
// compiler -> execution) for real libc calls.

// compileAndRunC runs a Tinoc program and returns (stdout, exit code),
// exactly like compileAndRun but the source is expected to use the
// `tinoc_exit` idiom internally via an extern "C" declaration (no
// harness stub swapping needed).
func compileAndRunWithExitDecl(t *testing.T, source string) (string, int) {
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
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("cannot run compiled binary: %v", err)
		}
	}
	return stdout.String(), exitCode
}

// === #importc ===

func TestCImport_PrintfStringLiteral(t *testing.T) {
	out, code := compileAndRun(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.printf("%s\n", "Hello World");
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "Hello World\n" {
		t.Fatalf("expected %q, got %q", "Hello World\n", out)
	}
}

func TestCImport_PrintfStrVariable(t *testing.T) {
	out, code := compileAndRun(t, `
#importc "stdio.h" as cio;

fn main() void {
	var name str = "Tinoc";
	cio.printf("hello %s!\n", name);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "hello Tinoc!\n" {
		t.Fatalf("expected %q, got %q", "hello Tinoc!\n", out)
	}
}

func TestCImport_PrintfIntVar(t *testing.T) {
	out, code := compileAndRun(t, `
#importc "stdio.h" as cio;

fn main() void {
	var answer i32 = 42;
	cio.printf("%d\n", answer);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "42\n" {
		t.Fatalf("expected %q, got %q", "42\n", out)
	}
}

func TestCImport_MultipleHeadersAndReturnValues(t *testing.T) {
	out, code := compileAndRun(t, `
#importc "stdio.h" "string.h" as c;

fn main() void {
	var msg str = "hello";
	// strlen's C return type is size_t (mapped to u64 on LP64); %zu is
	// the exact size_t format, and passing the call result straight
	// through avoids any tinoc-level type juggling.
	c.printf("len=%zu\n", c.strlen(msg));
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "len=5\n" {
		t.Fatalf("expected %q, got %q", "len=5\n", out)
	}
}

func TestCImport_EnumConstantsAndMacros(t *testing.T) {
	// EOF is a macro constant in stdio.h; it should be exposed through
	// the alias as a typed i32 value on both the clang and gcc paths.
	out, code := compileAndRun(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.printf("EOF=%d\n", cio.EOF);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "EOF=-1\n" {
		t.Fatalf("expected %q, got %q", "EOF=-1\n", out)
	}
}

func TestCImport_ExternVars(t *testing.T) {
	// Extern globals (stdin/stdout/stderr) come from clang's JSON AST;
	// gcc's -aux-info fallback cannot recover variables, so this test
	// only runs when the clang path is active.
	d, err := findHeaderDumper()
	if err != nil || d.Kind != "clang" {
		t.Skip("extern var import requires the clang AST path")
	}
	out, code := compileAndRun(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.fprintf(cio.stdout, "hi\n");
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "hi\n" {
		t.Fatalf("expected %q, got %q", "hi\n", out)
	}
}

func TestCImport_MathLibrary(t *testing.T) {
	out, code := compileAndRun(t, `
#importc "stdio.h" "math.h" as c;

fn main() void {
	var root f64 = c.sqrt(16.0);
	c.printf("%.1f\n", root);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "4.0\n" {
		t.Fatalf("expected %q, got %q", "4.0\n", out)
	}
}

func TestCImport_DefaultAliasFromHeaderName(t *testing.T) {
	// No `as alias` — the alias defaults to the header's file stem.
	out, code := compileAndRun(t, `
#importc "stdio.h";

fn main() void {
	stdio.printf("no alias needed\n");
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "no alias needed\n" {
		t.Fatalf("expected %q, got %q", "no alias needed\n", out)
	}
}

func TestCImport_UndefinedMember(t *testing.T) {
	checkHasError(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.doesNotExist();
}
`, "undefined: cio.doesNotExist")
}

func TestCImport_WrongArgCount(t *testing.T) {
	checkHasError(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.printf();
}
`, "not enough arguments")
}

func TestCImport_WrongArgType(t *testing.T) {
	checkHasError(t, `
#importc "stdio.h" as cio;

fn main() void {
	var n i32 = 5;
	cio.fputs(n, cio.stdout);
}
`, "argument 1")
}

func TestCImport_DuplicateAlias(t *testing.T) {
	checkHasError(t, `
#importc "stdio.h" as c;
#importc "stdlib.h" as c;

fn main() void {
	c.printf("x\n");
}
`, "already imported")
}

// === extern "C" fn ===

func TestExternC_BasicCall(t *testing.T) {
	out, code := compileAndRun(t, `
extern "C" fn printf(fmt *const char, ...) i32;

fn main() void {
	printf("Raw C call: %d\n", 42);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "Raw C call: 42\n" {
		t.Fatalf("expected %q, got %q", "Raw C call: 42\n", out)
	}
}

func TestExternC_StrArgument(t *testing.T) {
	out, code := compileAndRun(t, `
extern "C" fn printf(fmt *const char, ...) i32;

fn main() void {
	var lang str = "Tinoc";
	printf("%s rocks\n", lang);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "Tinoc rocks\n" {
		t.Fatalf("expected %q, got %q", "Tinoc rocks\n", out)
	}
}

func TestExternC_ReturnValueUsed(t *testing.T) {
	out, code := compileAndRun(t, `	extern "C" fn strlen(s *const char) usize;
	extern "C" fn printf(fmt *const char, ...) i32;

fn main() void {
	var n = strlen("abcdef");
	printf("len=%zu\n", n);
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "len=6\n" {
		t.Fatalf("expected %q, got %q", "len=6\n", out)
	}
}

func TestExternC_CustomSymbol(t *testing.T) {
	// `fn my_puts.puts` declares a Tinoc callable named my_puts that maps
	// to the real C symbol `puts`.
	out, code := compileAndRun(t, `
extern "C" fn my_puts.puts(s *const char) i32;

fn main() void {
	my_puts("custom symbol");
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "custom symbol\n" {
		t.Fatalf("expected %q, got %q", "custom symbol\n", out)
	}
}

func TestExternC_ExitCodeViaC(t *testing.T) {
	_, code := compileAndRunWithExitDecl(t, `
extern "C" fn exit(status i32) void;

fn main() void {
	exit(7);
}
`)
	if code != 7 {
		t.Fatalf("expected exit 7, got %d", code)
	}
}

func TestExternC_VoidFunctionCall(t *testing.T) {
	_, code := compileAndRunWithExitDecl(t, `
extern "C" fn exit(status i32) void;
extern "C" fn puts(s *const char) i32;

fn main() void {
	puts("side effect");
	exit(3);
}
`)
	if code != 3 {
		t.Fatalf("expected exit 3, got %d", code)
	}
}

func TestExternC_ComptimeSafetyWrongArgType(t *testing.T) {
	checkHasError(t, `
extern "C" fn strlen(s *const char) usize;

fn main() void {
	var n = strlen(42);
}
`, "argument 1")
}

func TestExternC_ComptimeSafetyWrongArgCount(t *testing.T) {
	checkHasError(t, `
extern "C" fn strlen(s *const char) usize;

fn main() void {
	var n = strlen("a", "b");
}
`, "not enough arguments")
}

func TestExternC_UndefinedExternCall(t *testing.T) {
	checkHasError(t, `
extern "C" fn strlen(s *const char) usize;

fn main() void {
	unknown_fn();
}
`, "undefined: unknown_fn")
}

func TestExternC_BadLinkageSpec(t *testing.T) {
	checkHasError(t, `
extern "D" fn foo() i32;

fn main() void {
}
`, "linkage")
}

func TestExternC_VariadicNeedsNamedParam(t *testing.T) {
	checkHasError(t, `
extern "C" fn vprintf(...) i32;

fn main() void {
}
`, "named parameter")
}

func TestExternC_MixedWithImportc(t *testing.T) {
	// An extern "C" fn whose symbol is already declared by an #importc
	// header must not emit a duplicate/conflicting prototype — the
	// header's own declaration wins.
	out, code := compileAndRun(t, `
#importc "stdio.h" as cio;
extern "C" fn printf(fmt *const char, ...) i32;

fn main() void {
	printf("mixed %s\n", "works");
	cio.printf("and importc %s\n", "works");
}
`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "mixed works\nand importc works\n" {
		t.Fatalf("expected %q, got %q", "mixed works\nand importc works\n", out)
	}
}

// === local header (#importc "myheader.h") ===

func TestCImport_LocalHeader(t *testing.T) {
	cc := requireCC(t)
	dir := t.TempDir()

	header := `// local helper header for the tinoc test suite
extern int tinoc_double(int x);
int tinoc_double(int x) { return x * 2; }
`
	tnc := `#importc "mymath.h" as mm;

extern "C" fn exit(status i32) void;

fn main() void {
	var v = mm.tinoc_double(21);
	exit(v);
}
`

	if err := os.WriteFile(filepath.Join(dir, "mymath.h"), []byte(header), 0o644); err != nil {
		t.Fatalf("write header: %v", err)
	}
	tncPath := filepath.Join(dir, "main.tnc")
	if err := os.WriteFile(tncPath, []byte(tnc), 0o644); err != nil {
		t.Fatalf("write tnc: %v", err)
	}

	code, diags := GenerateC(tncPath, tnc)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("diagnostic: %s", d.String())
		}
		t.FailNow()
	}

	buildDir := t.TempDir()
	cFile := filepath.Join(buildDir, "main.c")
	if err := os.WriteFile(cFile, []byte(code), 0o644); err != nil {
		t.Fatalf("write C file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "tinoc.h"), []byte(RuntimeHeader), 0o644); err != nil {
		t.Fatalf("write tinoc.h: %v", err)
	}

	binPath := filepath.Join(buildDir, "main.bin")
	args := cc.BuildArgs(cFile, binPath, []string{buildDir, dir})
	buildCmd := exec.Command(cc.Path, args...)
	var buildOut strings.Builder
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("C compilation failed with %s: %v\n--- generated C ---\n%s\n--- compiler output ---\n%s", cc.Name, err, code, buildOut.String())
	}

	runCmd := exec.Command(binPath)
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			if exitErr.ExitCode() != 42 {
				t.Fatalf("expected exit 42, got %d", exitErr.ExitCode())
			}
			return
		}
		t.Fatalf("cannot run compiled binary: %v", err)
	}
	t.Fatal("expected non-zero exit (exit(42)), got success")
}
