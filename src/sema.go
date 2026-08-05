package src

import (
	"fmt"
	"path/filepath"
	"strings"
)

// === Semantic Analysis ===
//
// Sema walks the parsed *Program and performs the checks needed to make
// var/const/static var/static const/fn safe to hand to Codegen:
//
//   - name resolution (every identifier reference resolves to a binding
//     that is in scope at that point, or a known function)
//   - type resolution for every explicit type annotation
//   - type inference for `var x = <expr>` / `const x = <expr>` (no
//     explicit type given)
//   - type-checking of initializers against explicit types, of binary/
//     unary operators, of assignments, and of function call arguments
//     against parameter types
//   - const-mutability enforcement (`const` bindings cannot be
//     reassigned; `var`/`static var` can)
//   - function checks: duplicate parameter names, return-type agreement
//     between `return <expr>;` and the function's declared return type,
//     missing return, unknown callee, argument-count/type mismatches
//
// Every AST node Sema resolves is annotated in-place via side tables
// (rather than mutating the AST types) so re-running Sema is idempotent
// and Codegen can look resolved types up by node identity.

// SymbolKind distinguishes what a Symbol denotes.
type SymbolKind int

const (
	SymVar SymbolKind = iota
	SymConst
	SymFunc
	SymParam
)

// Symbol is a single resolved binding: a variable, constant, parameter, or
// function.
type Symbol struct {
	Name     string
	Kind     SymbolKind
	Type     *Type
	IsStatic bool // static var / static const storage
	Mutable  bool // false for const bindings and function params (by value)

	// Function-only fields.
	Params     []*Type
	ParamNames []string
	ReturnType *Type

	// C-interop fields (importc members and extern "C" fn declarations).
	// CSymbol is the real C name the call should use (e.g. "printf"); when
	// empty the symbol is a plain Tinoc function (codegen applies its
	// tnc_ prefix). IsCImport marks extern "C" fn / imported functions so
	// codegen knows to emit the C name directly and unwrap `str` args to
	// their underlying data pointer.
	CSymbol   string
	IsCImport bool
	Variadic  bool
}

// Scope is a single lexical scope: function body, block, or the file's
// top level. Scopes nest via Parent for lookups but each only ever holds
// the bindings introduced directly within it, matching Tinoc's C-like
// block scoping.
type Scope struct {
	Parent *Scope
	names  map[string]*Symbol
}

func newScope(parent *Scope) *Scope {
	return &Scope{Parent: parent, names: make(map[string]*Symbol)}
}

// Define adds a new binding to this scope. Returns false without
// overwriting if the name is already bound *directly* in this scope
// (shadowing an outer scope's binding is allowed, matching block-scoped
// languages generally, including Tinoc's C lineage).
func (s *Scope) Define(sym *Symbol) bool {
	if _, exists := s.names[sym.Name]; exists {
		return false
	}
	s.names[sym.Name] = sym
	return true
}

// Lookup resolves a name against this scope and its ancestors.
func (s *Scope) Lookup(name string) (*Symbol, bool) {
	for sc := s; sc != nil; sc = sc.Parent {
		if sym, ok := sc.names[name]; ok {
			return sym, true
		}
	}
	return nil, false
}

// Sema is the semantic analyzer. It walks a *Program once, resolving
// types and names, and collects diagnostics along the way.
type Sema struct {
	diags   *Diagnostics
	global  *Scope
	funcs   map[string]*Symbol // top-level function table, for forward calls
	current *Scope

	// sourceDir is the directory of the file being checked, used to
	// resolve local headers named by #importc.
	sourceDir string

	// importCModules holds the symbol surface of every #importc, keyed by
	// alias ("cio"); externCFuncs holds every `extern "C" fn` by its
	// Tinoc callable name; cTypes is the global registry of C typedefs /
	// struct / enum tags that resolveTypeExpr falls back to.
	importCModules map[string]*CImportModule
	externCFuncs   map[string]*Symbol
	cTypes         map[string]*Type

	// structTypes holds every declared struct's resolved *Type by name
	// (interned once at registration, so type equality is by identity or
	// name); structMethods holds each struct's method table by method
	// name. Both are populated before any function/struct body is
	// checked, so calls and self-references resolve regardless of
	// textual order.
	structTypes   map[string]*Type
	structMethods map[string]map[string]*Symbol

	// currentFn tracks the function being checked, for `return` type
	// checking. nil at top level.
	currentFn *Symbol

	// resolvedTypes maps expression nodes to their resolved Type, so
	// Codegen can query "what type is this expression" without redoing
	// inference. Keyed by node identity (pointer), since Expression is an
	// interface over pointer-typed AST node structs throughout this
	// codebase.
	resolvedTypes map[Expression]*Type

	// declTypes maps var/const declaration nodes to their resolved Type
	// (covers the decl-only case, e.g. `var x i32;`, which has no
	// initializer expression to key resolvedTypes off of).
	declVarTypes   map[*VarStatement]*Type
	declConstTypes map[*ConstStatement]*Type

	// cStrArgs marks call arguments that must be passed to C as their
	// underlying data pointer rather than as a Tinoc str struct (e.g. a
	// `str` variable handed to printf's variadic %s). Codegen consults it
	// when rendering real C calls.
	cStrArgs map[Expression]bool
}

// NewSema constructs a Sema instance bound to the given diagnostics
// collector (so its errors/warnings interleave with the parser's, in
// source order downstream, when the caller chooses to sort them that
// way).
func NewSema(diags *Diagnostics) *Sema {
	global := newScope(nil)
	return &Sema{
		diags:          diags,
		global:         global,
		funcs:          make(map[string]*Symbol),
		current:        global,
		resolvedTypes:  make(map[Expression]*Type),
		declVarTypes:   make(map[*VarStatement]*Type),
		declConstTypes: make(map[*ConstStatement]*Type),
		importCModules: make(map[string]*CImportModule),
		externCFuncs:   make(map[string]*Symbol),
		cTypes:         make(map[string]*Type),
		structTypes:    make(map[string]*Type),
		structMethods:  make(map[string]map[string]*Symbol),
		cStrArgs:       make(map[Expression]bool),
	}
}

// TypeOf returns the resolved type of a previously-checked expression, or
// nil if it wasn't resolved (e.g. sema failed on it, or it belongs to a
// construct Sema doesn't cover yet).
func (s *Sema) TypeOf(e Expression) *Type { return s.resolvedTypes[e] }

// TypeOfVarDecl / TypeOfConstDecl expose the resolved type of a
// declaration statement itself (covers decl-only statements with no
// initializer expression to key TypeOf off of).
func (s *Sema) TypeOfVarDecl(v *VarStatement) *Type     { return s.declVarTypes[v] }
func (s *Sema) TypeOfConstDecl(c *ConstStatement) *Type { return s.declConstTypes[c] }

func (s *Sema) errorAt(line, col int, format string, args ...interface{}) {
	s.diags.Error("sema", line, col, format, args...)
}

// Check runs semantic analysis over the whole program. It is safe to call
// once per Program; call Errors()/Diagnostics() after to see the result.
func (s *Sema) Check(prog *Program) {
	// Pass 0: process every #importc by invoking the C header parser
	// (clang/gcc) and registering the resulting module under its alias.
	// A failing import is a hard error reported at the directive.
	for _, stmt := range prog.Statements {
		if ics, ok := stmt.(*ImportCStatement); ok {
			s.importCModule(ics)
		}
	}

	// Pass 1: register every struct's type name first, so fields can
	// reference their own struct via pointers (struct Node { next ^Node; })
	// and methods' `self ^Node` parameters resolve during signature
	// registration.
	for _, stmt := range prog.Statements {
		if st, ok := stmt.(*StructStatement); ok {
			s.registerStructName(st)
		}
	}

	// Pass 2: resolve struct fields and register method signatures, now
	// that every struct name is visible.
	for _, stmt := range prog.Statements {
		if st, ok := stmt.(*StructStatement); ok {
			s.resolveStruct(st)
		}
	}

	// Pass 3: register every top-level function signature first, so calls
	// can appear textually before the function they call (Tinoc, like C
	// via forward declarations, allows this -- main() calling helpers
	// defined further down the file is the common case, see samples/*).
	for _, stmt := range prog.Statements {
		if fn, ok := stmt.(*FunctionStatement); ok {
			s.registerFunctionSignature(fn)
		} else if ecs, ok := stmt.(*ExternCFuncStatement); ok {
			s.registerExternCFunc(ecs)
		}
	}

	// Pass 4: check bodies and top-level var/const statements in order.
	for _, stmt := range prog.Statements {
		s.checkStatement(stmt)
	}
}

// importCModule runs the C header parser for one #importc statement and
// registers the resulting module under its alias.
func (s *Sema) importCModule(ics *ImportCStatement) {
	if len(ics.Headers) == 0 {
		s.errorAt(ics.Token.Line, ics.Token.Column, "#importc requires at least one header, e.g. #importc \"stdio.h\" as cio;")
		return
	}
	if _, dup := s.importCModules[ics.Alias]; dup {
		s.errorAt(ics.Token.Line, ics.Token.Column, "C module %s already imported", ics.Alias)
		return
	}
	mod, err := ImportCHeaders(ics.Alias, ics.Headers, s.sourceDir)
	if err != nil {
		s.errorAt(ics.Token.Line, ics.Token.Column, "%v", err)
		return
	}
	s.importCModules[ics.Alias] = mod
	// Fold the header's type names into the global registry so Tinoc
	// source can name them (e.g. `var f FILE` after importing stdio.h).
	for name, t := range mod.Types {
		if _, exists := s.cTypes[name]; !exists {
			s.cTypes[name] = t
		}
	}
}

// === Function Signatures ===

func (s *Sema) registerFunctionSignature(fn *FunctionStatement) {
	if fn.Name == nil {
		return
	}
	name := fn.Name.Value

	if len(fn.GenericParams) > 0 {
		s.errorAt(fn.Token.Line, fn.Token.Column, "generic functions are not yet supported (%s)", name)
		return
	}

	if fn.Variadic {
		s.errorAt(fn.Token.Line, fn.Token.Column, "defining variadic Tinoc functions is not yet supported (%s) — declare the C function instead: extern \"C\" fn %s(...) RetType;", name, name)
		return
	}

	if _, exists := s.funcs[name]; exists {
		s.errorAt(fn.Token.Line, fn.Token.Column, "%s redeclared in this block", name)
		return
	}

	sym := &Symbol{Name: name, Kind: SymFunc}

	seenParams := make(map[string]bool)
	for _, p := range fn.Params {
		if p.Name == nil {
			continue
		}
		pname := p.Name.Value
		if pname != "self" {
			if seenParams[pname] {
				s.errorAt(fn.Token.Line, fn.Token.Column, "duplicate parameter %s", pname)
			}
			seenParams[pname] = true
		}
		var pt *Type
		if p.Type != nil {
			pt = s.resolveTypeExpr(p.Type)
		} else {
			s.errorAt(fn.Token.Line, fn.Token.Column, "parameter %s is missing a type", pname)
			pt = &Type{Kind: KindInvalid}
		}
		sym.Params = append(sym.Params, pt)
		sym.ParamNames = append(sym.ParamNames, pname)
	}

	if fn.ReturnType != nil {
		sym.ReturnType = s.resolveTypeExpr(fn.ReturnType)
	} else {
		sym.ReturnType = typeVoid
	}

	s.funcs[name] = sym
	s.global.Define(sym)
}

// registerExternCFunc registers an `extern "C" fn` declaration: a real C
// function called by its C symbol, but type-checked like any other
// function in Tinoc. A variadic declaration needs at least one named
// parameter before the `...` (C's own rule).
func (s *Sema) registerExternCFunc(ecs *ExternCFuncStatement) {
	if ecs.Name == nil {
		return
	}
	name := ecs.Name.Value
	line, col := ecs.Token.Line, ecs.Token.Column

	if _, exists := s.funcs[name]; exists {
		s.errorAt(line, col, "%s redeclared in this block", name)
		return
	}
	if ecs.Variadic && len(ecs.Params) == 0 {
		s.errorAt(line, col, "variadic C function %s needs at least one named parameter before '...'", name)
		return
	}

	sym := &Symbol{Name: name, Kind: SymFunc, IsCImport: true, CSymbol: ecs.CSymbol, Variadic: ecs.Variadic}

	seenParams := make(map[string]bool)
	for _, p := range ecs.Params {
		if p == nil || p.Name == nil {
			continue
		}
		pname := p.Name.Value
		if seenParams[pname] {
			s.errorAt(line, col, "duplicate parameter %s", pname)
		}
		seenParams[pname] = true
		var pt *Type
		if p.Type != nil {
			pt = s.resolveTypeExpr(p.Type)
		} else {
			s.errorAt(line, col, "parameter %s is missing a type", pname)
			pt = &Type{Kind: KindInvalid}
		}
		sym.Params = append(sym.Params, pt)
		sym.ParamNames = append(sym.ParamNames, pname)
	}

	if ecs.ReturnType != nil {
		sym.ReturnType = s.resolveTypeExpr(ecs.ReturnType)
	} else {
		sym.ReturnType = typeVoid
	}

	s.funcs[name] = sym
	s.externCFuncs[name] = sym
	s.global.Define(sym)
}

// === Structs ===

// registerStructName creates the intered KindStruct *Type for a struct
// declaration (empty fields for now) so later passes can resolve the name
// from anywhere -- including inside the struct's own field types and its
// methods' `self ^Name` parameters, which is what makes `struct Node {
// next ^Node; }` type-check.
func (s *Sema) registerStructName(st *StructStatement) {
	if st.Name == nil {
		return
	}
	name := st.Name.Value
	line, col := st.Token.Line, st.Token.Column

	if _, dup := s.structTypes[name]; dup {
		s.errorAt(line, col, "%s redeclared in this block", name)
		return
	}

	t := &Type{Kind: KindStruct, Name: name, FieldIndex: make(map[string]int)}
	s.structTypes[name] = t
	s.structMethods[name] = make(map[string]*Symbol)
}

// resolveStruct resolves a struct's field types and registers its method
// signatures. Runs after every struct name is registered, so fields and
// self params can reference any struct (including the one being defined).
func (s *Sema) resolveStruct(st *StructStatement) {
	if st.Name == nil {
		return
	}
	t := s.structTypes[st.Name.Value]
	if t == nil {
		return // duplicate/error already reported during name registration
	}

	seen := make(map[string]bool)
	for _, f := range st.Fields {
		if f == nil || f.Name == nil {
			continue
		}
		name := f.Name.Value
		if seen[name] {
			s.errorAt(f.Name.Token.Line, f.Name.Token.Column, "duplicate field %s in struct %s", name, st.Name.Value)
			continue
		}
		seen[name] = true

		var ft *Type
		if f.Type != nil {
			ft = s.resolveTypeExpr(f.Type)
		}
		if ft == nil {
			s.errorAt(f.Name.Token.Line, f.Name.Token.Column, "field %s has an unsupported or unknown type", name)
			ft = &Type{Kind: KindInvalid}
		}
		// A struct cannot contain itself by value (C requires complete
		// types for by-value members); a pointer to itself is fine.
		if ft.Kind == KindStruct && ft.Name == st.Name.Value {
			s.errorAt(f.Name.Token.Line, f.Name.Token.Column, "struct %s contains itself by value (use ^%s for the field type)", st.Name.Value, st.Name.Value)
		}

		t.Fields = append(t.Fields, &StructFieldInfo{Name: name, Type: ft})
		t.FieldIndex[name] = len(t.Fields) - 1
	}

	for _, m := range st.Methods {
		s.registerStructMethod(st.Name.Value, m)
	}
}

// registerStructMethod registers one method (instance or static) of a
// struct into that struct's method table. Instance methods must declare a
// `self` first parameter typed `^Name` or `Name`; static methods take
// ordinary parameters only.
func (s *Sema) registerStructMethod(structName string, fn *FunctionStatement) {
	if fn == nil || fn.Name == nil {
		return
	}
	name := fn.Name.Value
	line, col := fn.Token.Line, fn.Token.Column

	if len(fn.GenericParams) > 0 {
		s.errorAt(line, col, "generic methods are not yet supported (%s.%s)", structName, name)
		return
	}
	if fn.Variadic {
		s.errorAt(line, col, "defining variadic Tinoc methods is not yet supported (%s.%s)", structName, name)
		return
	}
	if _, dup := s.structMethods[structName][name]; dup {
		s.errorAt(line, col, "%s.%s redeclared", structName, name)
		return
	}

	sym := &Symbol{Name: name, Kind: SymFunc, IsStatic: fn.IsStatic}

	seen := make(map[string]bool)
	for _, p := range fn.Params {
		if p == nil || p.Name == nil {
			continue
		}
		pname := p.Name.Value
		if pname != "self" {
			if seen[pname] {
				s.errorAt(line, col, "duplicate parameter %s", pname)
			}
			seen[pname] = true
		}
		var pt *Type
		if p.Type != nil {
			pt = s.resolveTypeExpr(p.Type)
		} else {
			s.errorAt(line, col, "parameter %s is missing a type", pname)
			pt = &Type{Kind: KindInvalid}
		}
		sym.Params = append(sym.Params, pt)
		sym.ParamNames = append(sym.ParamNames, pname)
	}

	if !fn.IsStatic {
		if len(sym.Params) == 0 {
			s.errorAt(line, col, "instance method %s.%s needs a self parameter (self ^%s or self %s)", structName, name, structName, structName)
		} else {
			selfType := sym.Params[0]
			valid := selfType != nil &&
				((selfType.Kind == KindPointer && selfType.Elem != nil && selfType.Elem.Kind == KindStruct && selfType.Elem.Name == structName) ||
					(selfType.Kind == KindStruct && selfType.Name == structName))
			if !valid {
				s.errorAt(line, col, "self parameter of %s.%s must be ^%s or %s, got %s", structName, name, structName, structName, typeStringOrInvalid(selfType))
			}
		}
	}

	if fn.ReturnType != nil {
		sym.ReturnType = s.resolveTypeExpr(fn.ReturnType)
	} else {
		sym.ReturnType = typeVoid
	}

	s.structMethods[structName][name] = sym
}

func typeStringOrInvalid(t *Type) string {
	if t == nil {
		return "<invalid>"
	}
	return t.String()
}

// === Statement Checking ===

func (s *Sema) checkStatement(stmt Statement) {
	switch st := stmt.(type) {
	case *VarStatement:
		s.checkVarStatement(st)
	case *ConstStatement:
		s.checkConstStatement(st)
	case *FunctionStatement:
		s.checkFunctionStatement(st)
	case *ExpressionStatement:
		if st.Expression != nil {
			s.checkExpression(st.Expression)
		}
	case *ReturnStatement:
		s.checkReturnStatement(st)
	case *BlockStatement:
		s.checkBlock(st)
	case *IfStatement:
		s.checkIfStatement(st)
	case *WhileStatement:
		s.checkWhileStatement(st)
	case *ForStatement:
		s.checkForStatement(st)
	case *StructStatement:
		s.checkStructStatement(st)
	case *BreakStatement, *ContinueStatement, *ImportStatement, *ImportCStatement, *ExternCFuncStatement:
		// Nothing to resolve for this pass (imports/extern decls were
		// processed in Check's passes 0-3).
	case nil:
		// Stray/skipped statement (e.g. parser recovered from an error).
	default:
		// Enum/union/switch/etc bodies aren't checked by this pass yet;
		// intentionally silent so partial programs using them don't spam
		// unrelated diagnostics for constructs Sema doesn't cover.
	}
}

func (s *Sema) checkBlock(b *BlockStatement) {
	if b == nil {
		return
	}
	prev := s.current
	s.current = newScope(prev)
	for _, st := range b.Statements {
		s.checkStatement(st)
	}
	s.current = prev
}

// === var / const ===

func (s *Sema) checkVarStatement(v *VarStatement) {
	if v.Name == nil {
		return
	}
	name := v.Name.Value
	line, col := v.Token.Line, v.Token.Column

	declaredType := s.resolveOptionalType(v.Type)

	var valueType *Type
	if v.Value != nil {
		valueType = s.checkExpression(v.Value)
	}

	finalType := s.reconcileDeclType(name, declaredType, valueType, v.Value, line, col)
	if finalType == nil && v.Value == nil && declaredType == nil {
		s.errorAt(line, col, "cannot infer type of %s: no type or initializer given", name)
	}

	s.declVarTypes[v] = finalType

	sym := &Symbol{Name: name, Kind: SymVar, Type: finalType, IsStatic: v.IsStatic, Mutable: true}
	if !s.current.Define(sym) {
		s.errorAt(line, col, "%s redeclared in this block", name)
	}
}

func (s *Sema) checkConstStatement(c *ConstStatement) {
	if c.Name == nil {
		return
	}
	name := c.Name.Value
	line, col := c.Token.Line, c.Token.Column

	if c.Value == nil {
		s.errorAt(line, col, "missing initializer in const declaration of %s", name)
	}

	declaredType := s.resolveOptionalType(c.Type)

	var valueType *Type
	if c.Value != nil {
		valueType = s.checkExpression(c.Value)
	}

	finalType := s.reconcileDeclType(name, declaredType, valueType, c.Value, line, col)
	s.declConstTypes[c] = finalType

	sym := &Symbol{Name: name, Kind: SymConst, Type: finalType, IsStatic: c.IsStatic, Mutable: false}
	if !s.current.Define(sym) {
		s.errorAt(line, col, "%s redeclared in this block", name)
	}
}

// resolveOptionalType resolves a possibly-nil TypeExpr, returning nil
// cleanly when there is none (the "inferred" form) instead of forcing
// every caller to nil-check the TypeExpr itself.
func (s *Sema) resolveOptionalType(te TypeExpr) *Type {
	if te == nil {
		return nil
	}
	return s.resolveTypeExpr(te)
}

// reconcileDeclType implements var/const's three declaration shapes from
// syntax.md:
//
//	var x T;            // declaredType set, no value -> T
//	var x T = expr;      // declaredType set, value present -> must match T
//	var x = expr;         // inferred -> value's type, with untyped-literal
//	                       // adaptation same as Go's untyped constants
//
// valueExpr is passed through so literal expressions can be "widened" in
// place to match an explicit declared type (e.g. `var x i64 = 5` retypes
// the literal 5 from its default-inferred i32 to i64, mirroring how Go
// treats untyped constants).
func (s *Sema) reconcileDeclType(name string, declaredType, valueType *Type, valueExpr Expression, line, col int) *Type {
	switch {
	case declaredType != nil && valueType != nil:
		if isUntypedLiteral(valueExpr) && valueType.isNumeric() && declaredType.isNumeric() {
			// Untyped numeric literal adapts to the declared type,
			// e.g. `var big i64 = 5;` -- 5 defaults to i32 but widens to
			// i64 here without a cast.
			s.resolvedTypes[valueExpr] = declaredType
			return declaredType
		}
		if !assignable(declaredType, valueType) {
			s.errorAt(line, col, "%s", describeMismatch(fmt.Sprintf("initializer for %s", name), valueType, declaredType))
		}
		return declaredType

	case declaredType != nil:
		return declaredType

	case valueType != nil:
		return valueType

	default:
		return nil
	}
}

// isUntypedLiteral reports whether e is a bare integer/float literal,
// which Sema treats as flexibly-typed (like Go's untyped constants) so it
// can adapt to whatever numeric type context it's declared or passed
// into, rather than being pinned to its own default inferred type.
func isUntypedLiteral(e Expression) bool {
	switch e.(type) {
	case *IntegerLiteral, *FloatLiteral:
		return true
	default:
		return false
	}
}

// === fn ===

func (s *Sema) checkFunctionStatement(fn *FunctionStatement) {
	if fn.Name == nil || len(fn.GenericParams) > 0 {
		return // generics: diagnostic already emitted during signature registration
	}

	sym := s.funcs[fn.Name.Value]
	if sym == nil {
		// Signature registration failed (e.g. duplicate name); nothing
		// further to check against.
		return
	}

	s.checkFunctionBody(fn.Name.Value, sym, fn.Params, fn.Body, fn.Token, false)
}

// checkStructStatement checks a struct declaration's method bodies (its
// fields and signatures were resolved during registration).
func (s *Sema) checkStructStatement(st *StructStatement) {
	if st.Name == nil {
		return
	}
	for _, m := range st.Methods {
		if m == nil || m.Name == nil {
			continue
		}
		sym := s.structMethods[st.Name.Value][m.Name.Value]
		if sym == nil {
			continue
		}
		s.checkFunctionBody(st.Name.Value+"."+m.Name.Value, sym, m.Params, m.Body, m.Token, true)
	}
}

// checkFunctionBody is the shared body-checking core for top-level
// functions and struct methods: it establishes the function scope
// (parameters bound, currentFn set), checks every body statement, and
// enforces the missing-return rule for non-void functions. isMethod only
// changes the diagnostic phrasing.
func (s *Sema) checkFunctionBody(label string, sym *Symbol, params []*Parameter, body *BlockStatement, tok Token, isMethod bool) {
	prevFn := s.currentFn
	prevScope := s.current
	s.currentFn = sym
	s.current = newScope(s.global)

	for i, p := range params {
		if p == nil || p.Name == nil {
			continue
		}
		var pt *Type
		if i < len(sym.Params) {
			pt = sym.Params[i]
		}
		s.current.Define(&Symbol{Name: p.Name.Value, Kind: SymParam, Type: pt, Mutable: false})
	}

	if body != nil {
		for _, st := range body.Statements {
			s.checkStatement(st)
		}
		if sym.ReturnType != nil && sym.ReturnType.Kind != KindVoid && !blockAlwaysReturns(body) {
			if isMethod {
				s.errorAt(tok.Line, tok.Column, "missing return at end of method %s", label)
			} else {
				s.errorAt(tok.Line, tok.Column, "missing return at end of function %s", label)
			}
		}
	}

	s.current = prevScope
	s.currentFn = prevFn
}

// blockAlwaysReturns is a conservative, purely-syntactic check for
// "every path through this block ends in a return statement". It
// recognizes direct trailing returns and if/else chains where every
// branch (including a final else) always returns; anything else
// (including loops, since Tinoc has no exhaustiveness signal on them) is
// treated as "may not return", matching Go's own conservative
// terminating-statement analysis in spirit.
func blockAlwaysReturns(b *BlockStatement) bool {
	if b == nil || len(b.Statements) == 0 {
		return false
	}
	last := b.Statements[len(b.Statements)-1]
	return stmtAlwaysReturns(last)
}

func stmtAlwaysReturns(stmt Statement) bool {
	switch st := stmt.(type) {
	case *ReturnStatement:
		return true
	case *BlockStatement:
		return blockAlwaysReturns(st)
	case *IfStatement:
		if st.Alternative == nil {
			return false // no else -> falls through when condition is false
		}
		return blockAlwaysReturns(st.Consequence) && stmtAlwaysReturns(st.Alternative)
	default:
		return false
	}
}

func (s *Sema) checkReturnStatement(r *ReturnStatement) {
	var got *Type
	if r.ReturnValue != nil {
		got = s.checkExpression(r.ReturnValue)
	} else {
		got = typeVoid
	}

	if s.currentFn == nil {
		return // return outside a function; parser-level concern, not sema's
	}

	want := s.currentFn.ReturnType
	if want == nil {
		return
	}

	if want.Kind == KindVoid {
		if r.ReturnValue != nil {
			s.errorAt(r.Token.Line, r.Token.Column, "too many return values\n\thave (%s)\n\twant ()", got.String())
		}
		return
	}

	if r.ReturnValue == nil {
		s.errorAt(r.Token.Line, r.Token.Column, "not enough return values\n\thave ()\n\twant (%s)", want.String())
		return
	}

	if isUntypedLiteral(r.ReturnValue) && got.isNumeric() && want.isNumeric() {
		s.resolvedTypes[r.ReturnValue] = want
		return
	}

	if !assignable(want, got) {
		s.errorAt(r.Token.Line, r.Token.Column, "%s", describeMismatch("return value", got, want))
	}
}

// === if / while / for ===

func (s *Sema) checkIfStatement(i *IfStatement) {
	if i.Condition != nil {
		condType := s.checkExpression(i.Condition)
		s.expectBool(condType, i.Token.Line, i.Token.Column, "if condition")
	}
	s.checkBlock(i.Consequence)
	if i.Alternative != nil {
		s.checkStatement(i.Alternative)
	}
}

func (s *Sema) checkWhileStatement(w *WhileStatement) {
	if w.Condition != nil {
		condType := s.checkExpression(w.Condition)
		s.expectBool(condType, w.Token.Line, w.Token.Column, "while condition")
	}
	s.checkBlock(w.Body)
}

func (s *Sema) checkForStatement(f *ForStatement) {
	prev := s.current
	s.current = newScope(prev)

	var elemType *Type
	if f.Collection != nil {
		s.checkExpression(f.Collection)
		elemType = &Type{Kind: KindUnknown, Name: "elem"} // arrays/slices unresolved in this pass
	} else {
		if f.Start != nil {
			s.checkExpression(f.Start)
		}
		if f.End != nil {
			s.checkExpression(f.End)
		}
		elemType = typeI32
	}

	if f.Capture != nil {
		s.current.Define(&Symbol{Name: f.Capture.Value, Kind: SymVar, Type: elemType, Mutable: true})
	}

	if f.Body != nil {
		for _, st := range f.Body.Statements {
			s.checkStatement(st)
		}
	}

	s.current = prev
}

func (s *Sema) expectBool(t *Type, line, col int, what string) {
	if t == nil {
		return
	}
	if t.Kind != KindBool && t.Kind != KindUnknown && t.Kind != KindInvalid {
		s.errorAt(line, col, "non-boolean %s (type %s)", what, t.String())
	}
}

// === Expression Checking ===
//
// checkExpression resolves and returns the type of an expression,
// recording it in s.resolvedTypes as a side effect so Codegen can query
// it later without recomputing. Every case that can't yet be resolved
// (struct literals, generic calls, etc) returns KindUnknown rather than
// nil so callers downstream of a partially-unsupported expression don't
// cascade into unrelated nil-type errors.

func (s *Sema) checkExpression(e Expression) *Type {
	if e == nil {
		return nil
	}
	t := s.inferExpression(e)
	s.resolvedTypes[e] = t
	return t
}

func (s *Sema) inferExpression(e Expression) *Type {
	switch ex := e.(type) {
	case *Identifier:
		return s.checkIdentifier(ex)
	case *IntegerLiteral:
		return inferIntegerLiteralType(ex)
	case *FloatLiteral:
		return typeF64
	case *StringLiteral:
		return typeStr
	case *CharLiteral:
		return typeChar
	case *BoolLiteral:
		return typeBool
	case *NullLiteral:
		return &Type{Kind: KindUnknown, Name: "null"}
	case *ArrayLiteral:
		for _, el := range ex.Elements {
			s.checkExpression(el)
		}
		return &Type{Kind: KindUnknown, Name: "array"}
	case *PrefixExpression:
		return s.checkPrefixExpression(ex)
	case *PostfixExpression:
		return s.checkPostfixExpression(ex)
	case *InfixExpression:
		return s.checkInfixExpression(ex)
	case *AssignExpression:
		return s.checkAssignExpression(ex)
	case *CallExpression:
		return s.checkCallExpression(ex)
	case *IndexExpression:
		if ex.Left != nil {
			s.checkExpression(ex.Left)
		}
		if ex.Index != nil {
			s.checkExpression(ex.Index)
		}
		return &Type{Kind: KindUnknown, Name: "index"}
	case *FieldAccessExpression:
		// Module member access (cio.printf / io.println) and struct field
		// access both parse to this node. C module members (#importc) are
		// resolved here; other modules stay unresolved.
		return s.checkFieldAccess(ex)
	case *GenericExpression:
		if ex.Base != nil {
			s.checkExpression(ex.Base)
		}
		return &Type{Kind: KindUnknown, Name: "generic"}
	case *StructLiteral:
		return s.checkStructLiteral(ex)
	default:
		return &Type{Kind: KindUnknown}
	}
}

// inferIntegerLiteralType implements syntax.md's integer-literal
// inference: "Compiler will infer the size of each integer literal,
// mostly default to i32 but if literal's size is more than i32 so it
// will go for i64..i128."
func inferIntegerLiteralType(lit *IntegerLiteral) *Type {
	bits := bitsNeeded(lit.Value)
	return intRankForBits(bits)
}

func bitsNeeded(v int64) int {
	if v < 0 {
		v = -v
	}
	bits := 1
	for v > 0x7fffffff { // beyond what fits in a signed 32-bit value
		v >>= 1
		bits++
	}
	if bits == 1 {
		return 32
	}
	return 32 + bits
}

func (s *Sema) checkIdentifier(id *Identifier) *Type {
	if sym, ok := s.current.Lookup(id.Value); ok {
		return sym.Type
	}
	if sym, ok := s.funcs[id.Value]; ok {
		return sym.ReturnType // bare function reference in expr position; rare, best-effort
	}
	// A #importc alias used outside member access (e.g. `var x = cio;`)
	// has no value type; report it as an opaque module rather than an
	// undefined-name error, since the name is defined.
	if _, ok := s.importCModules[id.Value]; ok {
		return &Type{Kind: KindUnknown, Name: "module"}
	}
	s.errorAt(id.Token.Line, id.Token.Column, "undefined: %s", id.Value)
	return &Type{Kind: KindInvalid}
}

// checkFieldAccess resolves `alias.member` for #importc modules, struct
// static member references (`Type.method`), and struct field/method
// access on a struct-typed value (`p.x`, `p.method`, `self^.x`).
// Function members keep their call signature (checked at the call site);
// constant members and struct fields resolve to their types.
func (s *Sema) checkFieldAccess(fa *FieldAccessExpression) *Type {
	// 1. #importc module member: cio.EOF / cio.stdin / cio.printf.
	if mod, ok := s.moduleAlias(fa.Left); ok && fa.Field != nil {
		member := fa.Field.Value
		if fn, ok := mod.Funcs[member]; ok {
			return fn.ReturnType // bare member reference; call sites check the full signature
		}
		if c, ok := mod.Consts[member]; ok {
			return c.Type
		}
		s.errorAt(fa.Token.Line, fa.Token.Column, "undefined: %s.%s", mod.Alias, member)
		return &Type{Kind: KindInvalid}
	}

	// 2. Struct static member reference: `Point.create` (bare, non-call).
	if id, isID := fa.Left.(*Identifier); isID && fa.Field != nil {
		if st, ok := s.structTypes[id.Value]; ok {
			if m, ok := s.structMethods[st.Name][fa.Field.Value]; ok {
				return m.ReturnType // call sites check the full signature
			}
			s.errorAt(fa.Token.Line, fa.Token.Column, "type %s has no static member %s", st.Name, fa.Field.Value)
			return &Type{Kind: KindInvalid}
		}
	}

	// 3. Field/method access on a struct-typed value: `p.x`, `self^.x`,
	// `getPoint().x`. The receiver type drives everything.
	if fa.Left != nil && fa.Field != nil {
		recvType := s.checkExpression(fa.Left)
		if recvType != nil && recvType.Kind == KindStruct {
			if idx, ok := recvType.FieldIndex[fa.Field.Value]; ok {
				return recvType.Fields[idx].Type
			}
			if m, ok := s.structMethods[recvType.Name][fa.Field.Value]; ok {
				return m.ReturnType // bare method reference; call sites check the full signature
			}
			s.errorAt(fa.Token.Line, fa.Token.Column, "type %s has no field or method %s", recvType.Name, fa.Field.Value)
			return &Type{Kind: KindInvalid}
		}
	}

	// Anything else (module names without member access, non-struct field
	// access) stays unresolved; checkExpression on the left still runs so
	// undefined-name errors surface.
	if fa.Left != nil {
		s.checkExpression(fa.Left)
	}
	return &Type{Kind: KindUnknown, Name: "field"}
}

// moduleAlias returns the #importc module aliased by the expression, if
// the expression is a bare identifier naming one.
func (s *Sema) moduleAlias(e Expression) (*CImportModule, bool) {
	id, ok := e.(*Identifier)
	if !ok {
		return nil, false
	}
	mod, ok := s.importCModules[id.Value]
	return mod, ok
}

// checkStructLiteral type-checks `Point { .x = 1.0, .y = 2.0 }` against
// the named struct: every field must exist (with a compatible value
// type), no field may repeat, and every declared field must be present
// (a struct literal is an exhaustive description of the value, so
// omissions are reported rather than silently zero-filled).
func (s *Sema) checkStructLiteral(sl *StructLiteral) *Type {
	st := s.resolveTypeExpr(sl.Type)
	if st == nil {
		return &Type{Kind: KindUnknown, Name: "struct"}
	}
	if st.Kind != KindStruct {
		s.errorAt(sl.Token.Line, sl.Token.Column, "%s is not a struct type", st.String())
		return &Type{Kind: KindUnknown, Name: "struct"}
	}

	seen := make(map[string]bool)
	for _, f := range sl.Fields {
		if f == nil || f.Name == nil {
			continue
		}
		fname := f.Name.Value
		idx, ok := st.FieldIndex[fname]
		if !ok {
			s.errorAt(f.Name.Token.Line, f.Name.Token.Column, "unknown field %s in struct %s", fname, st.Name)
			continue
		}
		if seen[fname] {
			s.errorAt(f.Name.Token.Line, f.Name.Token.Column, "duplicate field %s in struct literal", fname)
			continue
		}
		seen[fname] = true

		ft := st.Fields[idx].Type
		if f.Value == nil {
			continue
		}
		vt := s.checkExpression(f.Value)
		if isUntypedLiteral(f.Value) && vt.isNumeric() && ft.isNumeric() {
			s.resolvedTypes[f.Value] = ft
			continue
		}
		if ft.Kind == KindUnknown || ft.Kind == KindInvalid || vt.Kind == KindUnknown || vt.Kind == KindInvalid {
			continue
		}
		if !assignable(ft, vt) {
			s.errorAt(f.Name.Token.Line, f.Name.Token.Column, "%s", describeMismatch(fmt.Sprintf("field %s", fname), vt, ft))
		}
	}

	var missing []string
	for _, sf := range st.Fields {
		if !seen[sf.Name] {
			missing = append(missing, sf.Name)
		}
	}
	if len(missing) > 0 {
		s.errorAt(sl.Token.Line, sl.Token.Column, "missing field(s) in %s literal: %s", st.Name, strings.Join(missing, ", "))
	}

	return st
}

// checkStructMethodCall type-checks `p.method(args)` (isStatic=false,
// receiver is a value/pointer of struct type) and `Type.method(args)`
// (isStatic=true, receiver is the type name). Instance method arguments
// are checked against the method's parameters after the implicit self
// slot; static methods have no self slot.
func (s *Sema) checkStructMethodCall(ce *CallExpression, st *Type, method string, isStatic bool) *Type {
	methods := s.structMethods[st.Name]
	m, ok := methods[method]
	if !ok {
		s.errorAt(ce.Token.Line, ce.Token.Column, "type %s has no %smethod %s", st.Name, map[bool]string{true: "static ", false: ""}[isStatic], method)
		return &Type{Kind: KindInvalid}
	}
	if m.IsStatic != isStatic {
		if isStatic {
			s.errorAt(ce.Token.Line, ce.Token.Column, "method %s.%s is not static; call it on a value of type %s", st.Name, method, st.Name)
		} else {
			s.errorAt(ce.Token.Line, ce.Token.Column, "static method %s.%s must be called on the type name %s, not on a value", st.Name, method, st.Name)
		}
		return m.ReturnType
	}

	if len(ce.GenericArgs) > 0 {
		s.errorAt(ce.Token.Line, ce.Token.Column, "generic method calls are not yet supported (%s.%s)", st.Name, method)
	}

	offset := 0
	if !isStatic {
		offset = 1 // skip the implicit self parameter
	}
	s.checkCallArgs(ce, m, st.Name+"."+method, offset)

	return m.ReturnType
}

// checkCallArgs checks a call's arguments against a function/method
// symbol, starting at param offset (0 for plain functions and static
// methods, 1 for instance methods whose self slot is implicit). It
// handles variadic counts, untyped-literal adaptation, and the str ->
// C-string-pointer unwrap for #importc/extern "C" calls.
func (s *Sema) checkCallArgs(ce *CallExpression, sym *Symbol, calleeName string, offset int) {
	fixed := len(sym.Params) - offset

	if sym.Variadic {
		if len(ce.Arguments) < fixed {
			s.errorAt(ce.Token.Line, ce.Token.Column, "not enough arguments in call to %s\n\thave (%d args)\n\twant at least (%d args)",
				calleeName, len(ce.Arguments), fixed)
		}
	} else if len(ce.Arguments) != fixed {
		s.errorAt(ce.Token.Line, ce.Token.Column, "not enough arguments in call to %s\n\thave (%d args)\n\twant (%d args)",
			calleeName, len(ce.Arguments), fixed)
	}

	n := len(ce.Arguments)
	if fixed < n {
		n = fixed
	}
	for i := 0; i < n; i++ {
		argType := s.checkExpression(ce.Arguments[i])
		want := sym.Params[offset+i]
		if want == nil || argType == nil {
			continue
		}
		if isUntypedLiteral(ce.Arguments[i]) && argType.isNumeric() && want.isNumeric() {
			s.resolvedTypes[ce.Arguments[i]] = want
			continue
		}
		// Tinoc's `str` (a {data,len} struct) implicitly converts to a C
		// `const char *` / `char *` parameter: codegen passes the data
		// pointer instead of the struct. Record the retype so codegen
		// knows to emit the unwrapped form.
		if sym.IsCImport && argType.Kind == KindStr && isCStringPtr(want) {
			s.resolvedTypes[ce.Arguments[i]] = want
			s.cStrArgs[ce.Arguments[i]] = true
			continue
		}
		if want.Kind == KindUnknown || argType.Kind == KindUnknown || argType.Kind == KindInvalid {
			continue
		}
		if !assignable(want, argType) {
			s.errorAt(ce.Token.Line, ce.Token.Column, "%s", describeMismatch(
				fmt.Sprintf("argument %d to %s", i+1, calleeName), argType, want))
		}
	}
	// Extra arguments beyond the param count (variadic C calls, or
	// already-flagged mismatch calls) still get checked so their own
	// inner expressions are resolved for codegen robustness. str-typed
	// variadic arguments must be unwrapped to their data pointer in C.
	for i := n; i < len(ce.Arguments); i++ {
		argType := s.checkExpression(ce.Arguments[i])
		if sym.IsCImport && argType != nil && argType.Kind == KindStr {
			s.cStrArgs[ce.Arguments[i]] = true
		}
	}
}

func (s *Sema) checkPrefixExpression(pe *PrefixExpression) *Type {
	var rt *Type
	if pe.Right != nil {
		rt = s.checkExpression(pe.Right)
	}
	switch pe.Operator {
	case "-":
		if rt != nil && !rt.isNumeric() && rt.Kind != KindUnknown && rt.Kind != KindInvalid {
			s.errorAt(pe.Token.Line, pe.Token.Column, "invalid operation: operator - not defined on %s (type %s)", pe.Right.String(), rt.String())
		}
		return rt
	case "!":
		s.expectBool(rt, pe.Token.Line, pe.Token.Column, "operand of !")
		return typeBool
	case "~":
		return rt
	case "&":
		return &Type{Kind: KindPointer, Name: "^" + safeTypeName(rt), Elem: rt}
	case "?":
		return rt
	case "-%":
		return rt
	default:
		return rt
	}
}

func safeTypeName(t *Type) string {
	if t == nil {
		return "?"
	}
	return t.Name
}

func (s *Sema) checkPostfixExpression(pe *PostfixExpression) *Type {
	var lt *Type
	if pe.Left != nil {
		lt = s.checkExpression(pe.Left)
	}
	switch pe.Operator {
	case "^": // pointer dereference
		if lt != nil && lt.Kind == KindPointer {
			return lt.Elem
		}
		if lt != nil && lt.Kind != KindUnknown && lt.Kind != KindInvalid {
			s.errorAt(pe.Token.Line, pe.Token.Column, "invalid operation: cannot dereference %s (type %s)", pe.Left.String(), lt.String())
		}
		return &Type{Kind: KindUnknown}
	case "?": // optional unwrap
		return lt
	default:
		return lt
	}
}

// arithmeticOps / comparisonOps / logicalOps classify infix operators for
// checkInfixExpression's per-category rules.
var comparisonOps = map[string]bool{
	"==": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
}

var logicalOps = map[string]bool{
	"and": true, "or": true,
}

func (s *Sema) checkInfixExpression(ie *InfixExpression) *Type {
	var lt, rt *Type
	if ie.Left != nil {
		lt = s.checkExpression(ie.Left)
	}
	if ie.Right != nil {
		rt = s.checkExpression(ie.Right)
	}

	switch {
	case logicalOps[ie.Operator]:
		s.expectBool(lt, ie.Token.Line, ie.Token.Column, "left operand of "+ie.Operator)
		s.expectBool(rt, ie.Token.Line, ie.Token.Column, "right operand of "+ie.Operator)
		return typeBool

	case comparisonOps[ie.Operator]:
		// A bare integer/float literal on either side adapts to the other
		// operand's type, e.g. `f32Val > 0.0` or `i64Val == 0`, the same
		// way the arithmetic branch below adapts literals.
		if isUntypedLiteral(ie.Left) && rt != nil && rt.isNumeric() {
			lt = rt
			s.resolvedTypes[ie.Left] = rt
		} else if isUntypedLiteral(ie.Right) && lt != nil && lt.isNumeric() {
			rt = lt
			s.resolvedTypes[ie.Right] = lt
		}
		s.checkOperandsCompatible(ie, lt, rt)
		// C has no equality operator for structs, so comparing them is
		// rejected outright rather than miscompiled.
		if (lt != nil && lt.Kind == KindStruct) || (rt != nil && rt.Kind == KindStruct) {
			s.errorAt(ie.Token.Line, ie.Token.Column, "invalid operation: cannot compare values of type %s with %s", typeStringOrInvalid(lt), typeStringOrInvalid(rt))
			return typeBool
		}
		return typeBool

	default: // arithmetic / bitwise
		if (lt != nil && lt.Kind == KindStruct) || (rt != nil && rt.Kind == KindStruct) {
			s.errorAt(ie.Token.Line, ie.Token.Column, "invalid operation: operator %s not defined on %s (type %s)", ie.Operator, ie.Left.String(), typeStringOrInvalid(lt))
			return &Type{Kind: KindInvalid}
		}
		// Untyped literal on either side adapts to the other operand's
		// type, e.g. `x + 1` where x is i64 -- 1 adapts to i64 rather
		// than forcing a mismatch against its own default i32.
		if isUntypedLiteral(ie.Left) && rt != nil && rt.isNumeric() {
			lt = rt
			s.resolvedTypes[ie.Left] = rt
		} else if isUntypedLiteral(ie.Right) && lt != nil && lt.isNumeric() {
			rt = lt
			s.resolvedTypes[ie.Right] = lt
		}
		s.checkOperandsCompatible(ie, lt, rt)
		if lt != nil && lt.isNumeric() {
			return lt
		}
		if rt != nil && rt.isNumeric() {
			return rt
		}
		return lt
	}
}

func (s *Sema) checkOperandsCompatible(ie *InfixExpression, lt, rt *Type) {
	if lt == nil || rt == nil {
		return
	}
	if lt.Kind == KindUnknown || rt.Kind == KindUnknown || lt.Kind == KindInvalid || rt.Kind == KindInvalid {
		return
	}
	if !typesEqual(lt, rt) {
		s.errorAt(ie.Token.Line, ie.Token.Column, "invalid operation: %s %s %s (mismatched types %s and %s)",
			ie.Left.String(), ie.Operator, ie.Right.String(), lt.String(), rt.String())
	}
}

func (s *Sema) checkAssignExpression(ae *AssignExpression) *Type {
	var targetType *Type
	if ae.Target != nil {
		targetType = s.checkExpression(ae.Target)
	}
	if id, ok := ae.Target.(*Identifier); ok {
		if sym, found := s.current.Lookup(id.Value); found && !sym.Mutable {
			s.errorAt(ae.Token.Line, ae.Token.Column, "cannot assign to %s (declared const)", id.Value)
		}
	}

	var valueType *Type
	if ae.Value != nil {
		valueType = s.checkExpression(ae.Value)
	}

	if isUntypedLiteral(ae.Value) && targetType != nil && targetType.isNumeric() {
		s.resolvedTypes[ae.Value] = targetType
		valueType = targetType
	}

	if targetType != nil && valueType != nil &&
		targetType.Kind != KindUnknown && targetType.Kind != KindInvalid &&
		valueType.Kind != KindUnknown && valueType.Kind != KindInvalid {
		if ae.Operator == "=" {
			if !assignable(targetType, valueType) {
				s.errorAt(ae.Token.Line, ae.Token.Column, "%s", describeMismatch("assignment", valueType, targetType))
			}
		} else {
			// Compound assign (+=, -=, etc): both sides must already be
			// numeric and matching, same as the equivalent binary op.
			if !targetType.isNumeric() {
				s.errorAt(ae.Token.Line, ae.Token.Column, "invalid operation: operator %s not defined on %s (type %s)", ae.Operator, ae.Target.String(), targetType.String())
			} else if !typesEqual(targetType, valueType) {
				s.errorAt(ae.Token.Line, ae.Token.Column, "invalid operation: mismatched types %s and %s", targetType.String(), valueType.String())
			}
		}
	}

	return targetType
}

func (s *Sema) checkCallExpression(ce *CallExpression) *Type {
	calleeName := ""
	var sym *Symbol

	if fa, ok := ce.Function.(*FieldAccessExpression); ok {
		// `cio.printf(...)`: resolve the member signature inside the
		// #importc module. Non-C module calls (io.println) stay
		// unresolved, same as before.
		if mod, isMod := s.moduleAlias(fa.Left); isMod && fa.Field != nil {
			member := fa.Field.Value
			if fn, ok := mod.Funcs[member]; ok {
				sym = fn
				calleeName = mod.Alias + "." + member
			} else {
				for _, a := range ce.Arguments {
					s.checkExpression(a)
				}
				s.errorAt(ce.Token.Line, ce.Token.Column, "undefined: %s.%s", mod.Alias, member)
				return &Type{Kind: KindInvalid}
			}
		} else if fa.Left != nil && fa.Field != nil {
			// Static method call: `Point.create(...)` — the receiver is the
			// bare type name, which resolves against the struct registry
			// (never as a value, so checkExpression is skipped for it).
			if id, isID := fa.Left.(*Identifier); isID {
				if st, ok := s.structTypes[id.Value]; ok {
					return s.checkStructMethodCall(ce, st, fa.Field.Value, true)
				}
			}
			// Instance method call: `p.translate(...)` on a struct-typed
			// (or pointer-to-struct) receiver.
			recvType := s.checkExpression(fa.Left)
			switch {
			case recvType != nil && recvType.Kind == KindStruct:
				return s.checkStructMethodCall(ce, recvType, fa.Field.Value, false)
			case recvType != nil && recvType.Kind == KindPointer && recvType.Elem != nil && recvType.Elem.Kind == KindStruct:
				return s.checkStructMethodCall(ce, recvType.Elem, fa.Field.Value, false)
			default:
				for _, a := range ce.Arguments {
					s.checkExpression(a)
				}
				return &Type{Kind: KindUnknown, Name: "call"}
			}
		} else {
			if ce.Function != nil {
				s.checkExpression(ce.Function)
			}
			for _, a := range ce.Arguments {
				s.checkExpression(a)
			}
			return &Type{Kind: KindUnknown, Name: "call"}
		}
	} else if ident, isIdent := ce.Function.(*Identifier); isIdent {
		var ok bool
		sym, ok = s.funcs[ident.Value]
		if !ok {
			// No first-class function values in this pass; treat as
			// unknown-callee.
			for _, a := range ce.Arguments {
				s.checkExpression(a)
			}
			s.errorAt(ce.Token.Line, ce.Token.Column, "undefined: %s", ident.Value)
			return &Type{Kind: KindInvalid}
		}
		calleeName = ident.Value
	} else {
		if ce.Function != nil {
			s.checkExpression(ce.Function)
		}
		for _, a := range ce.Arguments {
			s.checkExpression(a)
		}
		return &Type{Kind: KindUnknown, Name: "call"}
	}

	if len(ce.GenericArgs) > 0 {
		s.errorAt(ce.Token.Line, ce.Token.Column, "generic calls are not yet supported (%s)", calleeName)
	}

	s.checkCallArgs(ce, sym, calleeName, 0)

	return sym.ReturnType
}

// === CLI Entry Point ===

// RunSema runs semantic analysis over source text (lexing + parsing it
// first) and returns the analyzed Program, the Sema instance (for
// Codegen to query resolved types from), and the full diagnostic set
// (parser errors first, then sema errors, matching the order a person
// would want to fix them in).
func RunSema(file, source string) (*Program, *Sema, *Diagnostics) {
	diags := NewDiagnostics(file)

	prog, parseErrs := ParseSource(source)
	for _, pe := range parseErrs {
		diags.items = append(diags.items, diagFromParseError(file, pe))
	}

	sema := NewSema(diags)
	sema.sourceDir = filepath.Dir(file)
	if len(parseErrs) == 0 {
		sema.Check(prog)
	}

	return prog, sema, diags
}
