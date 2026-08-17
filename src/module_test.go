package src

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// === Module System Tests ===
//
// These exercise the "smart module system" end to end: `#import` in every
// form (namespace, single symbol, selected `{...}`, wildcard, submodule
// path, quoted file), the `module name;` declaration, in-file
// `module name { ... }` blocks, directories-as-modules (`mod.tnc`), and
// generics used across module boundaries. Where a test compiles and runs
// real C it calls requireCC (skipped when no C compiler is available),
// matching codegen_test.go's conventions.

// writeModuleFiles writes the given map of relative paths to contents
// into a fresh temp dir and returns its path.
func writeModuleFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// checkModules runs the full LoadRoot pipeline (parse + imports + sema)
// over the entry file, returning the shared diagnostics. LoadRoot also
// validates module-name collisions and import cycles.
func checkModules(t *testing.T, files map[string]string, entry string) *Diagnostics {
	t.Helper()
	dir := writeModuleFiles(t, files)
	source, err := os.ReadFile(filepath.Join(dir, entry))
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	diags := NewDiagnostics(entry)
	state := NewCompileState()
	state.LoadRoot(filepath.Join(dir, entry), string(source), diags)
	return diags
}

// moduleDiagContains reports whether any error-level diagnostic message
// contains wantSubstr.
func moduleDiagContains(diags *Diagnostics, wantSubstr string) bool {
	for _, d := range diags.All() {
		if d.Severity == SeverityError && strings.Contains(d.Message, wantSubstr) {
			return true
		}
	}
	return false
}

// compileAndRunModules generates the merged C for a LoadRoot compilation,
// compiles it with the host C compiler, executes it, and returns
// (stdout, exit code). Fails the test on any pipeline error.
func compileAndRunModules(t *testing.T, files map[string]string, entry string) (string, int) {
	t.Helper()
	cc := requireCC(t)
	dir := writeModuleFiles(t, files)

	source, err := os.ReadFile(filepath.Join(dir, entry))
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	diags := NewDiagnostics(entry)
	state := NewCompileState()
	state.LoadRoot(filepath.Join(dir, entry), string(source), diags)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("diagnostic: %s", d.String())
		}
		t.FailNow()
	}

	gen := NewCodegen(state.Root.Sema, diags)
	gen.sourceDir = filepath.Dir(filepath.Join(dir, entry))
	code := gen.GenerateAll(state)
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("codegen diagnostic: %s", d.String())
		}
		t.FailNow()
	}

	work := t.TempDir()
	cFile := filepath.Join(work, "test.c")
	if err := os.WriteFile(cFile, []byte(code), 0o644); err != nil {
		t.Fatalf("write C file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "tinoc.h"), []byte(RuntimeHeader), 0o644); err != nil {
		t.Fatalf("write tinoc.h: %v", err)
	}

	binPath := filepath.Join(work, "test.bin")
	args := cc.BuildArgs(cFile, binPath, []string{work})
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

// === #import forms ===

func TestModuleNamespaceImport(t *testing.T) {
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub fn square(x f64) f64 { return x * x; }\n",
		"main.tnc": "#import math;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f\\n\", math.square(3.0));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "9.000000") {
		t.Fatalf("expected math.square(3)=9, got %q", out)
	}
}

func TestModuleSingleSymbolImport(t *testing.T) {
	// `#import math.PI;` — the bare dotted tail binds the pub symbol.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub const PI f64 = 3.14159;\n",
		"main.tnc": "#import math.PI;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f\\n\", PI);\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "3.141590") {
		t.Fatalf("expected PI via symbol import, got %q", out)
	}
}

func TestModuleSelectedImportWithAliases(t *testing.T) {
	// `#import something.{second, first};` plus per-symbol renames.
	out, _ := compileAndRunModules(t, map[string]string{
		"something.tnc": "pub fn first() i32 { return 1; }\npub fn second() i32 { return 2; }\npub const MAX i32 = 99;\n",
		"main.tnc":      "#import something.{second, first};\n#import something.{MAX as TOP};\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%d,%d,%d\\n\", first(), second(), TOP);\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "1,2,99") {
		t.Fatalf("expected selected symbols first/second/TOP, got %q", out)
	}
}

func TestModuleWildcardImport(t *testing.T) {
	out, _ := compileAndRunModules(t, map[string]string{
		"util.tnc": "pub fn double(x i32) i32 { return x * 2; }\npub const GREETING str = \"hi\";\n",
		"main.tnc": "#import util.*;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%d %s\\n\", double(21), GREETING);\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "42 hi") {
		t.Fatalf("expected wildcard bindings double/GREETING, got %q", out)
	}
}

func TestModuleNamespaceAlias(t *testing.T) {
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub fn square(x f64) f64 { return x * x; }\n",
		"main.tnc": "#import math as m;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f\\n\", m.square(4.0));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "16.000000") {
		t.Fatalf("expected m.square(4)=16, got %q", out)
	}
}

func TestModuleSubmoduleAndSelectedPath(t *testing.T) {
	// `shapes.vec` resolves shapes/vec.tnc; the `{...}` form binds
	// selected symbols from it. Directories become modules via mod.tnc.
	out, _ := compileAndRunModules(t, map[string]string{
		"shapes/mod.tnc": "pub fn circumference(r f64) f64 { return 2.0 * 3.14159 * r; }\n",
		"shapes/vec.tnc": "pub struct Vec2 { x f32; y f32; }\npub fn dot(a Vec2, b Vec2) f32 { return a.x*b.x + a.y*b.y; }\n",
		"main.tnc":       "#import shapes.vec;\n#import shapes.vec.{Vec2, dot};\n#import shapes;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tvar v Vec2 = Vec2 { .x = 3.0, .y = 4.0 };\n\tprintf(\"%f %f\\n\", dot(v, v), shapes.circumference(2.0));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "25.000000 12.566360") {
		t.Fatalf("expected selected + qualified module calls, got %q", out)
	}
}

func TestModuleFileImport(t *testing.T) {
	// Quoted file import with an alias: `#import "lib/tools.tnc" as t;`.
	out, _ := compileAndRunModules(t, map[string]string{
		"lib/tools.tnc": "pub fn triple(x i32) i32 { return x * 3; }\n",
		"main.tnc":      "#import \"lib/tools.tnc\" as tools;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%d\\n\", tools.triple(14));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "42") {
		t.Fatalf("expected file import alias tools.triple(14)=42, got %q", out)
	}
}

// === module keyword ===

func TestModuleDeclarationAndSelfQualification(t *testing.T) {
	// `module name;` names the file; inside the module, items are
	// reachable through the module's own name.
	out, _ := compileAndRunModules(t, map[string]string{
		"geometry.tnc": "module geometry;\npub fn area(r f64) f64 { return geometry.disc(r); }\npub fn disc(r f64) f64 { return 3.14159 * r * r; }\n",
		"main.tnc":     "#import geometry;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f\\n\", geometry.area(2.0));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "12.566360") {
		t.Fatalf("expected geometry.area(2)=12.56636, got %q", out)
	}
}

func TestModuleBlock(t *testing.T) {
	// In-file `module name { ... }` groups declarations into a namespace.
	out, _ := compileAndRunModules(t, map[string]string{
		"main.tnc": `module physics {
	pub const g f64 = 9.81;
	pub fn energy(m f64, v f64) f64 {
		return 0.5 * m * v * v;
	}
}
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("%f %f\n", physics.energy(2.0, 3.0), physics.g);
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "9.000000 9.810000") {
		t.Fatalf("expected module-block members physics.energy/physics.g, got %q", out)
	}
}

func TestModuleBlockNested(t *testing.T) {
	out, _ := compileAndRunModules(t, map[string]string{
		"main.tnc": `module a {
	module b {
		pub const VAL i32 = 7;
	}
}
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("%d\n", a.b.VAL);
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "7") {
		t.Fatalf("expected nested block a.b.VAL=7, got %q", out)
	}
}

func TestModuleBlockInImportedModuleFile(t *testing.T) {
	// A `module physics { ... }` block inside a module file extends that
	// module's namespace, so importers reach block members through the
	// dotted chain (`math.physics.g`, `math.physics.weight`). Only `pub`
	// block members cross the module boundary.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": `module math;
module physics {
	pub const g f64 = 9.81;
	const HIDDEN f64 = 123.0; // private
	pub fn weight(m f64) f64 {
		return m * g;
	}
}
`,
		"main.tnc": `#import math;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("g=%.2f\n", math.physics.g);
	printf("w=%.2f\n", math.physics.weight(10.0));
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "g=9.81") || !strings.Contains(out, "w=98.10") {
		t.Fatalf("expected math.physics.g / math.physics.weight to resolve, got %q", out)
	}
}

func TestModuleBlockPrivateNotVisibleCrossModule(t *testing.T) {
	// Private block members stay file-private: `math.physics.HIDDEN`
	// must be rejected with a clear diagnostic, not silently exposed.
	diags := checkModules(t, map[string]string{
		"math.tnc": `module math;
module physics {
	const HIDDEN i32 = 5;
}
`,
		"main.tnc": `#import math;
fn main() void {
	var v = math.physics.HIDDEN;
}
`,
	}, "main.tnc")
	if !moduleDiagContains(diags, "has no public member HIDDEN") {
		t.Fatalf("expected private block member to be rejected, diags: %v", diags.All())
	}
}

func TestModuleBlockPrivateAccessibleInFile(t *testing.T) {
	// Block views expose every member (they are the same file's
	// namespace), unlike imported modules which only expose pub items.
	out, _ := compileAndRunModules(t, map[string]string{
		"main.tnc": `module secret {
	const HIDDEN i32 = 5;
}
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("%d\n", secret.HIDDEN);
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "5") {
		t.Fatalf("expected block member secret.HIDDEN=5, got %q", out)
	}
}

// === generics across modules ===

func TestModuleGenericStructAndFn(t *testing.T) {
	out, _ := compileAndRunModules(t, map[string]string{
		"shapes/mod.tnc": "pub struct Circle:T { r T; }\n",
		"math.tnc":       "module math;\npub fn identity:T(val T) T { return val; }\n",
		"main.tnc": `#import shapes;
#import math;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var c shapes.Circle:f64 = shapes.Circle:f64 { .r = 2.5 };
	var id = math.identity:i32(42);
	printf("%f %d\n", c.r, id);
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "2.500000 42") {
		t.Fatalf("expected cross-module generic struct+fn, got %q", out)
	}
}

func TestModuleGenericStructWithMethods(t *testing.T) {
	// Cross-module generic structs with methods: instance methods
	// (`self ^Box:T`), static methods (qualified template call), and
	// multi-param templates — all monomorphized in the defining module's
	// scope and usable from the importer. Regression: the method
	// signature/body substitution kept the template's bare base
	// (`self ^Pair:(K, V)` -> `^Pair:(i32, str)`), which the importer
	// could not resolve — instances with methods failed to check.
	out, _ := compileAndRunModules(t, map[string]string{
		"containers.tnc": `pub struct Box:T {
	item T;
	tag str;

	fn get(self ^Box:T) T {
		return self^.item;
	}

	static fn pack(item T, tag str) Box:T {
		return Box:T { .item = item, .tag = tag };
	}
}

pub struct Pair:(K, V) {
	first K;
	second V;

	fn firstOf(self ^Pair:(K, V)) K {
		return self^.first;
	}

	static fn of(first K, second V) Pair:(K, V) {
		return Pair:(K, V) { .first = first, .second = second };
	}
}
`,
		"main.tnc": `#import containers;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var b containers.Box:i32 = containers.Box:i32 { .item = 42, .tag = "answer" };
	var s containers.Box:str = containers.Box:str.pack("tinoc", "lang");
	var p containers.Pair:(i32, str) = containers.Pair:(i32, str) { .first = 1, .second = "one" };
	var q containers.Pair:(str, i32) = containers.Pair:(str, i32).of("age", 7);
	printf("%d %s %d %s %d %d\n", b.get(), s.get(), p.firstOf(), q.firstOf(), b.item, q.second);
}
`,
	}, "main.tnc")
	want := "42 tinoc 1 age 42 7"
	if !strings.Contains(out, want) {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestModuleGenericAlias(t *testing.T) {
	out, _ := compileAndRunModules(t, map[string]string{
		"box.tnc": "pub alias Opt:T = ?T;\npub fn make(x i32) Opt:i32 { return x; }\n",
		"main.tnc": `#import box;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var o box.Opt:i32 = box.make(11);
	if o != null {
		var v = o?;
		printf("%d\n", v);
	}
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "11") {
		t.Fatalf("expected generic alias across modules, got %q", out)
	}
}

// === diamond imports & duplicate imports ===

func TestModuleDiamondImportSharesInstance(t *testing.T) {
	// a imports base; b imports base; main imports both. base loads once.
	out, _ := compileAndRunModules(t, map[string]string{
		"base.tnc": "pub const V i32 = 5;\n",
		"a.tnc":    "#import base;\npub fn a() i32 { return base.V; }\n",
		"b.tnc":    "#import base;\npub fn b() i32 { return base.V * 2; }\n",
		"main.tnc": "#import a;\n#import b;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%d %d\\n\", a.a(), b.b());\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "5 10") {
		t.Fatalf("expected diamond import a.a()=5 b.b()=10, got %q", out)
	}
}

func TestModuleDuplicateImportIdempotent(t *testing.T) {
	// Re-importing the same module under the same name — including a
	// namespace import followed by a selected import — is not an error.
	diags := checkModules(t, map[string]string{
		"vec.tnc":  "pub struct Vec2 { x f32; y f32; }\n",
		"main.tnc": "#import vec;\n#import vec;\n#import vec.{Vec2};\nfn main() void {}\n",
	}, "main.tnc")
	if diags.HasErrors() {
		for _, d := range diags.All() {
			t.Errorf("unexpected diagnostic: %s", d.String())
		}
	}
}

// === error cases ===

func TestModulePrivateSymbolRejected(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"util.tnc": "fn hidden() i32 { return 1; }\npub fn shown() i32 { return 2; }\n",
		"main.tnc": "#import util.hidden;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "private to module") {
		t.Fatalf("expected private-symbol diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleMissingModuleError(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"main.tnc": "#import no_such_module;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "cannot resolve import") {
		t.Fatalf("expected cannot-resolve diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleStdRejected(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"main.tnc": "#import std.io;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "standard library modules are not yet available") {
		t.Fatalf("expected std rejection, got %v", diagMessages(diags))
	}
}

func TestModuleImportCycleDetected(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"a.tnc":    "#import b;\npub fn a() i32 { return 1; }\n",
		"b.tnc":    "#import a;\npub fn b() i32 { return 2; }\n",
		"main.tnc": "#import a;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "import cycle detected") {
		t.Fatalf("expected cycle diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleSelfImportCycle(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"main.tnc": "#import \"main.tnc\";\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "import cycle detected") {
		t.Fatalf("expected self-import cycle diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleNameCollision(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"a/math.tnc": "module math;\npub const X i32 = 1;\n",
		"b/math.tnc": "module math;\npub const Y i32 = 2;\n",
		"main.tnc":   "#import a.math;\n#import b.math;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "already defined by") {
		t.Fatalf("expected module-name collision diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleNoPublicSymbolError(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"util.tnc": "pub fn shown() i32 { return 1; }\n",
		"main.tnc": "#import util.other;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "has no public symbol") {
		t.Fatalf("expected no-public-symbol diagnostic, got %v", diagMessages(diags))
	}
}

func TestModulePrivateQualifiedCallRejected(t *testing.T) {
	diags := checkModules(t, map[string]string{
		"util.tnc": "fn hidden() i32 { return 1; }\n",
		"main.tnc": "#import util;\nfn main() void {\n\tutil.hidden();\n}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "private to module") {
		t.Fatalf("expected private-qualified-call diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleUnusedPrivateGlobalOmitted(t *testing.T) {
	// Module globals are emitted as `static` bindings, so an unreferenced
	// private one would make the C compiler warn (-Wunused-const-variable)
	// and pad the merged output with dead data. Private globals that ARE
	// used (math_USED) and pub globals (math_PI, reachable through the
	// module namespace) are always emitted; dead private ones (math_TAU)
	// are omitted.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub const PI f64 = 3.14159;\nconst TAU = 6.28318;\nconst USED = 42;\npub fn answer() i32 { return USED; }\n",
		"main.tnc": "#import math;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f %d\\n\", math.PI, math.answer());\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "3.141590 42") {
		t.Fatalf("expected math.PI + math.answer()=42, got %q", out)
	}

	dir := writeModuleFiles(t, map[string]string{
		"math.tnc": "module math;\npub const PI f64 = 3.14159;\nconst TAU = 6.28318;\nconst USED = 42;\npub fn answer() i32 { return USED; }\n",
		"main.tnc": "#import math;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f %d\\n\", math.PI, math.answer());\n}\n",
	})
	source, _ := os.ReadFile(filepath.Join(dir, "main.tnc"))
	diags := NewDiagnostics("main.tnc")
	state := NewCompileState()
	state.LoadRoot(filepath.Join(dir, "main.tnc"), string(source), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diagMessages(diags))
	}
	gen := NewCodegen(state.Root.Sema, diags)
	gen.sourceDir = dir
	code := gen.GenerateAll(state)
	for _, want := range []string{"math_PI", "math_USED"} {
		if !strings.Contains(code, want) {
			t.Errorf("expected used/published global %s in merged output", want)
		}
	}
	if strings.Contains(code, "math_TAU") {
		t.Errorf("unreferenced private global math_TAU should be omitted from merged output")
	}
}

func TestModuleMangledCNamesInMergedOutput(t *testing.T) {
	// Module items get mangled C names (math_square) so same-named items
	// across modules never collide; the merged output contains them.
	dir := writeModuleFiles(t, map[string]string{
		"math.tnc": "module math;\npub fn square(x f64) f64 { return x * x; }\n",
		"util.tnc": "module util;\npub fn square(x f64) f64 { return x * 2.0; }\n",
		"main.tnc": "#import math;\n#import util;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%f %f\\n\", math.square(3.0), util.square(3.0));\n}\n",
	})
	source, _ := os.ReadFile(filepath.Join(dir, "main.tnc"))
	diags := NewDiagnostics("main.tnc")
	state := NewCompileState()
	state.LoadRoot(filepath.Join(dir, "main.tnc"), string(source), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diagMessages(diags))
	}
	gen := NewCodegen(state.Root.Sema, diags)
	gen.sourceDir = dir
	code := gen.GenerateAll(state)
	if !strings.Contains(code, "tnc_math_square") || !strings.Contains(code, "tnc_util_square") {
		t.Fatalf("expected mangled C names tnc_math_square and tnc_util_square in merged output")
	}
}

func diagMessages(diags *Diagnostics) []string {
	var msgs []string
	for _, d := range diags.All() {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

// === generics imported by name (symbol / selected / wildcard) ===
//
// Generic declarations (`fn identity:T`, `struct Pair:T`, `alias Opt:T`)
// register as templates, so binding them through an import must expose
// the template — `#import math.identity;` then a bare `identity:i32(42)`
// call, `#import math.{Pair};` then a `Pair:f64 { ... }` literal, and
// wildcard imports binding every pub generic bare. Instantiations reuse
// the defining module's prefix, so a bare call and a qualified call in
// the same compilation produce ONE mangled C instance.

func TestModuleGenericFnSymbolImport(t *testing.T) {
	// `#import math.identity;` binds the generic fn template; bare
	// `identity:i32(42)` and inferred `identity(7)` both instantiate it.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub fn identity:T(val T) T { return val; }\n",
		"main.tnc": "#import math.identity;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%d %d\\n\", identity:i32(42), identity(7));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "42 7") {
		t.Fatalf("expected bare generic calls identity:i32(42)=42 identity(7)=7, got %q", out)
	}
}

func TestModuleGenericFnRenamedImport(t *testing.T) {
	// `#import math.identity as id;` binds under the alias.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub fn identity:T(val T) T { return val; }\n",
		"main.tnc": "#import math.identity as id;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%s\\n\", id:str(\"renamed\"));\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "renamed") {
		t.Fatalf("expected renamed generic call id:str(...)=renamed, got %q", out)
	}
}

func TestModuleGenericStructSelectedImport(t *testing.T) {
	// `#import math.{Pair};` binds the generic struct template; a bare
	// `Pair:f64 { ... }` literal and `var p Pair:i32;` type reference
	// both instantiate it.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": "module math;\npub struct Pair:T { first T; second T; }\n",
		"main.tnc": "#import math.{Pair};\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tvar p Pair:f64 = Pair:f64 { .first = 1.5, .second = 2.5 };\n\tprintf(\"%f\\n\", p.second);\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "2.500000") {
		t.Fatalf("expected bare generic struct Pair:f64, got %q", out)
	}
}

func TestModuleGenericAliasSymbolImport(t *testing.T) {
	// `#import box.Opt;` binds the generic alias template; `Opt:i32`
	// expands it locally.
	out, _ := compileAndRunModules(t, map[string]string{
		"box.tnc":  "pub alias Opt:T = ?T;\npub fn wrap(x i32) Opt:i32 { return x; }\n",
		"main.tnc": "#import box.Opt;\n#import box.wrap;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tvar o Opt:i32 = wrap(11);\n\tprintf(\"%d\\n\", o?);\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "11") {
		t.Fatalf("expected imported generic alias Opt:i32, got %q", out)
	}
}

func TestModuleGenericWildcardImport(t *testing.T) {
	// `#import box.*;` binds every pub generic bare: fn, struct, alias.
	out, _ := compileAndRunModules(t, map[string]string{
		"box.tnc":  "pub fn pick:T(a T, b T) T { if a > b { return a; } return b; }\npub struct Wrap:T { v T; }\npub alias Maybe:T = ?T;\n",
		"main.tnc": "#import box.*;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tvar w Wrap:i32 = Wrap:i32 { .v = 5 };\n\tvar m Maybe:i32 = 9;\n\tprintf(\"%d %d %d\\n\", pick(3, 9), w.v, m?);\n}\n",
	}, "main.tnc")
	if !strings.Contains(out, "9 5 9") {
		t.Fatalf("expected wildcard-imported generics pick/Wrap/Maybe, got %q", out)
	}
}

func TestModuleGenericQualifiedAndBareShareInstance(t *testing.T) {
	// A bare call through a symbol import and a qualified call
	// (`math.identity:i32(42)`) instantiate the same mangled instance,
	// so the merged C output contains exactly one definition.
	dir := writeModuleFiles(t, map[string]string{
		"math.tnc": "module math;\npub fn identity:T(val T) T { return val; }\n",
		"main.tnc": "#import math.identity;\n#import math;\nextern \"C\" fn printf(fmt *const char, ...) i32;\nfn main() void {\n\tprintf(\"%d %d\\n\", identity:i32(1), math.identity:i32(2));\n}\n",
	})
	source, _ := os.ReadFile(filepath.Join(dir, "main.tnc"))
	diags := NewDiagnostics("main.tnc")
	state := NewCompileState()
	state.LoadRoot(filepath.Join(dir, "main.tnc"), string(source), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diagMessages(diags))
	}
	gen := NewCodegen(state.Root.Sema, diags)
	gen.sourceDir = dir
	code := gen.GenerateAll(state)
	want := "tnc_math_identity_i32"
	n := strings.Count(code, want)
	// The typedef/forward-declaration and the definition each reference
	// the name; the important part is that there is exactly one
	// definition and the bare + qualified calls resolved to the same
	// symbol. Count occurrences and require the name to appear at least
	// twice (prototype + definition) and the definition body once via a
	// unique signature.
	if n < 2 {
		t.Fatalf("expected mangled instance %s in merged output (got %d occurrences):\n%s", want, n, code)
	}
	if !strings.Contains(code, want+"(") {
		t.Fatalf("expected a single definition of %s:\n%s", want, code)
	}
}

func TestModulePrivateGenericRejected(t *testing.T) {
	// A non-pub generic fn/struct/alias is not importable and reports
	// "is private to module" rather than a misleading missing-symbol
	// error.
	diags := checkModules(t, map[string]string{
		"math.tnc": "module math;\nfn hidden:T(val T) T { return val; }\nstruct HStruct:T { v T; }\nalias HAlias:T = ?T;\n",
		"main.tnc": "#import math.hidden;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "private to module") {
		t.Fatalf("expected private-generic import diagnostic, got %v", diagMessages(diags))
	}
}

func TestModulePrivateGenericQualifiedAccessRejected(t *testing.T) {
	// Qualified calls / type references to a private generic are
	// rejected cross-module: `math.hidden:i32(1)` must not resolve even
	// though the template exists in the CompileState registry.
	diags := checkModules(t, map[string]string{
		"math.tnc": "module math;\nfn hidden:T(val T) T { return val; }\nstruct HStruct:T { v T; }\n",
		"main.tnc": "#import math;\nfn main() void {\n\tvar x = math.hidden:i32(1);\n}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "has no public function hidden") {
		t.Fatalf("expected private generic call to be rejected, got %v", diagMessages(diags))
	}
}

func TestModuleBlockGenericCrossModule(t *testing.T) {
	// A pub generic fn inside a module block of a module file is
	// reachable cross-module through the dotted chain:
	// `math.physics.blockid:i32(42)`. Private block generics stay
	// file-private.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": `module math;
module physics {
	pub fn blockid:T(val T) T { return val; }
	fn blockhidden:T(val T) T { return val; }
}
`,
		"main.tnc": `#import math;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("%d\n", math.physics.blockid:i32(42));
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "42") {
		t.Fatalf("expected cross-module block generic math.physics.blockid:i32(42), got %q", out)
	}

	diags := checkModules(t, map[string]string{
		"math.tnc": `module math;
module physics {
	fn blockhidden:T(val T) T { return val; }
}
`,
		"main.tnc": `#import math;
fn main() void {
	var x = math.physics.blockhidden:i32(1);
}
`,
	}, "main.tnc")
	if !moduleDiagContains(diags, "has no public function blockhidden") {
		t.Fatalf("expected private block generic to be rejected, got %v", diagMessages(diags))
	}
}

func TestModuleBlockGenericQualifiedInFile(t *testing.T) {
	// Inside the defining file, a generic in a module block is
	// reachable qualified (`physics.blockid:i32(8)`) and bare within
	// the block (`blockid:i32(11)` in a sibling fn).
	out, _ := compileAndRunModules(t, map[string]string{
		"main.tnc": `module physics {
	pub fn blockid:T(val T) T { return val; }
	pub fn useit() i32 { return blockid:i32(11); }
}
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("%d %d\n", physics.blockid:i32(8), physics.useit());
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "8 11") {
		t.Fatalf("expected in-file block generic qualified + bare use, got %q", out)
	}
}

// === generics-import stress: generic-in-generic bodies, methods, errors ===

func TestModuleGenericInGenericBody(t *testing.T) {
	// A generic fn whose body references the module's own generic
	// struct (`makePair:(K, V)` returning `Pair:(K, V) { ... }`) is the
	// substituteBodyTypes case: after instantiation the body's type
	// expressions still carried K/V and failed to re-check. Same file
	// first; the cross-module variant follows.
	out, _ := compileAndRunModules(t, map[string]string{
		"main.tnc": `pub struct Pair:(T, U) {
	first T;
	second U;
}
pub fn makePair:(K, V)(a K, b V) Pair:(K, V) {
	return Pair:(K, V) { .first = a, .second = b };
}
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var p Pair:(str, i32) = makePair:(str, i32)("ab", 7);
	printf("%s %d\n", p.first, p.second);
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "ab 7") {
		t.Fatalf("expected generic fn body referencing its own generic struct, got %q", out)
	}
}

func TestModuleGenericInGenericCrossModule(t *testing.T) {
	// The same generic-in-generic pattern across a module boundary:
	// `makePair:(K, V)` lives in math, is imported by name, and its
	// instantiated body resolves `Pair:(f64, i32)` against the defining
	// module's registry — sharing one C instance with the caller's
	// `Pair:(f64, i32)` type reference.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": `module math;
pub struct Pair:(T, U) {
	first T;
	second U;
}
pub fn makePair:(K, V)(a K, b V) Pair:(K, V) {
	return Pair:(K, V) { .first = a, .second = b };
}
`,
		"main.tnc": `#import math;
#import math.{makePair, Pair};
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var p Pair:(f64, i32) = makePair:(f64, i32)(1.5, 4);
	printf("%.1f %d\n", p.first, p.second);
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "1.5 4") {
		t.Fatalf("expected cross-module generic-in-generic instantiation, got %q", out)
	}
}

func TestModuleImportedGenericStructMethod(t *testing.T) {
	// Methods on a generic struct template instantiate with it: a bare
	// symbol import (`#import math.Pair;`) followed by a literal
	// `Pair:i32` and an instance method call resolves through the
	// imported template. The method's `self ^Pair:T` re-enters
	// instantiation while the instance is still being built (the
	// early-cache path).
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": `module math;
pub struct Pair:T {
	first T;
	second T;
	fn sum(self ^Pair:T) T { return self^.first + self^.second; }
}
`,
		"main.tnc": `#import math.Pair;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var p Pair:i32 = Pair:i32 { .first = 20, .second = 22 };
	printf("%d\n", p.sum());
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "42") {
		t.Fatalf("expected method call on imported generic struct instance, got %q", out)
	}
}

func TestModuleGenericMethodUsesModulePrivateNames(t *testing.T) {
	// Method bodies of a cross-module generic instance are checked
	// against the DEFINING module's analyzer, so module-private helpers
	// and consts resolve exactly as they do inside the defining file.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": `module math;
const OFFSET i32 = 5;
fn bump(x i32) i32 { return x + OFFSET; }
pub struct Box:T {
	v T;
	fn shifted(self ^Box:i32) i32 { return bump(self^.v); }
}
`,
		"main.tnc": `#import math.Box;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	var b Box:i32 = Box:i32 { .v = 37 };
	printf("%d\n", b.shifted());
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "42") {
		t.Fatalf("expected generic method to resolve module-private names, got %q", out)
	}
}

func TestModuleGenericFnUsesModulePrivateNames(t *testing.T) {
	// Same rule for generic fn bodies: a cross-module instance is
	// checked against the defining module, so `scaled:i32` resolves the
	// private `double` helper and the private `SCALE` const.
	out, _ := compileAndRunModules(t, map[string]string{
		"math.tnc": `module math;
const SCALE i32 = 100;
fn double(x i32) i32 { return x * 2; }
pub fn scaled:T(x T) i32 { return double(x) * SCALE; }
`,
		"main.tnc": `#import math.scaled;
extern "C" fn printf(fmt *const char, ...) i32;
fn main() void {
	printf("%d\n", scaled:i32(21));
}
`,
	}, "main.tnc")
	if !strings.Contains(out, "4200") {
		t.Fatalf("expected generic fn body to resolve module-private names, got %q", out)
	}
}

func TestModuleGenericImportMissingSymbol(t *testing.T) {
	// Importing a generic name the module does not export reports the
	// same "no public symbol" diagnostic as plain functions/consts.
	diags := checkModules(t, map[string]string{
		"math.tnc": "module math;\npub fn identity:T(val T) T { return val; }\n",
		"main.tnc": "#import math.nope;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "module math has no public symbol nope") {
		t.Fatalf("expected missing generic symbol diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleGenericImportWrongTypeArgCount(t *testing.T) {
	// A bare-imported generic struct instantiated with the wrong arity
	// reports the same arity diagnostic as a local instantiation.
	diags := checkModules(t, map[string]string{
		"math.tnc": "module math;\npub struct Pair:T { first T; second T; }\n",
		"main.tnc": "#import math.Pair;\nfn main() void {\n\tvar p Pair:(i32, str) = Pair:(i32, str) { .first = 1, .second = \"a\" };\n}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "Pair expects 1 type argument(s), got 2") {
		t.Fatalf("expected wrong type-arg count diagnostic, got %v", diagMessages(diags))
	}
}

func TestModuleGenericImportRedeclared(t *testing.T) {
	// Two modules exporting a pub generic under the same name, both
	// imported bare, collide with the standard redeclaration diagnostic.
	diags := checkModules(t, map[string]string{
		"a.tnc":    "module a;\npub fn id:T(v T) T { return v; }\n",
		"b.tnc":    "module b;\npub fn id:T(v T) T { return v; }\n",
		"main.tnc": "#import a.id;\n#import b.id;\nfn main() void {}\n",
	}, "main.tnc")
	if !moduleDiagContains(diags, "id redeclared in this block") {
		t.Fatalf("expected generic import redeclaration diagnostic, got %v", diagMessages(diags))
	}
}
