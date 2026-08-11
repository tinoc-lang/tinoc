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
