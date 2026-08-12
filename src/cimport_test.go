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
// These tests generate real C11, compile it with the host C compiler,
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

// requireHeaderDumper skips the test when no clang or gcc is available to
// parse C headers, mirroring how requireCC skips the compile-and-run tests.
// The #importc error tests below drive sema through a real header dump, so
// without a working clang/gcc the expected diagnostic can never be produced
// — failing hard there would break `ci` on machines without a C toolchain
// (e.g. the Windows CI runner).
func requireHeaderDumper(t *testing.T) {
	t.Helper()
	if _, err := findHeaderDumper(); err != nil {
		t.Skipf("skipping: %v", err)
	}
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
	// Extern globals come from clang's JSON AST; gcc's -aux-info fallback
	// cannot recover variables, so this test only runs when the clang path
	// is active. A project-owned header is used instead of a libc one:
	// stdin/stdout/stderr are real extern declarations on glibc but macros
	// on macOS (the externs are __stdinp/__stdoutp/__stderrp), and other
	// libc externs (e.g. optind) are hidden by feature-test macros under
	// the strict -std=c11 used at compile time — so no libc symbol is
	// portable. The header declares the extern; a companion C file
	// compiled into the same binary provides its definition.
	d, err := findHeaderDumper()
	if err != nil || d.Kind != "clang" {
		t.Skip("extern var import requires the clang AST path")
	}
	cc := requireCC(t)
	dir := t.TempDir()

	header := `// extern vars for the tinoc extern-var import test
extern int tinoc_counter;
`
	defs := `// definitions backing the externs declared in externs.h
int tinoc_counter = 21;
`
	tnc := `#importc "externs.h" as ext;

extern "C" fn exit(status i32) void;

fn main() void {
	exit(ext.tinoc_counter * 2);
}
`

	if err := os.WriteFile(filepath.Join(dir, "externs.h"), []byte(header), 0o644); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "externs_def.c"), []byte(defs), 0o644); err != nil {
		t.Fatalf("write definitions: %v", err)
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

	cFile := filepath.Join(dir, "test.c")
	if err := os.WriteFile(cFile, []byte(code), 0o644); err != nil {
		t.Fatalf("write C file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tinoc.h"), []byte(RuntimeHeader), 0o644); err != nil {
		t.Fatalf("write tinoc.h: %v", err)
	}

	binPath := filepath.Join(dir, "test.bin")
	// BuildArgs compiles a single input; splice the extern definitions
	// file in alongside the generated C so the extern links.
	args := cc.BuildArgs("", binPath, []string{dir})
	args = args[:len(args)-2] // drop the empty input and -lm
	args = append(args, cFile, filepath.Join(dir, "externs_def.c"), "-lm")

	buildCmd := exec.Command(cc.Path, args...)
	var buildOut strings.Builder
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("C compilation failed with %s: %v\n--- generated C ---\n%s\n--- compiler output ---\n%s", cc.Name, err, code, buildOut.String())
	}

	runCmd := exec.Command(binPath)
	exitCode := 0
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("cannot run compiled binary: %v", err)
		}
	}
	if exitCode != 42 {
		t.Fatalf("expected exit 42 (extern var imported and read), got %d", exitCode)
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
	requireHeaderDumper(t)
	checkHasError(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.doesNotExist();
}
`, "undefined: cio.doesNotExist")
}

func TestCImport_WrongArgCount(t *testing.T) {
	requireHeaderDumper(t)
	checkHasError(t, `
#importc "stdio.h" as cio;

fn main() void {
	cio.printf();
}
`, "not enough arguments")
}

func TestCImport_WrongArgType(t *testing.T) {
	requireHeaderDumper(t)
	// cio.stdout is a macro on macOS (the extern is __stdoutp), so it is
	// not portable; the wrong-arg-type check works with fprintf instead.
	checkHasError(t, `
#importc "stdio.h" as cio;

fn main() void {
	var n i32 = 5;
	cio.fprintf(n, "x");
}
`, "argument 1")
}

func TestCImport_DuplicateAlias(t *testing.T) {
	requireHeaderDumper(t)
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

func TestCImport_LocalHeaderInSubdirModule(t *testing.T) {
	// A module in a subdirectory may #importc a header that lives next
	// to itself (`#importc "vecmath.h"` inside lib/vecmath.tnc with
	// vecmath.h beside it). The merged C is compiled from a scratch work
	// directory, so the C compiler needs every loaded module's directory
	// on its include path — not just the entry file's — for the quoted
	// include to resolve. This runs the same pipeline the CLI uses
	// (LoadRoot + GenerateAll + compileGeneratedC + moduleIncludeDirs),
	// so it guards the regression directly.
	cc := requireCC(t)
	requireHeaderDumper(t)
	dir := t.TempDir()

	header := `// local header next to a module in a subdirectory
int tinoc_quad(int x) { return x * 4; }
`
	mod := `module vecmath;
#importc "vecmath.h" as vm;

pub fn quad(x i32) i32 {
	return vm.tinoc_quad(x);
}
`
	entry := `#import lib.vecmath;

extern "C" fn exit(status i32) void;

fn main() void {
	var v = vecmath.quad(10);
	exit(v);
}
`

	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	// The header lives NEXT TO the module file (lib/vecmath.h), not next
	// to the entry file — the exact regression: only the entry file's
	// dir used to reach the C compiler's include path.
	if err := os.WriteFile(filepath.Join(dir, "lib", "vecmath.h"), []byte(header), 0o644); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib", "vecmath.tnc"), []byte(mod), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	entryPath := filepath.Join(dir, "main.tnc")
	if err := os.WriteFile(entryPath, []byte(entry), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	diags := NewDiagnostics(entryPath)
	state := NewCompileState()
	state.LoadRoot(entryPath, entry, diags)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("diagnostic: %s", d.String())
		}
		t.FailNow()
	}

	gen := NewCodegen(state.Root.Sema, diags)
	gen.sourceDir = filepath.Dir(entryPath)
	code := gen.GenerateAll(state)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("codegen diagnostic: %s", d.String())
		}
		t.FailNow()
	}
	if !strings.Contains(code, `#include "vecmath.h"`) {
		t.Fatalf("merged C should quote-include the local header:\n%s", code)
	}

	binPath := filepath.Join(dir, "main.bin")
	_, workDir, err := compileGeneratedC(cc, entryPath, code, binPath, false, false, false, moduleIncludeDirs(state))
	if err != nil {
		t.Fatalf("compileGeneratedC failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	runCmd := exec.Command(binPath)
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			if exitErr.ExitCode() != 40 {
				t.Fatalf("expected exit 40, got %d", exitErr.ExitCode())
			}
			return
		}
		t.Fatalf("cannot run compiled binary: %v", err)
	}
	t.Fatal("expected non-zero exit (exit(40)), got success")
}

func TestCImport_CacheDistinguishesSameNamedLocalHeaders(t *testing.T) {
	// Two projects can each have their own `vecmath.h` with different
	// contents. The parse cache is keyed on the wrapper text
	// (`#include "vecmath.h"`), which is identical for both — so the key
	// must also fold in the resolved local header path and content hash,
	// or the second parse is silently served from the first project's
	// (wrong) cached dump.
	requireHeaderDumper(t)
	t.Setenv("TINOC_CACHE_DIR", t.TempDir())
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "vecmath.h"), []byte("int tinoc_one(void);\n"), 0o644); err != nil {
		t.Fatalf("write header A: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "vecmath.h"), []byte("int tinoc_two(void);\n"), 0o644); err != nil {
		t.Fatalf("write header B: %v", err)
	}

	modA, err := ImportCHeaders("vm", []string{"vecmath.h"}, dirA)
	if err != nil {
		t.Fatalf("parse A: %v", err)
	}
	if _, ok := modA.Funcs["tinoc_one"]; !ok {
		t.Fatalf("header A parse should expose tinoc_one, funcs=%v", funcKeys(modA))
	}
	if _, ok := modA.Funcs["tinoc_two"]; ok {
		t.Fatalf("header A parse must not expose tinoc_two (stale cache of B?)")
	}

	modB, err := ImportCHeaders("vm", []string{"vecmath.h"}, dirB)
	if err != nil {
		t.Fatalf("parse B: %v", err)
	}
	if _, ok := modB.Funcs["tinoc_two"]; !ok {
		t.Fatalf("header B parse should expose tinoc_two, funcs=%v", funcKeys(modB))
	}
	if _, ok := modB.Funcs["tinoc_one"]; ok {
		t.Fatalf("header B parse must not expose tinoc_one (stale cache of A?)")
	}
}

func TestCImport_CacheInvalidatesOnHeaderEdit(t *testing.T) {
	// Editing a local header must invalidate its cached parse: the key
	// folds in the header's current content hash, so a build after the
	// edit sees the new declarations, not yesterday's dump.
	requireHeaderDumper(t)
	t.Setenv("TINOC_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	header := filepath.Join(dir, "mine.h")
	if err := os.WriteFile(header, []byte("int tinoc_first(void);\n"), 0o644); err != nil {
		t.Fatalf("write header: %v", err)
	}

	mod1, err := ImportCHeaders("m", []string{"mine.h"}, dir)
	if err != nil {
		t.Fatalf("parse v1: %v", err)
	}
	if _, ok := mod1.Funcs["tinoc_first"]; !ok {
		t.Fatalf("v1 parse should expose tinoc_first, funcs=%v", funcKeys(mod1))
	}

	if err := os.WriteFile(header, []byte("int tinoc_second(void);\n"), 0o644); err != nil {
		t.Fatalf("rewrite header: %v", err)
	}
	mod2, err := ImportCHeaders("m", []string{"mine.h"}, dir)
	if err != nil {
		t.Fatalf("parse v2: %v", err)
	}
	if _, ok := mod2.Funcs["tinoc_second"]; !ok {
		t.Fatalf("v2 parse should expose tinoc_second, funcs=%v", funcKeys(mod2))
	}
	if _, ok := mod2.Funcs["tinoc_first"]; ok {
		t.Fatalf("v2 parse must not expose tinoc_first (stale cache of v1?)")
	}
}

func funcKeys(m *CImportModule) []string {
	ks := make([]string, 0, len(m.Funcs))
	for k := range m.Funcs {
		ks = append(ks, k)
	}
	return ks
}
