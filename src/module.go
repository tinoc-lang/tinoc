package src

import (
	"os"
	"path/filepath"
	"strings"
)

// === Modules & Generics ===
//
// This file implements the "smart module system" and monomorphized
// generics on top of the per-file Sema pipeline:
//
//   - `module name;` names a file's module; a file without one gets its
//     module name from its file stem. `module name { ... }` blocks group
//     declarations into an in-file namespace.
//   - `pub` marks a declaration as exported from its module; everything
//     else is private and rejected when imported.
//   - `#import` resolves module paths relative to the importing file's
//     directory: `a.b.c` looks for `a/b/c.tnc`, then the directory form
//     `a/b/c/mod.tnc` / `a/b/c/c.tnc`. Imports load recursively (with
//     cycle detection) and cache by absolute file path, so diamond
//     imports share one module instance.
//   - Every module's code is merged into a single C translation unit
//     (GenerateAll); module items get mangled C names
//     (`math.abs` -> `tnc_math_abs`) so same-named items across modules
//     never collide.
//   - Generic `fn name:T(...)`, `struct Name:T {...}`, and
//     `alias Name:T = ...;` are monomorphized: each concrete type
//     argument set produces one fresh copy, checked and emitted with a
//     mangled C name (`Pair:i32` -> `tnc_Pair_i32`), cached by instance
//     key so each (decl, args) pair is instantiated exactly once.

// === Module Registry ===

// constExport is how modules expose top-level const/var bindings to
// importers: the resolved type plus the C name their codegen emitted.
type constExport struct {
	Name     string
	Type     *Type
	CName    string
	Mutable  bool // true for `pub var` exports (rare)
	Static   bool
	IsStatic bool
}

// TinocModule is one loaded source file treated as a module: its parsed
// program, its Sema instance, and the exported (pub) surface importers
// bind against. The same *TinocModule is shared by every file that
// imports the module (cached by absolute path).
type TinocModule struct {
	Name     string // canonical dotted module name (declared or derived)
	Path     string // absolute file path
	IsRoot   bool   // the entry file
	IsBlock  bool   // synthetic view for an in-file `module name { ... }`
	Prog     *Program
	Sema     *Sema
	RootSema *Sema // nil for block views; the file's own sema otherwise

	// Pub* hold the module's exported surface (bare names -> items).
	PubFuncs  map[string]*Symbol
	PubTypes  map[string]*Type
	PubConsts map[string]*constExport

	// PubGeneric* hold the module's exported generic templates
	// (`fn identity:T`, `struct Pair:T`, `alias Opt:T = ?T;`), so
	// importers can bind them as single symbols, selected symbols, or
	// wildcards and then instantiate them bare (`identity:i32(42)`,
	// `Pair:i32 { ... }`, `Opt:i32`). The values are the shared
	// CompileState templates keyed by their canonical dotted name
	// (e.g. `math.identity`), so a bare instantiation in an importing
	// file produces the same mangled C instance as the qualified form.
	PubGenericFns     map[string]*GenericFnDecl
	PubGenericStructs map[string]*GenericStructDecl
	PubGenericAliases map[string]*GenericAliasDecl

	// Funcs holds every function (pub and private) for "symbol is
	// private" diagnostics on qualified access.
	Funcs map[string]*Symbol

	// PrivateGenerics holds the bare names of non-pub generic
	// declarations (fn/struct/alias), so importing them reports
	// "symbol x is private to module y" instead of a misleading
	// "no public symbol".
	PrivateGenerics map[string]bool

	// PrivateConsts holds the names of top-level const/var globals that
	// are NOT pub, so importers get an explicit "is private to module"
	// diagnostic instead of a generic "no public symbol" (the const
	// counterpart of Funcs).
	PrivateConsts map[string]bool

	// SubModules exposes in-file `module name { ... }` blocks of a
	// module file as nested namespaces, so importers can chain through
	// them (`module math; ... module physics { pub const g; }` ->
	// `math.physics.g`). Values are pub-only projections of the blocks:
	// blocks expose every member in-file (same-file namespace), but
	// across the module boundary only `pub` members are visible.
	SubModules map[string]*TinocModule
}

func newTinocModule(name, path string) *TinocModule {
	return &TinocModule{
		Name:              name,
		Path:              path,
		PubFuncs:          make(map[string]*Symbol),
		PubTypes:          make(map[string]*Type),
		PubConsts:         make(map[string]*constExport),
		PubGenericFns:     make(map[string]*GenericFnDecl),
		PubGenericStructs: make(map[string]*GenericStructDecl),
		PubGenericAliases: make(map[string]*GenericAliasDecl),
		Funcs:             make(map[string]*Symbol),
		PrivateConsts:     make(map[string]bool),
		PrivateGenerics:   make(map[string]bool),
		SubModules:        make(map[string]*TinocModule),
	}
}

// === Generic Declarations ===

// GenericFnDecl is a registered generic function template
// (`fn identity:T(x T) T`). Key is the canonical decl name (module +
// name); Prefix is the module/block name segments for C mangling; Short
// is the bare function name; Params the type parameter names.
// GenericFnDecl is a registered generic function template
// (`fn identity:T(x T) T`). Sema is the analyzer that owns the
// template's source file — instantiated bodies are checked
// against it so module-local names (consts, private functions,
// #importc aliases) resolve exactly as in the defining module.
type GenericFnDecl struct {
	Key    string
	Prefix []string
	Short  string
	Params []string
	Fn     *FunctionStatement
	Sema   *Sema
}

func (d *GenericFnDecl) hasParam(name string) bool {
	for _, p := range d.Params {
		if p == name {
			return true
		}
	}
	return false
}

// GenericStructDecl is a registered generic struct template
// (`struct Pair:T { ... }`). Sema is the analyzer that owns the
// template's source file; monomorphized method bodies are
// checked against it (see GenericFnDecl.Sema).
type GenericStructDecl struct {
	Key    string
	Prefix []string
	Short  string
	Params []string
	St     *StructStatement
	Sema   *Sema
}

// GenericAliasDecl is a registered generic alias template
// (`alias Opt:T = ?T;`).
type GenericAliasDecl struct {
	Key    string
	Prefix []string
	Short  string
	Params []string
	Type   TypeExpr
}

// StructInstance is one monomorphized generic struct: its concrete Type
// (canonical name + C name), the shared method table, and the cloned
// declaration whose method bodies codegen emits.
type StructInstance struct {
	Type    *Type
	Methods map[string]*Symbol
	Decl    *StructStatement
	// Sema is the analyzer that instantiated this struct: the one whose
	// registries (structTypes/structMethods) and resolved-type side
	// tables cover the instance's field types and method bodies, which
	// merged codegen needs to emit them.
	Sema *Sema
}

// FnInstance is one monomorphized generic function: the cloned
// declaration plus the Sema that instantiated it (whose registries and
// resolved-type tables cover the clone's body).
type FnInstance struct {
	Fn   *FunctionStatement
	Sema *Sema
}

// === CompileState ===

// CompileState is the whole-compilation registry shared by every file's
// Sema: the module table (cached by absolute path), the generic
// declaration tables, and the monomorphized instance caches.
type CompileState struct {
	Root       *TinocModule
	Modules    map[string]*TinocModule // absolute path -> module
	ModuleList []*TinocModule          // load order (imports precede importers)
	nameOwners map[string]string       // module name -> absolute path (collision guard)
	loading    []string                // absolute paths being loaded (cycle detection)

	GenericFns     map[string]*GenericFnDecl
	GenericStructs map[string]*GenericStructDecl
	GenericAliases map[string]*GenericAliasDecl

	FnInstances     map[string]*FunctionStatement
	StructInstances map[string]*StructInstance

	// InstantiatedFns / InstantiatedStructs are the emission queues:
	// every monomorphized copy, in creation order, whose C definition
	// GenerateAllC appends after the regular declarations. Each entry
	// carries the Sema that created it so merged codegen can emit its
	// body against the right registries.
	InstantiatedFns     []*FnInstance
	InstantiatedStructs []*StructInstance
}

// NewCompileState creates an empty compilation state.
func NewCompileState() *CompileState {
	return &CompileState{
		Modules:         make(map[string]*TinocModule),
		nameOwners:      make(map[string]string),
		GenericFns:      make(map[string]*GenericFnDecl),
		GenericStructs:  make(map[string]*GenericStructDecl),
		GenericAliases:  make(map[string]*GenericAliasDecl),
		FnInstances:     make(map[string]*FunctionStatement),
		StructInstances: make(map[string]*StructInstance),
	}
}

// === Module Loading ===

// LoadRoot parses, imports, and fully checks the entry file (plus every
// transitively imported module) into this CompileState. diags is shared
// by every file so diagnostics interleave in one list. The root is
// loaded through the ordinary module path so it participates in the same
// push/pop loading-stack discipline as every imported module.
func (cs *CompileState) LoadRoot(file, source string, diags *Diagnostics) {
	cs.Root = cs.loadModuleFile(file, source, diags)
}

// loadModuleFile loads one module file: parse, register, resolve its
// imports (recursively), run its Sema, and publish its exports. Cached
// by absolute path; the root file's source may be passed in directly.
func (cs *CompileState) loadModuleFile(path, source string, diags *Diagnostics) *TinocModule {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)

	// Cycle detection runs BEFORE the Modules cache: any path in the
	// loading stack is by definition mid-load (registered but not yet
	// fully checked), so re-entering it through an import chain is a
	// genuine cycle — A -> B -> A — and must be reported rather than
	// silently reusing an incomplete module. The root file lands in the
	// stack here, not in LoadRoot, so plain entry files are never
	// misreported as importing themselves.
	for _, l := range cs.loading {
		if l == abs {
			diags.Error("module", 0, 0, "import cycle detected: %s", strings.Join(append(append([]string{}, cs.loading...), abs), " -> "))
			stub := newTinocModule(filepath.Base(abs), abs)
			stub.Prog = &Program{}
			cs.Modules[abs] = stub
			return stub
		}
	}
	if mod, ok := cs.Modules[abs]; ok {
		return mod
	}
	cs.loading = append(cs.loading, abs)

	if source == "" {
		data, err := os.ReadFile(abs)
		if err != nil {
			diags.Error("module", 0, 0, "cannot read module file %s: %v", abs, err)
			stub := newTinocModule(filepath.Base(abs), abs)
			stub.Prog = &Program{}
			cs.Modules[abs] = stub
			cs.loading = cs.loading[:len(cs.loading)-1]
			return stub
		}
		source = string(data)
	}

	prog, parseErrs := ParseSource(source)
	for _, pe := range parseErrs {
		diags.items = append(diags.items, diagFromParseError(abs, pe))
	}

	modName := declaredModuleName(prog)
	if modName == "" {
		modName = defaultModuleName(abs)
	}

	// Module-name collision guard: two different files claiming the
	// same module name would mangle to the same C symbols.
	if owner, ok := cs.nameOwners[modName]; ok && owner != abs {
		diags.Error("module", 0, 0, "module %s is already defined by %s (this file is %s)", modName, owner, abs)
	}

	mod := newTinocModule(modName, abs)
	mod.IsRoot = len(cs.loading) == 1
	mod.Prog = prog

	sema := NewSema(diags)
	sema.state = cs
	sema.moduleName = modName
	sema.sourceDir = filepath.Dir(abs)
	sema.module = mod
	sema.selfView = mod
	mod.Sema = sema
	mod.RootSema = sema

	cs.Modules[abs] = mod
	cs.ModuleList = append(cs.ModuleList, mod)
	cs.nameOwners[modName] = abs

	// Bind the file's own module name as a namespace so in-file code
	// can qualify its own items (`module math; ... math.abs(1.0)`).
	if modName != "" && !mod.IsRoot {
		sema.modules[modName] = mod
	}

	if len(parseErrs) == 0 {
		sema.processImports()
		sema.Check(prog)
	}

	// Publish the module's full function table (pub and private) so
	// qualified access and symbol imports can tell "private" apart
	// from "missing" (`module math has no public function x` vs
	// `symbol x is private to module math`).
	for name, sym := range sema.funcs {
		mod.Funcs[name] = sym
	}

	cs.loading = cs.loading[:len(cs.loading)-1]
	return mod
}

// declaredModuleName returns the dotted name from the file's
// `module name;` declaration, if any.
func declaredModuleName(prog *Program) string {
	for _, stmt := range prog.Statements {
		if mds, ok := stmt.(*ModuleDeclStatement); ok && len(mds.Name) > 0 {
			return strings.Join(mds.Name, ".")
		}
		// A module block is not a file-level declaration; keep scanning.
		if mb, ok := stmt.(*ModuleBlockStatement); ok {
			for _, inner := range mb.Statements {
				if mds, ok := inner.(*ModuleDeclStatement); ok && len(mds.Name) > 0 {
					return strings.Join(mds.Name, ".")
				}
			}
		}
	}
	return ""
}

// defaultModuleName derives a module name from a file path: the file
// stem, or the containing directory's name for convention files named
// `mod.tnc` / `index.tnc` (the "directories become modules" rule:
// `app/util/mod.tnc` is the module `util`).
func defaultModuleName(path string) string {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "mod" || stem == "index" {
		if dir := filepath.Base(filepath.Dir(path)); dir != "" && dir != "." && dir != string(filepath.Separator) {
			return dir
		}
	}
	return stem
}

// tryLoadModulePath resolves a dotted import path to a module file:
// `a.b.c` -> `<fromDir>/a/b/c.tnc`, else the directory form
// `<fromDir>/a/b/c/mod.tnc` / `<fromDir>/a/b/c/c.tnc`.
func (cs *CompileState) tryLoadModulePath(s *Sema, fromDir string, segs []string) (*TinocModule, bool) {
	if len(segs) == 0 {
		return nil, false
	}
	joined := filepath.Join(append([]string{fromDir}, segs...)...)
	last := segs[len(segs)-1]

	for _, cand := range []string{joined + ".tnc", filepath.Join(joined, "mod.tnc"), filepath.Join(joined, last+".tnc")} {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cs.loadModuleFile(cand, "", s.diags), true
		}
	}
	return nil, false
}

// processImports resolves every top-level #import in this file,
// recursively loading the referenced modules and binding their symbols
// into this Sema's scope. Runs before Check's own passes so struct
// bodies and function signatures can reference imported names.
func (s *Sema) processImports() {
	if s.state == nil {
		return
	}
	for _, stmt := range s.ProgStatements() {
		if is, ok := stmt.(*ImportStatement); ok {
			s.processImport(is)
		}
	}
}

// ProgStatements returns the file's top-level statements (used by
// processImports and the block-walking passes).
func (s *Sema) ProgStatements() []Statement {
	if s.module != nil && s.module.Prog != nil {
		return s.module.Prog.Statements
	}
	return nil
}

func (s *Sema) processImport(is *ImportStatement) {
	line, col := is.Token.Line, is.Token.Column

	// Quoted file import: `#import "rel/path.tnc"`.
	if is.IsFileImport {
		path := filepath.Join(s.sourceDir, is.FilePath)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			s.errorAt(line, col, "cannot resolve import %q (no such file)", is.FilePath)
			return
		}
		mod := s.state.loadModuleFile(path, "", s.diags)
		s.bindModuleImport(is, mod, moduleBindingName(is, mod))
		return
	}

	if len(is.Path) == 0 {
		return
	}

	// The standard library is a future language feature; reject std.*
	// imports explicitly rather than with a confusing file-not-found.
	if is.Path[0] == "std" {
		s.errorAt(line, col, "standard library modules are not yet available (%s)", strings.Join(is.Path, "."))
		return
	}

	// A full-path module: `#import math` / `#import math.util`.
	if mod, ok := s.state.tryLoadModulePath(s, s.sourceDir, is.Path); ok {
		s.bindModuleImport(is, mod, moduleBindingName(is, mod))
		return
	}

	// A bare dotted tail with no module file is a single-symbol import:
	// `#import math.PI` imports the pub symbol PI of module math (with
	// an optional rename: `#import math.PI as pi;`).
	if len(is.Path) > 1 {
		prefix := is.Path[:len(is.Path)-1]
		if mod, ok := s.state.tryLoadModulePath(s, s.sourceDir, prefix); ok {
			s.bindSymbolImport(is, mod, is.Path[len(is.Path)-1], is.ModuleAlias, line, col)
			return
		}
	}

	s.errorAt(line, col, "cannot resolve import %q (no such module)", strings.Join(is.Path, "."))
}

// moduleBindingName decides the local namespace name an import binds:
// the `as alias` if given, else the last path segment (for quoted file
// imports, the module's declared name if present, else the file stem).
func moduleBindingName(is *ImportStatement, mod *TinocModule) string {
	if is.ModuleAlias != "" {
		return is.ModuleAlias
	}
	if is.IsFileImport {
		if mod.Name != "" {
			return strings.Split(mod.Name, ".")[len(strings.Split(mod.Name, "."))-1]
		}
		base := filepath.Base(is.FilePath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return is.Path[len(is.Path)-1]
}

// bindModuleImport binds a module namespace: `#import math as m;` makes
// `m.symbol` usable. Namespace-only imports never bind bare symbols.
func (s *Sema) bindModuleImport(is *ImportStatement, mod *TinocModule, localName string) {
	if localName == "" {
		return
	}
	// Re-importing the same module under the same local name is
	// idempotent (`#import vec; #import vec;` and the common
	// `#import vec; #import vec.{a, b};` pair both bind `vec` once, the
	// second call adding its selected symbols). Only a genuinely
	// different module under a taken name is an error.
	if dup, ok := s.modules[localName]; ok {
		if dup != mod {
			s.errorAt(is.Token.Line, is.Token.Column, "%s already bound in this file (module namespace)", localName)
			return
		}
	} else {
		s.modules[localName] = mod
	}

	// Wildcard: `#import math.*;` binds every pub symbol bare.
	if is.Wildcard {
		for name, sym := range mod.PubFuncs {
			s.bindImportFunc(name, sym, is.Token.Line, is.Token.Column)
		}
		for name, ce := range mod.PubConsts {
			s.bindImportConst(name, ce, is.Token.Line, is.Token.Column)
		}
		for name, t := range mod.PubTypes {
			// A pub const/var export is mirrored into PubTypes; it was
			// already bound above via bindImportConst, so binding it a
			// second time as a type would falsely report a redeclaration.
			if _, isConst := mod.PubConsts[name]; isConst {
				continue
			}
			s.bindImportedType(name, t, mod, is.Token.Line, is.Token.Column)
		}
		for name, d := range mod.PubGenericFns {
			s.bindImportedGenericFn(name, d, is.Token.Line, is.Token.Column)
		}
		for name, d := range mod.PubGenericStructs {
			s.bindImportedGenericStruct(name, d, is.Token.Line, is.Token.Column)
		}
		for name, d := range mod.PubGenericAliases {
			s.bindImportedGenericAlias(name, d, is.Token.Line, is.Token.Column)
		}
		return
	}

	// Selected: `#import math.{a, b as c}`.
	if len(is.Symbols) > 0 {
		for _, sy := range is.Symbols {
			if sy == nil {
				continue
			}
			s.bindSymbolImport(is, mod, sy.Name, sy.Alias, is.Token.Line, is.Token.Column)
		}
	}
}

// bindSymbolImport binds one pub symbol of a module under a local name
// (the symbol's own name, or an alias for `{name as alias}` / the
// bare-tail `.symbol` form).
func (s *Sema) bindSymbolImport(is *ImportStatement, mod *TinocModule, name, alias string, line, col int) {
	localName := alias
	if localName == "" {
		localName = name
	}
	if fn := mod.PubFuncs[name]; fn != nil {
		s.bindImportFunc(localName, fn, line, col)
		return
	}
	if ce := mod.PubConsts[name]; ce != nil {
		s.bindImportConst(localName, ce, line, col)
		return
	}
	if t := mod.PubTypes[name]; t != nil {
		s.bindImportedType(localName, t, mod, line, col)
		return
	}
	if d := mod.PubGenericFns[name]; d != nil {
		s.bindImportedGenericFn(localName, d, line, col)
		return
	}
	if d := mod.PubGenericStructs[name]; d != nil {
		s.bindImportedGenericStruct(localName, d, line, col)
		return
	}
	if d := mod.PubGenericAliases[name]; d != nil {
		s.bindImportedGenericAlias(localName, d, line, col)
		return
	}
	if _, priv := mod.Funcs[name]; priv {
		s.errorAt(line, col, "symbol %s is private to module %s", name, mod.Name)
		return
	}
	if mod.PrivateConsts[name] {
		s.errorAt(line, col, "symbol %s is private to module %s", name, mod.Name)
		return
	}
	if mod.PrivateGenerics[name] {
		s.errorAt(line, col, "symbol %s is private to module %s", name, mod.Name)
		return
	}
	s.errorAt(line, col, "module %s has no public symbol %s", mod.Name, name)
}

// bindImportedGenericFn binds a generic function template from another
// module under a local name, so bare instantiations (`identity:i32(42)`,
// `identity(42)` with inference) resolve through lookupGenericFn. The
// shared decl keeps its defining module's prefix, so the instantiated C
// name (`tnc_math_identity_str`) matches what a qualified call in the
// same compilation produces — no duplicate instances.
func (s *Sema) bindImportedGenericFn(localName string, d *GenericFnDecl, line, col int) {
	if _, dup := s.importedGenericFns[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.funcs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.importConsts[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.typeAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	s.importedGenericFns[localName] = d
}

// bindImportedGenericStruct binds a generic struct template from
// another module under a local name (`#import shapes.Circle;`), so
// bare instantiations (`Circle:f64 { ... }`, `var c Circle:i32;`)
// resolve through lookupGenericStruct.
func (s *Sema) bindImportedGenericStruct(localName string, d *GenericStructDecl, line, col int) {
	if _, dup := s.importedGenericStructs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.typeAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.funcs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.importConsts[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	s.importedGenericStructs[localName] = d
}

// bindImportedGenericAlias binds a generic alias template from another
// module under a local name (`#import box.Opt;`), so bare expansions
// (`Opt:i32`) resolve.
func (s *Sema) bindImportedGenericAlias(localName string, d *GenericAliasDecl, line, col int) {
	if _, dup := s.importedGenericAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.typeAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.funcs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.importConsts[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	s.importedGenericAliases[localName] = d
}

func (s *Sema) bindImportFunc(localName string, sym *Symbol, line, col int) {
	if _, dup := s.funcs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.importConsts[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.typeAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	s.funcs[localName] = sym
	if !s.global.Define(sym) {
		s.errorAt(line, col, "%s redeclared in this block", localName)
	}
}

func (s *Sema) bindImportConst(localName string, ce *constExport, line, col int) {
	if _, dup := s.importConsts[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.funcs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.typeAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	sym := &Symbol{Name: localName, Kind: SymConst, Type: ce.Type, Mutable: ce.Mutable}
	sym.CName = ce.CName
	s.importConsts[localName] = sym
	if !s.global.Define(sym) {
		s.errorAt(line, col, "%s redeclared in this block", localName)
	}
}

// bindImportedType makes an imported aggregate type usable locally: it
// registers the shared *Type under the bare name (so `Circle { ... }`,
// `Circle.create()`, and `var c Circle;` all resolve) and mirrors the
// defining module's method table so method calls type-check.
func (s *Sema) bindImportedType(localName string, t *Type, mod *TinocModule, line, col int) {
	if _, dup := s.typeAliases[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.funcs[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	if _, dup := s.importConsts[localName]; dup {
		s.errorAt(line, col, "%s redeclared in this block", localName)
		return
	}
	s.typeAliases[localName] = t
	if mod == nil || mod.RootSema == nil {
		return
	}
	// Mirror the defining module's registries so every existing
	// code path (static calls, variant refs, method tables, field
	// access) works with the bare local name and the canonical key.
	owner := mod.RootSema
	switch t.Kind {
	case KindStruct:
		s.structTypes[localName] = t
		s.structTypes[t.Name] = t
		methods := owner.structMethods[localBareName(t, owner, "struct")]
		if methods == nil {
			methods = make(map[string]*Symbol)
		}
		s.structMethods[t.Name] = methods
		s.structMethods[localName] = methods
	case KindEnum:
		s.enumTypes[localName] = t
		s.enumTypes[t.Name] = t
		methods := owner.enumMethods[localBareName(t, owner, "enum")]
		if methods == nil {
			methods = make(map[string]*Symbol)
		}
		s.enumMethods[t.Name] = methods
		s.enumMethods[localName] = methods
	case KindUnion:
		s.unionTypes[localName] = t
		s.unionTypes[t.Name] = t
		methods := owner.unionMethods[localBareName(t, owner, "union")]
		if methods == nil {
			methods = make(map[string]*Symbol)
		}
		s.unionMethods[t.Name] = methods
		s.unionMethods[localName] = methods
	}
}

// localBareName recovers the bare declaration name of an imported type
// from the owner sema's canonical-name registry (imported types were
// registered under their canonical dotted name, e.g. "math.Circle").
func localBareName(t *Type, owner *Sema, kind string) string {
	switch kind {
	case "struct":
		for k := range owner.structTypes {
			if k != t.Name {
				// owner registers structs under their bare local name too
				// (root files) or canonical only; try the canonical's
				// registered entry directly.
				_ = k
			}
		}
		if reg, ok := owner.structTypes[t.Name]; ok && reg == t {
			// canonical key registered — find the bare key that maps to t
			for k, v := range owner.structTypes {
				if v == t && k != t.Name && !strings.Contains(k, ".") {
					return k
				}
			}
			return t.Name
		}
	case "enum":
		if _, ok := owner.enumTypes[t.Name]; ok {
			for k, v := range owner.enumTypes {
				if v == t && k != t.Name && !strings.Contains(k, ".") {
					return k
				}
			}
		}
		return t.Name
	case "union":
		if _, ok := owner.unionTypes[t.Name]; ok {
			for k, v := range owner.unionTypes {
				if v == t && k != t.Name && !strings.Contains(k, ".") {
					return k
				}
			}
		}
		return t.Name
	}
	return t.Name
}

// === Name Mangling ===

// itemPrefix returns the name segments of the module scope the current
// statement lives in: the file's module name plus any enclosing
// `module name { ... }` block prefix. The entry file's own module name is
// skipped so its items keep the legacy bare C naming (and the C entry
// point `main` stays `main`); module files and in-file module blocks
// still mangle.
func (s *Sema) itemPrefix() []string {
	var segs []string
	if s.moduleName != "" && !s.isRootFile() {
		segs = append(segs, strings.Split(s.moduleName, ".")...)
	}
	segs = append(segs, s.blockPrefix...)
	return segs
}

// isRootFile reports whether this Sema analyzes the compilation's entry
// file (whose top-level items keep the legacy bare C naming).
func (s *Sema) isRootFile() bool {
	return s.selfView != nil && s.selfView.IsRoot
}

// itemCanonical returns the canonical dotted name of a top-level item
// (`math.Circle`, `math.vec2.len`, or the bare name for the entry file).
func (s *Sema) itemCanonical(name string) string {
	segs := append(s.itemPrefix(), name)
	return strings.Join(segs, ".")
}

// itemCName returns the C identifier base for a top-level item in this
// scope: "" for entry-file items (existing naming applies), else the
// module-mangled form (`math_Circle`, `math_vec2_len`).
func (s *Sema) itemCName(name string) string {
	segs := s.itemPrefix()
	if len(segs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(segs)+1)
	for _, g := range segs {
		parts = append(parts, sanitizeCIdent(g))
	}
	parts = append(parts, sanitizeCIdent(name))
	return strings.Join(parts, "_")
}

// moduleSegsCName joins name segments into a C identifier
// (`math_vec2_len`), used for generic instance mangling.
func moduleSegsCName(prefix []string, names ...string) string {
	parts := make([]string, 0, len(prefix)+len(names))
	for _, g := range prefix {
		parts = append(parts, sanitizeCIdent(g))
	}
	for _, n := range names {
		parts = append(parts, sanitizeCIdent(n))
	}
	return strings.Join(parts, "_")
}

// genericInstanceKey builds the cache key for one monomorphization:
// decl key + each concrete arg's C-type identifier.
func genericInstanceKey(declKey string, argTypes []*Type) string {
	parts := []string{declKey}
	for _, t := range argTypes {
		if t == nil {
			parts = append(parts, "invalid")
			continue
		}
		parts = append(parts, cTypeIdentSpelling(t.CType()))
	}
	return strings.Join(parts, ":")
}

// argsDisplay renders `Pair:i32`-style display names for diagnostics.
func argsDisplay(argTypes []*Type) string {
	parts := make([]string, 0, len(argTypes))
	for _, t := range argTypes {
		parts = append(parts, t.String())
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// genericTypeCName mangles a generic struct instance's C typedef name:
// `Pair:i32` in module math -> `math_Pair_i32`; root -> `Pair_i32`.
func genericTypeCName(prefix []string, short string, argTypes []*Type) string {
	names := []string{short}
	for _, t := range argTypes {
		if t == nil {
			names = append(names, "invalid")
			continue
		}
		names = append(names, cTypeIdentSpelling(t.CType()))
	}
	return moduleSegsCName(prefix, names...)
}

// genericFnCName mangles a generic function instance's C name:
// `identity:str` in module math -> `tnc_math_identity_str`.
func genericFnCName(prefix []string, short string, argTypes []*Type) string {
	return "tnc_" + genericTypeCName(prefix, short, argTypes)
}

// === Qualified Type Resolution ===

// resolveQualifiedType resolves a dotted type name (`math.Circle`)
// through the bound module namespaces.
func (s *Sema) resolveQualifiedType(t *NamedType) *Type {
	segs := strings.Split(t.Name, ".")
	if len(segs) < 2 {
		return &Type{Kind: KindUnknown, Name: t.Name}
	}
	mod, ok := s.modules[segs[0]]
	if !ok {
		if s.state == nil {
			// Single-file legacy pipeline has no module namespaces.
			return &Type{Kind: KindUnknown, Name: t.Name}
		}
		s.errorAt(t.Token.Line, t.Token.Column, "undefined module %s", segs[0])
		return nil
	}
	rest := strings.Join(segs[1:], ".")
	if ty, ok := mod.PubTypes[rest]; ok {
		return ty
	}
	// Module-block members live under bare names; allow the last
	// segment to resolve them (`math.vec2.Point` -> pub type Point).
	if len(segs) > 2 {
		if ty, ok := mod.PubTypes[segs[len(segs)-1]]; ok {
			return ty
		}
	}
	// A block inside a module file extends its namespace
	// (`math.physics.Point`): descend into the block's pub projection.
	if len(segs) > 2 {
		if sub, ok := mod.SubModules[segs[1]]; ok {
			if ty, ok := sub.PubTypes[strings.Join(segs[2:], ".")]; ok {
				return ty
			}
		}
	}
	s.errorAt(t.Token.Line, t.Token.Column, "module %s has no public type %s", segs[0], rest)
	return nil
}

// tinocModule returns the user module bound to the expression (a bare
// identifier naming an imported module or an in-file module namespace).
func (s *Sema) tinocModule(e Expression) (*TinocModule, bool) {
	switch ex := e.(type) {
	case *Identifier:
		mod, ok := s.modules[ex.Value]
		return mod, ok
	case *FieldAccessExpression:
		// Dotted module chain: `a.b` -> the namespace bound under its
		// full dotted name (nested module blocks / dotted block names).
		if base, ok := s.tinocModule(ex.Left); ok && ex.Field != nil {
			if nmod, ok2 := s.modules[base.Name+"."+ex.Field.Value]; ok2 {
				return nmod, true
			}
			// Blocks inside a module file extend its namespace:
			// `math.physics` resolves through the file module's
			// SubModules (pub-only projection of the block).
			if base.SubModules != nil {
				if nmod, ok2 := base.SubModules[ex.Field.Value]; ok2 {
					return nmod, true
				}
			}
		}
		return nil, false
	}
	return nil, false
}

// lookupModuleGenericFn finds a generic function template exported by a
// module for qualified generic calls such as `math.identity:i32(42)`.
// Only pub templates are reachable cross-module: the module's own
// registry (selfView) and the pub-only block projections carry exactly
// the exported surface, so a private generic (`fn hidden:T ...` without
// `pub`) is rejected here rather than silently callable by importers.
// Blocks inside a module file publish pub members into the file
// module's SubModules projections, which populate their own
// PubGenericFns — so `math.physics.blockid:i32(42)` resolves through
// the projection's decl (whose Key carries the full canonical
// `math.physics.blockid`).
func (s *Sema) lookupModuleGenericFn(mod *TinocModule, name string) *GenericFnDecl {
	if s.state == nil || mod == nil {
		return nil
	}
	return mod.PubGenericFns[name]
}

// === Generic Registration ===

func (cs *CompileState) registerGenericFn(s *Sema, fn *FunctionStatement, canonical string) {
	if _, dup := cs.GenericFns[canonical]; dup {
		s.errorAt(fn.Token.Line, fn.Token.Column, "%s redeclared in this block", canonical)
		return
	}
	decl := &GenericFnDecl{
		Key:    canonical,
		Prefix: append([]string{}, s.itemPrefix()...),
		Short:  fn.Name.Value,
		Params: fn.GenericParams,
		Fn:     fn,
		Sema:   s,
	}
	cs.GenericFns[canonical] = decl
	// Publish the template into the module's export surface so
	// importers can bind it by name (`#import math.identity;`) or via
	// wildcard/selected imports, and track private generics so
	// importing one reports "is private to module" precisely.
	if s.selfView != nil {
		if fn.IsPub {
			s.selfView.PubGenericFns[fn.Name.Value] = decl
		} else {
			s.selfView.PrivateGenerics[fn.Name.Value] = true
		}
	}
}

func (cs *CompileState) registerGenericStruct(s *Sema, st *StructStatement, canonical string) {
	if _, dup := cs.GenericStructs[canonical]; dup {
		s.errorAt(st.Token.Line, st.Token.Column, "%s redeclared in this block", canonical)
		return
	}
	decl := &GenericStructDecl{
		Key:    canonical,
		Prefix: append([]string{}, s.itemPrefix()...),
		Short:  st.Name.Value,
		Params: st.GenericParams,
		St:     st,
		Sema:   s,
	}
	cs.GenericStructs[canonical] = decl
	if s.selfView != nil {
		if st.IsPub {
			s.selfView.PubGenericStructs[st.Name.Value] = decl
		} else {
			s.selfView.PrivateGenerics[st.Name.Value] = true
		}
	}
}

func (cs *CompileState) registerGenericAlias(s *Sema, tas *TypeAliasStatement, canonical string) {
	if _, dup := cs.GenericAliases[canonical]; dup {
		s.errorAt(tas.Token.Line, tas.Token.Column, "%s redeclared in this block", canonical)
		return
	}
	decl := &GenericAliasDecl{
		Key:    canonical,
		Prefix: append([]string{}, s.itemPrefix()...),
		Short:  tas.Name.Value,
		Params: tas.GenericParams,
		Type:   tas.Type,
	}
	cs.GenericAliases[canonical] = decl
	if s.selfView != nil {
		if tas.IsPub {
			s.selfView.PubGenericAliases[tas.Name.Value] = decl
		} else {
			s.selfView.PrivateGenerics[tas.Name.Value] = true
		}
	}
}

// lookupGenericFn finds a generic function template by bare name in the
// current module scope — a local template or one imported by name
// (`#import math.identity;` / `#import math.*;`).
func (s *Sema) lookupGenericFn(name string) *GenericFnDecl {
	if s.state == nil {
		return nil
	}
	if d, ok := s.state.GenericFns[s.itemCanonical(name)]; ok {
		return d
	}
	if d, ok := s.importedGenericFns[name]; ok {
		return d
	}
	return nil
}

// lookupGenericStruct finds a generic struct template by its dotted
// spelling ("Pair", "math.Pair") or an imported bare name
// (`#import shapes.Circle;`).
func (s *Sema) lookupGenericStruct(base string) *GenericStructDecl {
	if s.state == nil {
		return nil
	}
	if d, ok := s.state.GenericStructs[base]; ok {
		return d
	}
	if d, ok := s.state.GenericStructs[s.itemCanonical(base)]; ok {
		return d
	}
	if d, ok := s.importedGenericStructs[base]; ok {
		return d
	}
	return nil
}

// resolveGenericType handles a `Pair:i32` / `Opt:T` type expression:
// monomorphize the generic struct, or expand the generic alias.
func (s *Sema) resolveGenericType(t *GenericType) *Type {
	base := t.Base
	// Qualified base: `math.Pair:i32` -> find the module's generic. Only
	// pub templates are reachable: the module's registry (selfView)
	// carries exactly the exported generic surface, so a private generic
	// struct/alias is rejected here instead of silently instantiable by
	// importers (lookupModuleGenericFn applies the same rule to fns).
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		modName, short := base[:idx], base[idx+1:]
		mod, ok := s.modules[modName]
		if !ok {
			s.errorAt(t.Token.Line, t.Token.Column, "undefined module %s", modName)
			return nil
		}
		if d := mod.PubGenericStructs[short]; d != nil {
			return s.instantiateGenericStruct(d, t.Args, t.Token)
		}
		if a := mod.PubGenericAliases[short]; a != nil {
			return s.instantiateGenericAlias(a, t.Args, t.Token)
		}
		s.errorAt(t.Token.Line, t.Token.Column, "module %s has no generic type %s", modName, short)
		return nil
	}
	if d := s.lookupGenericStruct(base); d != nil {
		return s.instantiateGenericStruct(d, t.Args, t.Token)
	}
	if a := s.state.GenericAliases[s.itemCanonical(base)]; a != nil {
		return s.instantiateGenericAlias(a, t.Args, t.Token)
	}
	if a := s.importedGenericAliases[base]; a != nil {
		return s.instantiateGenericAlias(a, t.Args, t.Token)
	}
	s.errorAt(t.Token.Line, t.Token.Column, "undefined generic type %s", base)
	return nil
}

// === Generic Instantiation (monomorphization) ===

// resolveGenericArgs resolves the type arguments of an instantiation in
// the caller's scope, validating arity.
func (s *Sema) resolveGenericArgs(params []string, args []TypeExpr, what string, tok Token) []*Type {
	if len(args) != len(params) {
		s.errorAt(tok.Line, tok.Column, "%s expects %d type argument(s), got %d", what, len(params), len(args))
		return nil
	}
	argTypes := make([]*Type, len(args))
	for i, a := range args {
		t := s.resolveTypeExpr(a)
		if t == nil {
			return nil
		}
		if t.Kind == KindUnknown || t.Kind == KindInvalid {
			s.errorAt(tok.Line, tok.Column, "cannot use %s as a type argument for %s", a.String(), what)
			return nil
		}
		argTypes[i] = t
	}
	return argTypes
}

// instantiateGenericStruct creates (or fetches) the concrete struct
// type for a generic struct + type args, registering it in this Sema's
// tables so fields, methods, and calls resolve normally.
func (s *Sema) instantiateGenericStruct(decl *GenericStructDecl, args []TypeExpr, tok Token) *Type {
	argTypes := s.resolveGenericArgs(decl.Params, args, decl.Short, tok)
	if argTypes == nil {
		return nil
	}
	key := genericInstanceKey(decl.Key, argTypes)
	if inst, ok := s.state.StructInstances[key]; ok {
		s.registerInstanceLocally(inst)
		return inst.Type
	}

	display := decl.Short + ":" + argsDisplay(argTypes)
	canonical := display
	if len(decl.Prefix) > 0 {
		canonical = strings.Join(append(append([]string{}, decl.Prefix...), display), ".")
	}
	cname := genericTypeCName(decl.Prefix, decl.Short, argTypes)

	// Build the substitution environment and clone the declaration. The
	// struct's own short name also substitutes to this instance's
	// canonical name, so `self ^Pair` (and any other reference to the
	// generic struct's own type inside its methods or fields) resolves
	// to the concrete instance type, which is registered below.
	env := make(map[string]TypeExpr, len(decl.Params)+1)
	for i, p := range decl.Params {
		env[p] = args[i]
	}
	env[decl.Short] = &NamedType{Name: canonical}
	clone := cloneStructStatement(decl.St)
	clone.GenericParams = nil
	for _, f := range clone.Fields {
		if f != nil && f.Type != nil {
			f.Type = substituteTypeExpr(f.Type, env)
		}
	}
	for _, m := range clone.Methods {
		substituteFunctionTypes(m, env)
		substituteBodyTypes(m.Body, env)
	}

	t := &Type{Kind: KindStruct, Name: canonical, CName: cname, FieldIndex: make(map[string]int)}
	methods := make(map[string]*Symbol)
	inst := &StructInstance{Type: t, Methods: methods, Decl: clone, Sema: decl.Sema}
	// Cache the instance BEFORE its method signatures resolve. A method
	// that references the struct's own generic type (`fn swap(self
	// ^Pair:T)`) re-enters instantiateGenericStruct with the same key
	// while `self ^Pair:(i32)` resolves; without the early entry it
	// would find no cache hit and recurse forever (clone -> register
	// method -> resolve signature -> re-instantiate).
	s.state.StructInstances[key] = inst
	// Register the instance's concrete type before its methods resolve,
	// so a method's `self` parameter and every body referencing the
	// concrete type resolves during signature registration.
	s.structTypes[canonical] = t
	s.structMethods[canonical] = methods
	s.resolveStructFields(t, clone, canonical)

	wrapper := map[string]map[string]*Symbol{canonical: methods}
	for _, m := range clone.Methods {
		if m != nil && m.Name != nil {
			s.registerTypeMethod(canonical, wrapper, m, KindStruct)
		}
	}

	s.state.InstantiatedStructs = append(s.state.InstantiatedStructs, inst)
	s.registerInstanceLocally(inst)
	// Mirror the instance into the defining module's analyzer too:
	// its method bodies are checked against decl.Sema (so
	// module-local names resolve) and codegen emits the definition
	// through inst.Sema, so both must see the concrete type and
	// method table.
	if decl.Sema != nil && decl.Sema != s {
		decl.Sema.registerInstanceLocally(inst)
		s.mirrorCallerTypes(decl.Sema)
	}

	for _, m := range clone.Methods {
		if m != nil && m.Name != nil {
			s.pendingChecks = append(s.pendingChecks, pendingCheck{kind: checkStructMethod, canonical: canonical, fn: m, sema: decl.Sema})
		}
	}
	return t
}

// registerInstanceLocally mirrors a (possibly already-created) struct
// instance into this Sema's registries so every code path that keys on
// the canonical name — field access, method tables, struct literals —
// resolves it.
func (s *Sema) registerInstanceLocally(inst *StructInstance) {
	s.structTypes[inst.Type.Name] = inst.Type
	s.structMethods[inst.Type.Name] = inst.Methods
}

// mirrorCallerTypes copies the instantiating analyzer's type
// registries into the defining module's analyzer so a generic
// body checked cross-module can still resolve concrete type
// arguments that come from the caller's scope (e.g. a root-file
// struct passed to a module's generic function). Only missing
// keys are copied: the defining module's own names always win.
func (s *Sema) mirrorCallerTypes(dst *Sema) {
	if dst == nil || dst == s {
		return
	}
	for k, v := range s.structTypes {
		if _, exists := dst.structTypes[k]; !exists {
			dst.structTypes[k] = v
		}
	}
	for k, v := range s.structMethods {
		if _, exists := dst.structMethods[k]; !exists {
			dst.structMethods[k] = v
		}
	}
	for k, v := range s.enumTypes {
		if _, exists := dst.enumTypes[k]; !exists {
			dst.enumTypes[k] = v
		}
	}
	for k, v := range s.enumMethods {
		if _, exists := dst.enumMethods[k]; !exists {
			dst.enumMethods[k] = v
		}
	}
	for k, v := range s.unionTypes {
		if _, exists := dst.unionTypes[k]; !exists {
			dst.unionTypes[k] = v
		}
	}
	for k, v := range s.unionMethods {
		if _, exists := dst.unionMethods[k]; !exists {
			dst.unionMethods[k] = v
		}
	}
}

// instantiateGenericAlias expands a generic alias: substitute the type
// args into the aliased type expression and resolve it.
func (s *Sema) instantiateGenericAlias(decl *GenericAliasDecl, args []TypeExpr, tok Token) *Type {
	argTypes := s.resolveGenericArgs(decl.Params, args, decl.Short, tok)
	if argTypes == nil {
		return nil
	}
	env := make(map[string]TypeExpr, len(decl.Params))
	for i, p := range decl.Params {
		env[p] = args[i]
	}
	return s.resolveTypeExpr(substituteTypeExpr(decl.Type, env))
}

// tryGenericCall handles a call to a generic function: explicit type
// args (`identity:str(x)`) or inference from the argument types
// (`identity(x)`). Returns (sym, true) when the callee is generic and
// the call was handled (sym may be a best-effort symbol when the
// instantiation failed, so callers don't cascade extra errors).
func (s *Sema) tryGenericCall(ce *CallExpression, ident *Identifier) (*Symbol, bool) {
	decl := s.lookupGenericFn(ident.Value)
	if decl == nil {
		return nil, false
	}
	line, col := ce.Token.Line, ce.Token.Column

	var argTypes []*Type
	if len(ce.GenericArgs) > 0 {
		argTypes = s.resolveGenericArgs(decl.Params, ce.GenericArgs, ident.Value, ce.Token)
		if argTypes == nil {
			s.checkArgsOnly(ce)
			return &Symbol{Name: ident.Value, ReturnType: typeVoid}, true
		}
	} else {
		argTypes = s.inferGenericArgs(decl, ce)
		if argTypes == nil {
			s.checkArgsOnly(ce)
			return &Symbol{Name: ident.Value, ReturnType: typeVoid}, true
		}
	}

	sym := s.instantiateGenericFn(decl, argTypes)
	if sym == nil {
		s.checkArgsOnly(ce)
		return &Symbol{Name: ident.Value, ReturnType: typeVoid}, true
	}
	_ = line
	_ = col
	s.callTargets[ce] = sym
	for _, a := range ce.Arguments {
		s.checkExpression(a)
	}
	s.checkCallArgs(ce, sym, ident.Value, 0)
	return sym, true
}

// checkArgsOnly type-checks call arguments without a callee signature
// (used after instantiation failures so the body still gets checked).
func (s *Sema) checkArgsOnly(ce *CallExpression) {
	for _, a := range ce.Arguments {
		s.checkExpression(a)
	}
}

// inferGenericArgs derives type arguments from call-site argument types
// for `identity(x)`. Only simple `param TypeParam` parameters
// participate; anything else requires explicit type arguments.
func (s *Sema) inferGenericArgs(decl *GenericFnDecl, ce *CallExpression) []*Type {
	params := decl.Fn.Params
	if len(params) != len(ce.Arguments) {
		missing := make([]string, 0, len(decl.Params))
		for _, p := range decl.Params {
			missing = append(missing, p)
		}
		s.errorAt(ce.Token.Line, ce.Token.Column, "cannot infer type parameter(s) %s — provide explicit type arguments (e.g. %s:...)", strings.Join(missing, ", "), decl.Short)
		return nil
	}
	env := make(map[string]*Type, len(decl.Params))
	for i, p := range params {
		if p == nil || p.Type == nil {
			continue
		}
		nt, ok := p.Type.(*NamedType)
		if !ok || !decl.hasParam(nt.Name) {
			continue
		}
		at := s.checkExpression(ce.Arguments[i])
		if at == nil || at.Kind == KindInvalid || at.Kind == KindUnknown {
			s.errorAt(ce.Token.Line, ce.Token.Column, "cannot infer type parameter %s from argument %d", nt.Name, i+1)
			return nil
		}
		if prev, ok := env[nt.Name]; ok {
			if !typesEqual(prev, at) {
				s.errorAt(ce.Token.Line, ce.Token.Column, "inconsistent type arguments for %s: got %s and %s", nt.Name, prev.String(), at.String())
				return nil
			}
			continue
		}
		env[nt.Name] = at
	}
	if len(env) != len(decl.Params) {
		missing := make([]string, 0, len(decl.Params))
		for _, p := range decl.Params {
			if env[p] == nil {
				missing = append(missing, p)
			}
		}
		s.errorAt(ce.Token.Line, ce.Token.Column, "cannot infer type parameter(s) %s — provide explicit type arguments (e.g. %s:...)", strings.Join(missing, ", "), decl.Short)
		return nil
	}
	argTypes := make([]*Type, 0, len(decl.Params))
	for _, p := range decl.Params {
		argTypes = append(argTypes, env[p])
	}
	return argTypes
}

// instantiateGenericFn clones a generic function with concrete type
// arguments, registers it under its mangled name, and queues its body
// for checking. Cached per (decl, args).
func (s *Sema) instantiateGenericFn(decl *GenericFnDecl, argTypes []*Type) *Symbol {
	key := genericInstanceKey(decl.Key, argTypes)
	display := decl.Short + ":" + argsDisplay(argTypes)

	if inst, ok := s.state.FnInstances[key]; ok {
		if sym := s.funcs[inst.Name.Value]; sym != nil {
			return sym
		}
		return s.registerFnInstanceLocally(inst)
	}

	env := make(map[string]TypeExpr, len(decl.Params))
	for i, p := range decl.Params {
		env[p] = typeExprFromType(argTypes[i])
	}
	clone := cloneFunctionStatement(decl.Fn)
	clone.GenericParams = nil
	clone.Name = &Identifier{Value: display}
	substituteFunctionTypes(clone, env)
	substituteBodyTypes(clone.Body, env)

	sym := s.resolveFnSignature(clone, display, clone.Token.Line, clone.Token.Column)
	if sym == nil {
		return nil
	}
	sym.CName = genericFnCName(decl.Prefix, decl.Short, argTypes)
	s.funcs[display] = sym
	// The body is checked against the defining module's analyzer so
	// module-local names resolve; mirror the caller's type
	// registries (the type arguments may be caller-local types) and
	// register the instance symbol there for codegen.
	if decl.Sema != nil && decl.Sema != s {
		s.mirrorCallerTypes(decl.Sema)
		decl.Sema.funcs[display] = sym
	}

	s.state.FnInstances[key] = clone
	s.state.InstantiatedFns = append(s.state.InstantiatedFns, &FnInstance{Fn: clone, Sema: decl.Sema})
	s.pendingChecks = append(s.pendingChecks, pendingCheck{kind: checkFn, fn: clone, label: display, sema: decl.Sema})
	return sym
}

// registerFnInstanceLocally registers a function instance created by
// another file into this Sema's function table (its CName is already
// mangled).
func (s *Sema) registerFnInstanceLocally(fn *FunctionStatement) *Symbol {
	if sym := s.funcs[fn.Name.Value]; sym != nil {
		return sym
	}
	sym := s.resolveFnSignature(fn, fn.Name.Value, fn.Token.Line, fn.Token.Column)
	if sym == nil {
		return nil
	}
	s.funcs[fn.Name.Value] = sym
	return sym
}

// === Pending Body Checks ===

type pendingKind int

const (
	checkFn pendingKind = iota
	checkStructMethod
)

type pendingCheck struct {
	kind      pendingKind
	canonical string
	fn        *FunctionStatement
	label     string
	// sema is the analyzer the body must be checked against: the
	// defining module's analyzer for cross-module instances (nil
	// means the draining analyzer itself).
	sema *Sema
}

// drainPendingChecks checks every monomorphized function/method body
// queued during registration and earlier checks, until no new
// instantiations remain (instantiated bodies can trigger further
// instantiations).
func (s *Sema) drainPendingChecks() {
	for len(s.pendingChecks) > 0 {
		pc := s.pendingChecks[0]
		s.pendingChecks = s.pendingChecks[1:]
		if pc.fn == nil || pc.fn.Name == nil || pc.fn.Body == nil {
			continue
		}
		// Cross-module generic instances are checked against the
		// defining module's analyzer (pc.sema), where the body's
		// module-local names resolve; instances of the analyzer's own
		// generics use itself.
		checker := pc.sema
		if checker == nil {
			checker = s
		}
		switch pc.kind {
		case checkFn:
			if sym := checker.funcs[pc.fn.Name.Value]; sym != nil {
				checker.checkFunctionBody(pc.label, sym, pc.fn.Params, pc.fn.Body, pc.fn.Token, false)
			}
		case checkStructMethod:
			methods := checker.structMethods[pc.canonical]
			if methods == nil {
				continue
			}
			if sym := methods[pc.fn.Name.Value]; sym != nil {
				checker.checkFunctionBody(pc.canonical+"."+pc.fn.Name.Value, sym, pc.fn.Params, pc.fn.Body, pc.fn.Token, true)
			}
		}
		// A body checked against another analyzer can instantiate
		// further generics, which queue on that analyzer; fold them
		// back into this queue so they drain here too.
		if checker != s && len(checker.pendingChecks) > 0 {
			s.pendingChecks = append(s.pendingChecks, checker.pendingChecks...)
			checker.pendingChecks = nil
		}
	}
}

// === Module Block Views ===

// bindBlockView publishes an in-file `module name { ... }` namespace so
// `name.member` resolves within the file. Block views expose every
// member (pub and private — they are the same file's namespace), unlike
// imported modules which only expose pub items.
func (s *Sema) bindBlockView(mbs *ModuleBlockStatement, full []string) {
	if s.state == nil {
		return
	}
	// full is the block's full dotted name: the parser records only the
	// block's own declared name (`module a { module b { ... } }` parses
	// block b with Name ["b"]), and pass 3.5 threads the enclosing names
	// so nested blocks register as `a.b`. The last segment is the bare
	// namespace name ("b"), which also binds for direct access.
	segName := full[len(full)-1]
	if _, dup := s.modules[segName]; dup {
		s.errorAt(mbs.Token.Line, mbs.Token.Column, "%s already bound in this file (module namespace)", segName)
		return
	}
	view := newTinocModule(strings.Join(full, "."), s.sourceDir)
	view.IsBlock = true
	for _, stmt := range mbs.Statements {
		switch inner := stmt.(type) {
		case *FunctionStatement:
			if inner.Name != nil {
				if sym := s.funcs[inner.Name.Value]; sym != nil {
					view.PubFuncs[inner.Name.Value] = sym
					view.Funcs[inner.Name.Value] = sym
				}
				// Generic functions register as templates, not symbols;
				// expose the template so qualified calls
				// (`physics.identity:i32(42)`) resolve in-file.
				if len(inner.GenericParams) > 0 && s.state != nil {
					if d := s.state.GenericFns[blockItemCanonical(s, full, inner.Name.Value)]; d != nil {
						view.PubGenericFns[inner.Name.Value] = d
					}
				}
			}
		case *StructStatement:
			if inner.Name != nil {
				if t := s.structTypes[s.canonNames[inner]]; t != nil {
					view.PubTypes[inner.Name.Value] = t
				}
				if len(inner.GenericParams) > 0 && s.state != nil {
					if d := s.state.GenericStructs[blockItemCanonical(s, full, inner.Name.Value)]; d != nil {
						view.PubGenericStructs[inner.Name.Value] = d
					}
				}
			}
		case *TypeAliasStatement:
			if inner.Name != nil {
				if t := s.typeAliases[inner.Name.Value]; t != nil {
					view.PubTypes[inner.Name.Value] = t
				}
				if len(inner.GenericParams) > 0 && s.state != nil {
					if d := s.state.GenericAliases[blockItemCanonical(s, full, inner.Name.Value)]; d != nil {
						view.PubGenericAliases[inner.Name.Value] = d
					}
				}
			}
		case *EnumStatement:
			if inner.Name != nil {
				if t := s.enumTypes[s.canonNames[inner]]; t != nil {
					view.PubTypes[inner.Name.Value] = t
				}
			}
		case *UnionStatement:
			if inner.Name != nil {
				if t := s.unionTypes[s.canonNames[inner]]; t != nil {
					view.PubTypes[inner.Name.Value] = t
				}
			}
		case *ConstStatement:
			if inner.Name != nil {
				if t := s.declConstTypes[inner]; t != nil {
					view.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, full, inner.Name.Value), Mutable: false, IsStatic: inner.IsStatic}
					view.PubTypes[inner.Name.Value] = t
				} else if inner.Type != nil {
					// Pass 3.5 runs before top-level consts are checked, so
					// resolve their types eagerly to publish the view.
					if t := s.resolveTypeExpr(inner.Type); t != nil {
						view.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, full, inner.Name.Value), Mutable: false, IsStatic: inner.IsStatic}
						view.PubTypes[inner.Name.Value] = t
					}
				} else if inner.Value != nil {
					if t := s.checkExpression(inner.Value); t != nil && t.Kind != KindInvalid {
						view.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, full, inner.Name.Value), Mutable: false, IsStatic: inner.IsStatic}
						view.PubTypes[inner.Name.Value] = t
					}
				}
			}
		case *VarStatement:
			if inner.Name != nil {
				if t := s.declVarTypes[inner]; t != nil {
					view.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, full, inner.Name.Value), Mutable: true, IsStatic: inner.IsStatic}
					view.PubTypes[inner.Name.Value] = t
				} else if inner.Type != nil {
					if t := s.resolveTypeExpr(inner.Type); t != nil {
						view.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, full, inner.Name.Value), Mutable: true, IsStatic: inner.IsStatic}
						view.PubTypes[inner.Name.Value] = t
					}
				} else if inner.Value != nil {
					if t := s.checkExpression(inner.Value); t != nil && t.Kind != KindInvalid {
						view.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, full, inner.Name.Value), Mutable: true, IsStatic: inner.IsStatic}
						view.PubTypes[inner.Name.Value] = t
					}
				}
			}
		}
	}
	s.modules[segName] = view
	// Nested blocks (`module a { module b { ... } }`) and dotted block
	// names (`module a.b { ... }`) also register under every dotted
	// prefix of the full name — including the full key itself — so
	// qualified chains (`a.b.VAL`, `a.b.c.VAL`) resolve through
	// tinocModule's dotted lookups. A key that is already bound keeps its
	// owner (a directly-declared block owns its own segment); the last
	// segment was checked and registered above.
	for i := 1; i <= len(full); i++ {
		key := strings.Join(full[:i], ".")
		if _, dup := s.modules[key]; !dup {
			s.modules[key] = view
		}
	}

	// Publish a pub-only projection of the block into the file module's
	// namespace so importers can chain through it (`module math;` file
	// containing `module physics { pub const g; }` -> `math.physics.g`).
	// Blocks expose every member in-file (same-file namespace), but
	// across the module boundary only `pub` members are visible, so the
	// projection filters IsPub. Nested blocks wire parent -> child so
	// chains (`math.a.b.VAL`) resolve through each projection's own
	// SubModules.
	if s.selfView != nil {
		proj := s.blockPubProjection(view, mbs)
		s.selfView.SubModules[strings.Join(full, ".")] = proj
		if len(full) > 1 {
			parent := s.selfView.SubModules[strings.Join(full[:len(full)-1], ".")]
			if parent != nil {
				parent.SubModules[full[len(full)-1]] = proj
			}
		}
	}
}

// blockPubProjection builds the pub-only view of a block for
// cross-module access: every member marked `pub` (functions, types,
// consts/vars) is exported; private block members stay file-private.
func (s *Sema) blockPubProjection(view *TinocModule, mbs *ModuleBlockStatement) *TinocModule {
	proj := newTinocModule(view.Name, view.Path)
	proj.IsBlock = true
	// view.Name is the block's full dotted name ("physics" or "a.b"),
	// which blockMemberCName needs as its segment list.
	segments := strings.Split(view.Name, ".")
	for _, stmt := range mbs.Statements {
		switch inner := stmt.(type) {
		case *FunctionStatement:
			if inner.Name != nil && inner.IsPub {
				if sym := s.funcs[inner.Name.Value]; sym != nil {
					proj.PubFuncs[inner.Name.Value] = sym
					proj.Funcs[inner.Name.Value] = sym
				}
			}
			// Generic functions register as templates, not symbols;
			// publish the template so importers can call them
			// qualified through the block (`math.physics.identity:i32(42)`).
			if inner.Name != nil && inner.IsPub && len(inner.GenericParams) > 0 && s.state != nil {
				if d := s.state.GenericFns[blockItemCanonical(s, segments, inner.Name.Value)]; d != nil {
					proj.PubGenericFns[inner.Name.Value] = d
				}
			}
		case *StructStatement:
			if inner.Name != nil && inner.IsPub {
				if t := s.structTypes[s.canonNames[inner]]; t != nil {
					proj.PubTypes[inner.Name.Value] = t
				}
			}
			if inner.Name != nil && inner.IsPub && len(inner.GenericParams) > 0 && s.state != nil {
				if d := s.state.GenericStructs[blockItemCanonical(s, segments, inner.Name.Value)]; d != nil {
					proj.PubGenericStructs[inner.Name.Value] = d
				}
			}
		case *EnumStatement:
			if inner.Name != nil && inner.IsPub {
				if t := s.enumTypes[s.canonNames[inner]]; t != nil {
					proj.PubTypes[inner.Name.Value] = t
				}
			}
		case *UnionStatement:
			if inner.Name != nil && inner.IsPub {
				if t := s.unionTypes[s.canonNames[inner]]; t != nil {
					proj.PubTypes[inner.Name.Value] = t
				}
			}
		case *ConstStatement:
			if inner.Name != nil && inner.IsPub {
				if t := s.declConstTypes[inner]; t != nil {
					proj.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, segments, inner.Name.Value), Mutable: false, IsStatic: inner.IsStatic}
					proj.PubTypes[inner.Name.Value] = t
				} else if inner.Type != nil {
					if t := s.resolveTypeExpr(inner.Type); t != nil {
						proj.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, segments, inner.Name.Value), Mutable: false, IsStatic: inner.IsStatic}
						proj.PubTypes[inner.Name.Value] = t
					}
				} else if inner.Value != nil {
					if t := s.checkExpression(inner.Value); t != nil && t.Kind != KindInvalid {
						proj.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, segments, inner.Name.Value), Mutable: false, IsStatic: inner.IsStatic}
						proj.PubTypes[inner.Name.Value] = t
					}
				}
			}
		case *VarStatement:
			if inner.Name != nil && inner.IsPub {
				if t := s.declVarTypes[inner]; t != nil {
					proj.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, segments, inner.Name.Value), Mutable: true, IsStatic: inner.IsStatic}
					proj.PubTypes[inner.Name.Value] = t
				} else if inner.Type != nil {
					if t := s.resolveTypeExpr(inner.Type); t != nil {
						proj.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, segments, inner.Name.Value), Mutable: true, IsStatic: inner.IsStatic}
						proj.PubTypes[inner.Name.Value] = t
					}
				} else if inner.Value != nil {
					if t := s.checkExpression(inner.Value); t != nil && t.Kind != KindInvalid {
						proj.PubConsts[inner.Name.Value] = &constExport{Name: inner.Name.Value, Type: t, CName: blockMemberCName(s, segments, inner.Name.Value), Mutable: true, IsStatic: inner.IsStatic}
						proj.PubTypes[inner.Name.Value] = t
					}
				}
			}
		case *TypeAliasStatement:
			if inner.Name != nil && inner.IsPub {
				if t := s.typeAliases[inner.Name.Value]; t != nil {
					proj.PubTypes[inner.Name.Value] = t
				}
			}
			if inner.Name != nil && inner.IsPub && len(inner.GenericParams) > 0 && s.state != nil {
				if d := s.state.GenericAliases[blockItemCanonical(s, segments, inner.Name.Value)]; d != nil {
					proj.PubGenericAliases[inner.Name.Value] = d
				}
			}
		}
	}
	return proj
}

// blockItemCanonical returns the canonical dotted name a generic
// declaration inside an in-file module block registered under
// (`module math; module physics { pub fn id:T ... }` ->
// `math.physics.id`): the file module prefix (skipped for the
// entry file) plus the block's full dotted name plus the
// member name.
func blockItemCanonical(s *Sema, full []string, name string) string {
	segs := append(s.itemPrefix(), full...)
	return strings.Join(append(segs, name), ".")
}

// blockMemberCName computes the mangled C name a module-block member
// gets when it is emitted (`module physics { const g ... }` ->
// `physics_g`; nested `module a { module b { pub const VAL } }` ->
// `a_b_VAL`), matching the emit-time itemCName: the file's module
// prefix (skipped for the entry file) plus the block's full dotted name
// segments plus the member name.
func blockMemberCName(s *Sema, full []string, name string) string {
	segs := append(s.itemPrefix(), full...)
	return moduleSegsCName(segs, name)
}

// bindSelfExports publishes this file's pub items into its own module
// registry (selfView) as they are registered, so the module's export
// surface is complete by the time its Check finishes. Runs incrementally
// from the registration hooks.
func (s *Sema) publishExport(name string, kind string, sym *Symbol, t *Type, ce *constExport) {
	if s.selfView == nil {
		return
	}
	switch kind {
	case "fn":
		s.selfView.PubFuncs[name] = sym
		s.selfView.Funcs[name] = sym
	case "const":
		s.selfView.PubConsts[name] = ce
		s.selfView.PubTypes[name] = ce.Type
	case "type":
		s.selfView.PubTypes[name] = t
	}
}

// === AST Cloning & Type Substitution ===
//
// Monomorphization clones the generic declaration's AST and substitutes
// its type parameters with the concrete argument types, so each
// instance is checked and emitted from fresh nodes (shared nodes would
// overwrite resolved-type side tables between instances).

// cloneTypeExpr deep-copies a TypeExpr.
func cloneTypeExpr(te TypeExpr) TypeExpr {
	if te == nil {
		return nil
	}
	switch t := te.(type) {
	case *NamedType:
		c := *t
		return &c
	case *PointerType:
		return &PointerType{Token: t.Token, Elem: cloneTypeExpr(t.Elem)}
	case *CQualType:
		return &CQualType{Token: t.Token, Qual: t.Qual, Elem: cloneTypeExpr(t.Elem)}
	case *OptionalType:
		return &OptionalType{Token: t.Token, Elem: cloneTypeExpr(t.Elem)}
	case *ErrorUnionType:
		return &ErrorUnionType{Token: t.Token, ErrSet: cloneTypeExpr(t.ErrSet), Elem: cloneTypeExpr(t.Elem)}
	case *ArrayType:
		return &ArrayType{Token: t.Token, Size: cloneExpr(t.Size), Inferred: t.Inferred, Sentinel: cloneExpr(t.Sentinel), Elem: cloneTypeExpr(t.Elem)}
	case *GenericType:
		args := make([]TypeExpr, 0, len(t.Args))
		for _, a := range t.Args {
			args = append(args, cloneTypeExpr(a))
		}
		return &GenericType{Token: t.Token, Base: t.Base, Args: args}
	default:
		return te
	}
}

// substituteTypeExpr replaces every reference to a generic type
// parameter (a bare NamedType whose name is a key of env) with the
// cloned argument type expression.
func substituteTypeExpr(te TypeExpr, env map[string]TypeExpr) TypeExpr {
	if te == nil {
		return nil
	}
	switch t := te.(type) {
	case *NamedType:
		if repl, ok := env[t.Name]; ok {
			return cloneTypeExpr(repl)
		}
		c := *t
		return &c
	case *PointerType:
		return &PointerType{Token: t.Token, Elem: substituteTypeExpr(t.Elem, env)}
	case *CQualType:
		return &CQualType{Token: t.Token, Qual: t.Qual, Elem: substituteTypeExpr(t.Elem, env)}
	case *OptionalType:
		return &OptionalType{Token: t.Token, Elem: substituteTypeExpr(t.Elem, env)}
	case *ErrorUnionType:
		return &ErrorUnionType{Token: t.Token, ErrSet: substituteTypeExpr(t.ErrSet, env), Elem: substituteTypeExpr(t.Elem, env)}
	case *ArrayType:
		return &ArrayType{Token: t.Token, Size: cloneExpr(t.Size), Inferred: t.Inferred, Sentinel: cloneExpr(t.Sentinel), Elem: substituteTypeExpr(t.Elem, env)}
	case *GenericType:
		args := make([]TypeExpr, 0, len(t.Args))
		for _, a := range t.Args {
			args = append(args, substituteTypeExpr(a, env))
		}
		return &GenericType{Token: t.Token, Base: t.Base, Args: args}
	default:
		return cloneTypeExpr(te)
	}
}

// substituteFunctionTypes applies env to a function's parameter and
// return type expressions (its body is cloned wholesale separately).
func substituteFunctionTypes(fn *FunctionStatement, env map[string]TypeExpr) {
	if fn == nil {
		return
	}
	for _, p := range fn.Params {
		if p != nil && p.Type != nil {
			p.Type = substituteTypeExpr(p.Type, env)
		}
	}
	if fn.ReturnType != nil {
		fn.ReturnType = substituteTypeExpr(fn.ReturnType, env)
	}
}

// cloneFunctionStatement deep-copies a function declaration including
// its body.
func cloneFunctionStatement(fn *FunctionStatement) *FunctionStatement {
	if fn == nil {
		return nil
	}
	clone := &FunctionStatement{
		Token:         fn.Token,
		GenericParams: append([]string{}, fn.GenericParams...),
		Variadic:      fn.Variadic,
		IsPub:         fn.IsPub,
		IsStatic:      fn.IsStatic,
	}
	if fn.Name != nil {
		clone.Name = &Identifier{Token: fn.Name.Token, Value: fn.Name.Value}
	}
	for _, p := range fn.Params {
		if p == nil {
			continue
		}
		cp := &Parameter{}
		if p.Name != nil {
			cp.Name = &Identifier{Token: p.Name.Token, Value: p.Name.Value}
		}
		cp.Type = cloneTypeExpr(p.Type)
		clone.Params = append(clone.Params, cp)
	}
	clone.ReturnType = cloneTypeExpr(fn.ReturnType)
	clone.Body = cloneBlock(fn.Body)
	return clone
}

// cloneStructStatement deep-copies a struct declaration's name, fields,
// and methods (method bodies included).
func cloneStructStatement(st *StructStatement) *StructStatement {
	if st == nil {
		return nil
	}
	clone := &StructStatement{
		Token:         st.Token,
		GenericParams: append([]string{}, st.GenericParams...),
		IsPub:         st.IsPub,
	}
	if st.Name != nil {
		clone.Name = &Identifier{Token: st.Name.Token, Value: st.Name.Value}
	}
	for _, f := range st.Fields {
		if f == nil {
			continue
		}
		cf := &StructField{}
		if f.Name != nil {
			cf.Name = &Identifier{Token: f.Name.Token, Value: f.Name.Value}
		}
		cf.Type = cloneTypeExpr(f.Type)
		clone.Fields = append(clone.Fields, cf)
	}
	for _, m := range st.Methods {
		if m != nil {
			clone.Methods = append(clone.Methods, cloneFunctionStatement(m))
		}
	}
	return clone
}

// cloneBlock deep-copies a statement block.
func cloneBlock(b *BlockStatement) *BlockStatement {
	if b == nil {
		return nil
	}
	clone := &BlockStatement{Token: b.Token}
	for _, s := range b.Statements {
		if cs := cloneStatement(s); cs != nil {
			clone.Statements = append(clone.Statements, cs)
		}
	}
	return clone
}

// cloneStatement deep-copies a statement (the subset reachable inside
// function bodies).
func cloneStatement(st Statement) Statement {
	if st == nil {
		return nil
	}
	switch s := st.(type) {
	case *VarStatement:
		c := &VarStatement{Token: s.Token, IsStatic: s.IsStatic, IsPub: s.IsPub}
		if s.Name != nil {
			c.Name = &Identifier{Token: s.Name.Token, Value: s.Name.Value}
		}
		c.Type = cloneTypeExpr(s.Type)
		c.Value = cloneExpr(s.Value)
		return c
	case *ConstStatement:
		c := &ConstStatement{Token: s.Token, IsStatic: s.IsStatic, IsPub: s.IsPub}
		if s.Name != nil {
			c.Name = &Identifier{Token: s.Name.Token, Value: s.Name.Value}
		}
		c.Type = cloneTypeExpr(s.Type)
		c.Value = cloneExpr(s.Value)
		return c
	case *ReturnStatement:
		return &ReturnStatement{Token: s.Token, ReturnValue: cloneExpr(s.ReturnValue)}
	case *BreakStatement:
		return &BreakStatement{Token: s.Token}
	case *ContinueStatement:
		return &ContinueStatement{Token: s.Token}
	case *ExpressionStatement:
		return &ExpressionStatement{Token: s.Token, Expression: cloneExpr(s.Expression)}
	case *BlockStatement:
		return cloneBlock(s)
	case *FunctionStatement:
		return cloneFunctionStatement(s)
	case *IfStatement:
		return &IfStatement{Token: s.Token, Condition: cloneExpr(s.Condition), Consequence: cloneBlock(s.Consequence), Alternative: cloneStatement(s.Alternative)}
	case *WhileStatement:
		return &WhileStatement{Token: s.Token, Condition: cloneExpr(s.Condition), Body: cloneBlock(s.Body)}
	case *ForStatement:
		return &ForStatement{Token: s.Token, Start: cloneExpr(s.Start), End: cloneExpr(s.End), Collection: cloneExpr(s.Collection), Capture: cloneIdent(s.Capture), Body: cloneBlock(s.Body)}
	case *SwitchStatement:
		c := &SwitchStatement{Token: s.Token, Value: cloneExpr(s.Value)}
		for _, arm := range s.Arms {
			if arm == nil {
				continue
			}
			c.Arms = append(c.Arms, &SwitchArm{Value: cloneExpr(arm.Value), Body: cloneBlock(arm.Body)})
		}
		return c
	default:
		// Declarations that cannot appear inside function bodies are
		// cloned shallowly (generic bodies never contain them).
		return st
	}
}

func cloneIdent(id *Identifier) *Identifier {
	if id == nil {
		return nil
	}
	return &Identifier{Token: id.Token, Value: id.Value}
}

// cloneExpr deep-copies an expression.
func cloneExpr(e Expression) Expression {
	if e == nil {
		return nil
	}
	switch ex := e.(type) {
	case *Identifier:
		return cloneIdent(ex)
	case *IntegerLiteral:
		c := *ex
		return &c
	case *FloatLiteral:
		c := *ex
		return &c
	case *StringLiteral:
		c := *ex
		return &c
	case *CharLiteral:
		c := *ex
		return &c
	case *BoolLiteral:
		c := *ex
		return &c
	case *NullLiteral:
		c := *ex
		return &c
	case *ArrayLiteral:
		c := &ArrayLiteral{Token: ex.Token}
		for _, el := range ex.Elements {
			c.Elements = append(c.Elements, cloneExpr(el))
		}
		return c
	case *PrefixExpression:
		return &PrefixExpression{Token: ex.Token, Operator: ex.Operator, Right: cloneExpr(ex.Right)}
	case *PostfixExpression:
		return &PostfixExpression{Token: ex.Token, Operator: ex.Operator, Left: cloneExpr(ex.Left)}
	case *InfixExpression:
		return &InfixExpression{Token: ex.Token, Left: cloneExpr(ex.Left), Operator: ex.Operator, Right: cloneExpr(ex.Right)}
	case *AssignExpression:
		return &AssignExpression{Token: ex.Token, Target: cloneExpr(ex.Target), Operator: ex.Operator, Value: cloneExpr(ex.Value)}
	case *CallExpression:
		c := &CallExpression{Token: ex.Token, Function: cloneExpr(ex.Function)}
		for _, g := range ex.GenericArgs {
			c.GenericArgs = append(c.GenericArgs, cloneTypeExpr(g))
		}
		for _, a := range ex.Arguments {
			c.Arguments = append(c.Arguments, cloneExpr(a))
		}
		return c
	case *IndexExpression:
		return &IndexExpression{Token: ex.Token, Left: cloneExpr(ex.Left), Index: cloneExpr(ex.Index)}
	case *FieldAccessExpression:
		return &FieldAccessExpression{Token: ex.Token, Left: cloneExpr(ex.Left), Field: cloneIdent(ex.Field)}
	case *GenericExpression:
		c := &GenericExpression{Token: ex.Token, Base: cloneExpr(ex.Base)}
		for _, g := range ex.Args {
			c.Args = append(c.Args, cloneTypeExpr(g))
		}
		return c
	case *StructLiteral:
		c := &StructLiteral{Token: ex.Token, Type: cloneTypeExpr(ex.Type)}
		for _, f := range ex.Fields {
			if f == nil {
				continue
			}
			c.Fields = append(c.Fields, &StructLiteralField{Name: cloneIdent(f.Name), Value: cloneExpr(f.Value)})
		}
		return c
	default:
		return e
	}
}
