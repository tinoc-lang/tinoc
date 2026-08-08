package src

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

// === Codegen ===
//
// Codegen turns a Sema-checked *Program into C11 source text. It only
// emits code for the constructs this pass supports end-to-end: var,
// const, static var, static const, and fn (including their bodies:
// return, if/else, while, for (range and collection forms), break,
// continue, expression statements, and the expressions those bodies are
// built from).
//
// Anything Sema marked KindUnknown (struct/enum/union/generic/etc) is not
// silently miscompiled: Codegen records a clear diagnostic and skips
// emitting that construct, rather than guessing at C it can't back with a
// resolved type.
//
// Output shape mirrors the convention already established by
// utils/tinoc.h and README.md's transpile example: an `#include
// "tinoc.h"` line, followed by top-level declarations in source order.

// Codegen holds the state needed to walk a checked Program and produce C.
type Codegen struct {
	sema  *Sema
	diags *Diagnostics

	out       bytes.Buffer
	indent    int
	loopDepth int

	// inMain tracks whether the function currently being generated is
	// Tinoc's `main`, which C requires to return int even though Tinoc's
	// own signature is `fn main() void`. genReturn consults this to turn
	// a bare `return;` into `return 0;` instead of emitting an invalid
	// value-less return from an int function.
	inMain bool

	// curRetC is the C return type of the function currently being
	// generated, and curRetNeedsValue whether that type is non-void. An
	// exhaustive enum switch (all variants matched, no `_` arm) leaves C
	// unable to prove the function returns; genSwitch appends a dead-code
	// `default: return <zero>;` arm to silence the warning without
	// changing behavior.
	curRetC          string
	curRetNeedsValue bool

	// forwardDecls collects C prototypes for every Tinoc function so
	// call order in the source doesn't have to match C's top-to-bottom
	// declare-before-use rule (Tinoc, like Sema's own signature pass,
	// allows a function to call one defined later in the file).
	forwardDecls []string

	// sliceTypeDefs holds the named typedefs codegen emits for every
	// slice type used in the program (`[]i32` -> tnc_slice_i32). Named
	// typedefs keep prototypes and definitions of the same slice type
	// identical in C; the tinoc_slice(T) macro alone expands to a fresh
	// anonymous struct at each use, which C treats as distinct types.
	sliceTypeDefs []string

	// optionalTypeDefs holds the named typedefs codegen emits for every
	// optional type used in the program (`?i32` -> tnc_opt_i32). Same
	// rationale as sliceTypeDefs: a named typedef keeps every
	// prototype, definition and declaration of the same optional type
	// identical in C (see collectOptionalTypes).
	optionalTypeDefs []string

	// sourceDir is the directory of the .tnc file, used to decide whether
	// a #importc header is a local file (quote include) or a system header
	// (angle include).
	sourceDir string

	// cIncludes collects the `#include` lines demanded by #importc, and
	// cSymbolsIncluded the C function names those headers already declare
	// (so an extern "C" fn for the same symbol doesn't emit a duplicate,
	// possibly conflicting, prototype).
	cIncludes        []string
	cSymbolsIncluded map[string]bool
}

// NewCodegen constructs a Codegen bound to a Sema instance that has
// already run Check() on the program being generated, so type queries
// (TypeOf/TypeOfVarDecl/TypeOfConstDecl) return real answers.
func NewCodegen(sema *Sema, diags *Diagnostics) *Codegen {
	return &Codegen{sema: sema, diags: diags}
}

func (g *Codegen) errorAt(line, col int, format string, args ...interface{}) {
	g.diags.Error("codegen", line, col, format, args...)
}

func (g *Codegen) writeIndent() {
	g.out.WriteString(strings.Repeat("    ", g.indent))
}

func (g *Codegen) writef(format string, args ...interface{}) {
	fmt.Fprintf(&g.out, format, args...)
}

func (g *Codegen) writeln(format string, args ...interface{}) {
	g.writeIndent()
	fmt.Fprintf(&g.out, format, args...)
	g.out.WriteString("\n")
}

// Generate walks the whole program and returns the generated C source. It
// is safe to call once per Codegen instance.
func (g *Codegen) Generate(prog *Program) string {
	// Pre-pass: collect every #importc's include lines and the C symbols
	// those headers declare, before any generation starts, so extern "C"
	// prototypes can be deduped regardless of statement order.
	g.cSymbolsIncluded = make(map[string]bool)
	for _, stmt := range prog.Statements {
		if ics, ok := stmt.(*ImportCStatement); ok {
			for _, h := range ics.Headers {
				g.cIncludes = append(g.cIncludes, cIncludeDirective(h, g.sourceDir))
			}
			if mod, ok := g.sema.importCModules[ics.Alias]; ok {
				for name := range mod.Funcs {
					g.cSymbolsIncluded[name] = true
				}
			}
		}
	}

	// Type pre-pass: emit every slice and optional typedef, then enum's,
	// struct's and union's C types first (in source order), so
	// function/method prototypes that mention them -- emitted later in
	// the forward-decl block -- and every use in bodies sees a complete
	// type. Slice typedefs come first because struct fields and
	// signatures commonly reference them and optional payloads can be
	// slices; optional typedefs come next because struct and enum
	// fields can themselves be optional. Enums are emitted before
	// structs because struct fields commonly reference enum types;
	// by-value struct payloads on enums still require the struct to be
	// declared first, same as in C itself, and surface at C compile
	// time.
	g.collectSliceTypes(prog)
	g.collectOptionalTypes(prog)
	var types bytes.Buffer
	main := &g.out
	g.out = types
	for _, td := range g.sliceTypeDefs {
		g.writeln("%s", td)
	}
	if len(g.sliceTypeDefs) > 0 {
		g.writeln("")
	}
	for _, td := range g.optionalTypeDefs {
		g.writeln("%s", td)
	}
	if len(g.optionalTypeDefs) > 0 {
		g.writeln("")
	}
	for _, stmt := range prog.Statements {
		if es, ok := stmt.(*EnumStatement); ok {
			g.genEnumTypeDef(es)
		}
	}
	for _, stmt := range prog.Statements {
		if st, ok := stmt.(*StructStatement); ok {
			g.genStructTypeDef(st)
		}
	}
	for _, stmt := range prog.Statements {
		if us, ok := stmt.(*UnionStatement); ok {
			g.genUnionTypeDef(us)
		}
	}
	types = g.out
	g.out = *main

	var body bytes.Buffer

	// Emit function definitions, method definitions, and top-level
	// var/const in source order into `body`, while collecting forward
	// declarations (functions and methods) into g.forwardDecls as we go,
	// so the final output can place all prototypes before any definition.
	g.out = body
	for _, stmt := range prog.Statements {
		g.genTopLevelStatement(stmt)
	}
	body = g.out
	g.out = *main

	var final bytes.Buffer
	final.WriteString("// Code generated by tinoc. DO NOT EDIT.\n")
	final.WriteString("#include \"tinoc.h\"\n")
	for _, inc := range g.cIncludes {
		final.WriteString(inc)
		final.WriteString("\n")
	}
	final.WriteString("\n")

	final.Write(types.Bytes())
	if types.Len() > 0 {
		final.WriteString("\n")
	}

	if len(g.forwardDecls) > 0 {
		for _, d := range g.forwardDecls {
			final.WriteString(d)
			final.WriteString(";\n")
		}
		final.WriteString("\n")
	}

	final.Write(body.Bytes())

	return final.String()
}

// genTopLevelStatement dispatches the handful of statement kinds allowed
// directly at file scope: function declarations and var/const (including
// their static forms). Anything else at top level is out of scope for
// this pass (struct/enum/union/#import/test/etc) and is reported rather
// than silently dropped.
func (g *Codegen) genTopLevelStatement(stmt Statement) {
	switch st := stmt.(type) {
	case *FunctionStatement:
		g.genFunction(st)
	case *VarStatement:
		g.genTopLevelVar(st)
	case *ConstStatement:
		g.genTopLevelConst(st)
	case *StructStatement:
		g.genStructMethods(st)
	case *EnumStatement:
		g.genEnumMethods(st)
	case *UnionStatement:
		g.genUnionMethods(st)
	case *ImportStatement:
		// #import is Sema-level module resolution, not a C construct;
		// nothing to emit. (std.io's io.println etc are not yet backed
		// by a real standard library in this pass.)
	case *ImportCStatement:
		// Includes were collected in the pre-pass; nothing to emit here.
	case *ExternCFuncStatement:
		g.genExternCFuncProto(st)
	case nil:
	default:
		g.errorAt(0, 0, "codegen: unsupported top-level statement %T", stmt)
	}
}

// genExternCFuncProto emits the C prototype for an `extern "C" fn`
// declaration (unless the symbol is already declared by a #importc
// include, in which case the header's own declaration is used).
func (g *Codegen) genExternCFuncProto(ecs *ExternCFuncStatement) {
	if ecs.Name == nil || ecs.CSymbol == "" {
		return
	}
	if g.cSymbolsIncluded[ecs.CSymbol] {
		return
	}
	ret := "void"
	if ecs.ReturnType != nil {
		ret = cDeclTypeSpelling(ecs.ReturnType)
	}
	g.forwardDecls = append(g.forwardDecls, fmt.Sprintf("%s %s(%s)", ret, ecs.CSymbol, cParamListSpelling(ecs)))
}

// === Enums ===

// genEnumTypeDef emits one enum's C type. Fieldless enums become a plain
// C enum whose constants are namespaced as `<Enum>_<Variant>` (so
// variants of different enums never collide); enums with payload data
// become a tagged union -- a `tag` field plus an anonymous union holding
// one `<Variant>_<index>` member per payload slot -- with the variant
// tags emitted as anonymous-enum integer constants so switch case labels
// stay real integer constant expressions.
func (g *Codegen) genEnumTypeDef(es *EnumStatement) {
	if es.Name == nil {
		return
	}
	t := g.sema.enumTypes[es.Name.Value]
	if t == nil {
		return
	}
	name := sanitizeCIdent(es.Name.Value)

	if t.HasPayload {
		g.writeln("typedef struct %s {", name)
		g.indent++
		g.writeln("i32 tag;")
		g.writeln("union {")
		g.indent++
		for _, v := range t.EnumVariants {
			if v == nil || len(v.Types) == 0 {
				continue
			}
			// Each payload-carrying variant gets its own anonymous struct
			// inside the union, so multi-field payloads don't overlap
			// each other (a flattened union would alias Rect.w with
			// Rect.h and corrupt payloads at runtime).
			g.writeln("struct {")
			g.indent++
			for i, pt := range v.Types {
				if pt == nil {
					continue
				}
				g.writeln("%s %s_%d;", cTypeOrFallback(pt), sanitizeCIdent(v.Name), i)
			}
			g.indent--
			g.writeln("} %s;", sanitizeCIdent(v.Name))
		}
		g.indent--
		g.writeln("} data;")
		g.indent--
		g.writeln("} %s;", name)
		g.writeln("enum {")
		g.indent++
		for i, v := range t.EnumVariants {
			if v == nil {
				continue
			}
			g.writeln("%s_%s = %d,", name, sanitizeCIdent(v.Name), i)
		}
		g.indent--
		g.writeln("};")
	} else {
		g.writeln("typedef enum %s {", name)
		g.indent++
		for i, v := range t.EnumVariants {
			if v == nil {
				continue
			}
			line := fmt.Sprintf("%s_%s", name, sanitizeCIdent(v.Name))
			if i < len(t.EnumVariants)-1 {
				line += ","
			}
			g.writeln("%s", line)
		}
		g.indent--
		g.writeln("} %s;", name)
	}
	g.out.WriteString("\n")
}

// genEnumMethods emits every method of an enum, mirroring
// genStructMethods: C prototypes go into the shared forward-decl block
// and definitions follow the enum's typedef.
func (g *Codegen) genEnumMethods(es *EnumStatement) {
	if es.Name == nil {
		return
	}
	for _, m := range es.Methods {
		if m != nil {
			g.genTypeMethod(es.Name.Value, g.sema.enumMethods[es.Name.Value], m)
		}
	}
}

// === Structs ===

// genStructTypeDef emits one struct's C type: a tag forward declaration
// (`struct Point;`) followed by the typedef (`typedef struct Point {
// ... } Point;`). The tag forward decl lets pointer fields reference
// structs defined later in the file (including the struct itself). Field
// types use the C tag spelling (`struct Point`, `struct Node*`) so
// self-/mutually-referential pointer fields compile regardless of
// declaration order.
func (g *Codegen) genStructTypeDef(st *StructStatement) {
	if st.Name == nil {
		return
	}
	name := sanitizeCIdent(st.Name.Value)

	g.writeln("struct %s;", name)
	g.writeln("typedef struct %s {", name)
	g.indent++
	if resolved, ok := g.sema.structTypes[st.Name.Value]; ok {
		for _, f := range resolved.Fields {
			if f == nil {
				continue
			}
			g.writeln("%s;", cDeclarator(f.Type, sanitizeCIdent(f.Name)))
		}
	}
	g.indent--
	g.writeln("} %s;", name)
	g.out.WriteString("\n")
}

// genStructMethods emits every method of a struct: C prototypes go into
// the shared forward-decl block (so methods can call one another
// regardless of declaration order), and definitions follow immediately
// after the struct's typedef. Instance methods take an implicit `self`
// first parameter (a pointer to the struct for `self ^Name`, or the
// struct by value for `self Name`); static methods take only their
// declared parameters.
func (g *Codegen) genStructMethods(st *StructStatement) {
	if st.Name == nil {
		return
	}
	for _, m := range st.Methods {
		if m != nil {
			g.genTypeMethod(st.Name.Value, g.sema.structMethods[st.Name.Value], m)
		}
	}
}

// genUnionTypeDef emits one union's C type: a tag forward declaration
// (`union Data;`) followed by the typedef (`typedef union Data { ... }
// Data;`). All fields share one memory location at runtime, exactly as
// in C -- the declared field types simply overlap, so writing one field
// and reading another reinterprets the same bytes. Field types reuse the
// struct field C-type rendering so struct-typed fields still compile
// regardless of declaration order.
func (g *Codegen) genUnionTypeDef(us *UnionStatement) {
	if us.Name == nil {
		return
	}
	name := sanitizeCIdent(us.Name.Value)

	g.writeln("union %s;", name)
	g.writeln("typedef union %s {", name)
	g.indent++
	if resolved, ok := g.sema.unionTypes[us.Name.Value]; ok {
		for _, f := range resolved.Fields {
			if f == nil {
				continue
			}
			g.writeln("%s;", cDeclarator(f.Type, sanitizeCIdent(f.Name)))
		}
	}
	g.indent--
	g.writeln("} %s;", name)
	g.out.WriteString("\n")
}

// genUnionMethods emits every method of a union, mirroring
// genStructMethods: C prototypes go into the shared forward-decl block
// and definitions follow the union's typedef.
func (g *Codegen) genUnionMethods(us *UnionStatement) {
	if us.Name == nil {
		return
	}
	for _, m := range us.Methods {
		if m != nil {
			g.genTypeMethod(us.Name.Value, g.sema.unionMethods[us.Name.Value], m)
		}
	}
}

// genTypeMethod renders a single struct/enum method (instance or
// static). Method symbols are resolved from Sema's per-type method table
// (never s.funcs), and the C name is mangled as tnc_<Type>_<method> so
// methods of different types never collide.
func (g *Codegen) genTypeMethod(typeName string, methods map[string]*Symbol, fn *FunctionStatement) {
	if fn.Name == nil {
		return
	}

	sym := methods[fn.Name.Value]
	if sym == nil {
		g.errorAt(fn.Token.Line, fn.Token.Column, "codegen: no resolved signature for %s.%s (sema did not run or failed)", typeName, fn.Name.Value)
		return
	}

	retC := cReturnType(sym.ReturnType, fn.Name.Value)
	cName := "tnc_" + sanitizeCIdent(typeName) + "_" + sanitizeCIdent(fn.Name.Value)

	var params []string
	offset := 0
	if !fn.IsStatic && len(sym.Params) > 0 {
		params = append(params, fmt.Sprintf("%s self", cSelfParamType(sym.Params[0])))
		offset = 1
	}
	for i := offset; i < len(sym.ParamNames); i++ {
		var pt *Type
		if i < len(sym.Params) {
			pt = sym.Params[i]
		}
		params = append(params, cDeclarator(pt, sanitizeCIdent(sym.ParamNames[i])))
	}
	if len(params) == 0 {
		params = []string{"void"}
	}
	paramList := strings.Join(params, ", ")

	g.forwardDecls = append(g.forwardDecls, fmt.Sprintf("%s %s(%s)", retC, cName, paramList))

	prevRetC := g.curRetC
	prevRetNeeds := g.curRetNeedsValue
	g.curRetC = retC
	g.curRetNeedsValue = sym.ReturnType != nil && sym.ReturnType.Kind != KindVoid

	g.writeln("%s %s(%s) {", retC, cName, paramList)
	g.indent++
	if fn.Body != nil {
		for _, st := range fn.Body.Statements {
			g.genStatement(st)
		}
	}
	g.indent--
	g.writeln("}")
	g.out.WriteString("\n")

	g.curRetC = prevRetC
	g.curRetNeedsValue = prevRetNeeds
}

// cSelfParamType renders the C type for a method's self parameter: `^T`
// receivers become pointers, `T` receivers pass the struct by value.
func cSelfParamType(selfType *Type) string {
	if selfType == nil || selfType.Kind == KindInvalid {
		return "void*"
	}
	if selfType.Kind == KindPointer {
		elem := "void"
		if selfType.Elem != nil {
			elem = selfType.Elem.CType()
		}
		return elem + "*"
	}
	return cTypeOrFallback(selfType)
}

// structFieldCType renders a struct field's C type. Struct-typed fields
// use the tag spelling (`struct Point`, `struct Point*`) which compiles
// even when the referenced struct is declared later in the file (tag
// forward declarations were emitted in the struct pre-pass); everything
// else uses the ordinary CType mapping.
func structFieldCType(t *Type) string {
	if t == nil || t.Kind == KindInvalid || t.Kind == KindUnknown {
		return "void*"
	}
	if t.Kind == KindStruct {
		return "struct " + sanitizeCIdent(t.Name)
	}
	if t.Kind == KindPointer && t.Elem != nil && t.Elem.Kind == KindStruct {
		return "struct " + sanitizeCIdent(t.Elem.Name) + "*"
	}
	return t.CType()
}

// cDeclarator renders a full C declaration for a value, placing array
// dimensions after the identifier per C's declarator grammar:
// `[5][3]f32` x -> "f32 x[5][3]". Sentinel arrays allocate N+1 slots
// (see arrayDimSuffix). Struct-typed values use the tag form (`struct
// Point`) so forward declarations suffice regardless of declaration
// order, matching structFieldCType.
func cDeclarator(t *Type, name string) string {
	if t == nil || t.Kind == KindInvalid || t.Kind == KindUnknown {
		return "void* " + name
	}
	if t.Kind == KindArray {
		return cDeclarator(t.Elem, name+arrayDimSuffix(t))
	}
	return structFieldCType(t) + " " + name
}

// === Functions ===

func (g *Codegen) genFunction(fn *FunctionStatement) {
	if fn.Name == nil {
		return
	}
	if len(fn.GenericParams) > 0 {
		g.errorAt(fn.Token.Line, fn.Token.Column, "codegen: generic functions are not yet supported (%s)", fn.Name.Value)
		return
	}

	sym := g.sema.funcs[fn.Name.Value]
	if sym == nil {
		g.errorAt(fn.Token.Line, fn.Token.Column, "codegen: no resolved signature for %s (sema did not run or failed)", fn.Name.Value)
		return
	}

	retC := cReturnType(sym.ReturnType, fn.Name.Value)

	var params []string
	for i, pname := range sym.ParamNames {
		var pt *Type
		if i < len(sym.Params) {
			pt = sym.Params[i]
		}
		params = append(params, cDeclarator(pt, sanitizeCIdent(pname)))
	}
	if len(params) == 0 {
		params = []string{"void"}
	}
	paramList := strings.Join(params, ", ")

	cName := cFunctionName(fn.Name.Value)

	// C requires main() to return int; Tinoc's `fn main() void` maps to
	// `int main(void)` with an implicit `return 0;` appended, matching
	// the transpile example in README.md.
	isMain := fn.Name.Value == "main" && cName == "main"
	if isMain {
		retC = "int"
	}

	g.forwardDecls = append(g.forwardDecls, fmt.Sprintf("%s %s(%s)", retC, cName, paramList))

	prevInMain := g.inMain
	prevRetC := g.curRetC
	prevRetNeeds := g.curRetNeedsValue
	g.inMain = isMain
	g.curRetC = retC
	g.curRetNeedsValue = isMain || (sym.ReturnType != nil && sym.ReturnType.Kind != KindVoid)

	g.writeln("%s %s(%s) {", retC, cName, paramList)
	g.indent++
	if fn.Body != nil {
		for _, st := range fn.Body.Statements {
			g.genStatement(st)
		}
	}
	if isMain && !g.sema.blockAlwaysReturns(fn.Body) {
		g.writeln("return 0;")
	}
	g.indent--
	g.writeln("}")
	g.out.WriteString("\n")

	g.inMain = prevInMain
	g.curRetC = prevRetC
	g.curRetNeedsValue = prevRetNeeds
}

// cFunctionName maps a Tinoc function name to its C symbol. `main` passes
// through unchanged (it must, to be a valid C entry point); every other
// name is prefixed to avoid collisions with C standard library symbols
// (e.g. a Tinoc function literally named `printf`).
func cFunctionName(name string) string {
	if name == "main" {
		return "main"
	}
	return "tnc_" + sanitizeCIdent(name)
}

func cReturnType(t *Type, fnName string) string {
	if t == nil {
		return "void"
	}
	if t.Kind == KindUnknown {
		return "void /* unresolved return type */"
	}
	return t.CType()
}

func cTypeOrFallback(t *Type) string {
	if t == nil || t.Kind == KindInvalid {
		return "void*"
	}
	if t.Kind == KindUnknown {
		return "void*"
	}
	return t.CType()
}

// sanitizeCIdent guards against Tinoc identifiers that collide with C
// keywords (e.g. a Tinoc variable named `register` or `union`); such
// names get a suffix so the emitted C still compiles.
func sanitizeCIdent(name string) string {
	if cKeywords[name] {
		return name + "_"
	}
	return name
}

var cKeywords = map[string]bool{
	"auto": true, "break": true, "case": true, "char": true, "const": true,
	"continue": true, "default": true, "do": true, "double": true, "else": true,
	"enum": true, "extern": true, "float": true, "for": true, "goto": true,
	"if": true, "int": true, "long": true, "register": true, "return": true,
	"short": true, "signed": true, "sizeof": true, "static": true, "struct": true,
	"switch": true, "typedef": true, "union": true, "unsigned": true, "void": true,
	"volatile": true, "while": true, "inline": true, "restrict": true,
	"_Bool": true, "_Complex": true, "_Imaginary": true,
}

// === Top-level var / const ===

func (g *Codegen) genTopLevelVar(v *VarStatement) {
	t := g.sema.TypeOfVarDecl(v)
	if !g.checkEmittable(t, v.Token.Line, v.Token.Column, "var "+identName(v.Name)) {
		return
	}
	storage := "static "
	if !v.IsStatic {
		// A plain top-level `var` still needs C storage-class handling;
		// Tinoc's grammar only shows var/const inside function bodies in
		// syntax.md, but since file scope is reachable syntactically,
		// non-static top-level bindings are emitted as ordinary C globals.
		storage = ""
	}
	init := ""
	if v.Value != nil {
		init = " = " + g.genInit(v.Value, t)
	} else {
		init = " = " + zeroValue(t)
	}
	g.writeln("%s%s%s;", storage, cDeclarator(t, sanitizeCIdent(v.Name.Value)), init)
}

func (g *Codegen) genTopLevelConst(c *ConstStatement) {
	t := g.sema.TypeOfConstDecl(c)
	if !g.checkEmittable(t, c.Token.Line, c.Token.Column, "const "+identName(c.Name)) {
		return
	}
	init := ""
	if c.Value != nil {
		init = " = " + g.genInit(c.Value, t)
	}
	// `static` is applied regardless of IsStatic for top-level const,
	// since C requires internal linkage for a header-free single
	// translation unit and Tinoc const at file scope has no external
	// visibility story yet (no `pub` propagation to codegen in this pass).
	g.writeln("static const %s%s;", cDeclarator(t, sanitizeCIdent(c.Name.Value)), init)
}

func identName(id *Identifier) string {
	if id == nil {
		return "<anonymous>"
	}
	return id.Value
}

// checkEmittable reports (and records a diagnostic for) a nil/unresolved/
// invalid type before codegen tries to use it, so a Sema failure upstream
// turns into one clear codegen-side message instead of a nil-pointer
// panic or silently wrong C.
func (g *Codegen) checkEmittable(t *Type, line, col int, what string) bool {
	if t == nil {
		g.errorAt(line, col, "codegen: %s has no resolved type (sema error upstream)", what)
		return false
	}
	if t.Kind == KindInvalid {
		g.errorAt(line, col, "codegen: %s has an invalid type", what)
		return false
	}
	if t.Kind == KindArray || t.Kind == KindSlice {
		if t.Elem == nil {
			g.errorAt(line, col, "codegen: %s has an unresolved element type", what)
			return false
		}
		return true
	}
	if t.Kind == KindUnknown {
		g.errorAt(line, col, "codegen: %s has an unsupported type (%s) — error-union/generic codegen is not implemented yet", what, t.Name)
		return false
	}
	return true
}

// zeroValue renders a type's C zero-value, used for `var x T;` (decl-only,
// no initializer) so the emitted C still gives the binding a defined
// value rather than leaving it uninitialized (Tinoc's own semantics for
// decl-only var are not yet pinned down in syntax.md, so zero-init is the
// conservative, safest default).
func zeroValue(t *Type) string {
	if t == nil {
		return "{0}"
	}
	switch t.Kind {
	case KindBool:
		return "false"
	case KindInt, KindChar:
		return "0"
	case KindFloat:
		return "0.0"
	case KindStr:
		return "tinoc_str_lit(\"\", 0)"
	case KindPointer:
		return "NULL"
	case KindStruct, KindEnum, KindUnion:
		// Aggregate zero-initializer: every member gets zeroed, matching
		// Tinoc's decl-only `var p Point;` semantics.
		return "{0}"
	default:
		return "{0}"
	}
}

// === Statements (function-body level) ===

func (g *Codegen) genStatement(stmt Statement) {
	switch st := stmt.(type) {
	case *VarStatement:
		g.genLocalVar(st)
	case *ConstStatement:
		g.genLocalConst(st)
	case *ReturnStatement:
		g.genReturn(st)
	case *ExpressionStatement:
		if st.Expression != nil {
			g.writeln("%s;", g.genExpr(st.Expression))
		}
	case *IfStatement:
		g.genIf(st)
	case *WhileStatement:
		g.genWhile(st)
	case *ForStatement:
		g.genFor(st)
	case *BreakStatement:
		if g.loopDepth == 0 {
			g.errorAt(st.Token.Line, st.Token.Column, "codegen: break outside a loop")
			return
		}
		g.writeln("break;")
	case *SwitchStatement:
		g.genSwitch(st)
	case *ContinueStatement:
		if g.loopDepth == 0 {
			g.errorAt(st.Token.Line, st.Token.Column, "codegen: continue outside a loop")
			return
		}
		g.writeln("continue;")
	case *BlockStatement:
		g.writeln("{")
		g.indent++
		for _, s := range st.Statements {
			g.genStatement(s)
		}
		g.indent--
		g.writeln("}")
	case *ImportStatement:
		// no-op at statement level too
	case nil:
	default:
		g.errorAt(0, 0, "codegen: unsupported statement %T", stmt)
	}
}

func (g *Codegen) genLocalVar(v *VarStatement) {
	t := g.sema.TypeOfVarDecl(v)
	if !g.checkEmittable(t, v.Token.Line, v.Token.Column, "var "+identName(v.Name)) {
		return
	}
	storage := ""
	if v.IsStatic {
		storage = "static "
	}
	if v.Value != nil {
		g.writeln("%s%s = %s;", storage, cDeclarator(t, sanitizeCIdent(v.Name.Value)), g.genInit(v.Value, t))
	} else {
		g.writeln("%s%s = %s;", storage, cDeclarator(t, sanitizeCIdent(v.Name.Value)), zeroValue(t))
	}
}

func (g *Codegen) genLocalConst(c *ConstStatement) {
	t := g.sema.TypeOfConstDecl(c)
	if !g.checkEmittable(t, c.Token.Line, c.Token.Column, "const "+identName(c.Name)) {
		return
	}
	storage := "const "
	if c.IsStatic {
		storage = "static const "
	}
	init := ""
	if c.Value != nil {
		init = " = " + g.genInit(c.Value, t)
	}
	g.writeln("%s%s%s;", storage, cDeclarator(t, sanitizeCIdent(c.Name.Value)), init)
}

func (g *Codegen) genReturn(r *ReturnStatement) {
	if r.ReturnValue == nil {
		if g.inMain {
			// `fn main() void` maps to C's `int main(void)`; a bare
			// `return;` in Tinoc source becomes `return 0;` in C, since
			// C never allows a value-less return from a non-void function.
			g.writeln("return 0;")
			return
		}
		g.writeln("return;")
		return
	}
	g.writeln("return %s;", g.genExpr(r.ReturnValue))
}

func (g *Codegen) genIf(i *IfStatement) {
	cond := ""
	if i.Condition != nil {
		cond = g.genExpr(i.Condition)
	}
	g.writeln("if (%s) {", cond)
	g.indent++
	if i.Consequence != nil {
		for _, s := range i.Consequence.Statements {
			g.genStatement(s)
		}
	}
	g.indent--

	switch alt := i.Alternative.(type) {
	case nil:
		g.writeln("}")
	case *BlockStatement:
		g.writeln("} else {")
		g.indent++
		for _, s := range alt.Statements {
			g.genStatement(s)
		}
		g.indent--
		g.writeln("}")
	case *IfStatement:
		g.writeIndent()
		g.writef("} else ")
		// genElseIf writes the nested `if (...) { ... }` chain inline,
		// continuing the "} else if (...) {" C idiom instead of nesting
		// braces, which is what Tinoc's own else-if chain example expects
		// to compile down to.
		g.genElseIf(alt)
	default:
		g.writeln("}")
	}
}

// genElseIf writes an `if` statement as the continuation of an
// `} else <here>` line already started by the caller, recursing so
// `else if / else if / else` chains of any depth render as the natural C
// `} else if (...) { ... } else if (...) { ... } else { ... }` shape
// rather than deeply nested blocks.
func (g *Codegen) genElseIf(i *IfStatement) {
	cond := ""
	if i.Condition != nil {
		cond = g.genExpr(i.Condition)
	}
	g.writef("if (%s) {\n", cond)
	g.indent++
	if i.Consequence != nil {
		for _, s := range i.Consequence.Statements {
			g.genStatement(s)
		}
	}
	g.indent--

	switch alt := i.Alternative.(type) {
	case nil:
		g.writeln("}")
	case *BlockStatement:
		g.writeln("} else {")
		g.indent++
		for _, s := range alt.Statements {
			g.genStatement(s)
		}
		g.indent--
		g.writeln("}")
	case *IfStatement:
		g.writeIndent()
		g.writef("} else ")
		g.genElseIf(alt)
	default:
		g.writeln("}")
	}
}

func (g *Codegen) genWhile(w *WhileStatement) {
	cond := "true"
	if w.Condition != nil {
		cond = g.genExpr(w.Condition)
	}
	g.writeln("while (%s) {", cond)
	g.indent++
	g.loopDepth++
	if w.Body != nil {
		for _, s := range w.Body.Statements {
			g.genStatement(s)
		}
	}
	g.loopDepth--
	g.indent--
	g.writeln("}")
}

// genFor lowers both of Tinoc's for-loop forms to C `for`:
//
//	for <start>..<end> |i| { ... }   -> for (i32 i = start; i < end; i++) { ... }
//	for <collection> |x| { ... }     -> for (size_t <idx> = 0; <idx> < <coll>.len; <idx>++) { T x = <coll>.ptr[<idx>]; ... }
//
// The collection form assumes a slice/array-with-.len-and-.ptr shape,
// matching the slice representation documented in syntax.md; since
// array/slice types aren't resolved by Sema yet in this pass, the
// collection form is accepted syntactically but flagged as unsupported
// until array/slice codegen lands, rather than emitting code that likely
// won't compile.
func (g *Codegen) genFor(f *ForStatement) {
	if f.Collection != nil {
		g.genForCollection(f)
		return
	}

	capture := "_i"
	if f.Capture != nil {
		capture = sanitizeCIdent(f.Capture.Value)
	}
	start := "0"
	if f.Start != nil {
		start = g.genExpr(f.Start)
	}
	end := "0"
	if f.End != nil {
		end = g.genExpr(f.End)
	}

	g.writeln("for (i32 %s = %s; %s < %s; %s++) {", capture, start, capture, end, capture)
	g.indent++
	g.loopDepth++
	if f.Body != nil {
		for _, s := range f.Body.Statements {
			g.genStatement(s)
		}
	}
	g.loopDepth--
	g.indent--
	g.writeln("}")
}

// genForCollection lowers `for coll |x| { ... }` to an indexed C loop
// over the collection's storage: arrays iterate by their constant
// length, slices by their runtime len field. The collection is bound to
// a local first so it is evaluated only once, and the capture variable
// is declared inside the loop with the element type.
func (g *Codegen) genForCollection(f *ForStatement) {
	collType := g.sema.TypeOf(f.Collection)
	if collType == nil || (collType.Kind != KindArray && collType.Kind != KindSlice) || collType.Elem == nil {
		g.errorAt(f.Token.Line, f.Token.Column, "codegen: cannot iterate over a value of type %s", typeStringOrInvalid(collType))
		return
	}
	capture := "x"
	if f.Capture != nil {
		capture = sanitizeCIdent(f.Capture.Value)
	}
	coll := g.genExpr(f.Collection)
	temp := "__tnoc_coll"
	idx := "__tnoc_i"
	bound := ""
	access := ""
	if collType.Kind == KindArray {
		// Iteration only reads elements (the capture is a by-value copy),
		// so a const pointer is always valid and keeps const arrays from
		// warning about discarded qualifiers.
		g.writeln("const %s* %s = %s;", collType.Elem.CType(), temp, coll)
		bound = fmt.Sprintf("%d", collType.ArraySize)
		access = fmt.Sprintf("%s[%s]", temp, idx)
	} else {
		g.writeln("%s = %s;", cDeclarator(collType, temp), coll)
		bound = fmt.Sprintf("(%s).len", temp)
		access = fmt.Sprintf("(%s).ptr[%s]", temp, idx)
	}
	g.writeln("for (i32 %s = 0; (size_t)%s < %s; %s++) {", idx, idx, bound, idx)
	g.indent++
	g.loopDepth++
	if f.Body != nil {
		g.writeln("%s = %s;", cDeclarator(collType.Elem, capture), access)
		for _, s := range f.Body.Statements {
			g.genStatement(s)
		}
	}
	g.loopDepth--
	g.indent--
	g.writeln("}")
}

// genSwitch lowers Tinoc's switch statement to a C switch, emitting each
// arm as its own braced block terminated by an explicit `break;` (no
// fallthrough, matching syntax.md's notes). Values with payload-carrying
// enum types switch on their `tag` field; every arm value (enum variant
// constant or integer literal) renders to the matching C constant via
// genExpr. Arm bodies declare in their own scope so Tinoc block scoping
// matches C's braced-case scoping.
func (g *Codegen) genSwitch(sw *SwitchStatement) {
	value := ""
	if sw.Value != nil {
		value = g.genExpr(sw.Value)
	}
	// Tagged-union enums carry their discriminant in `.tag`; the switch
	// value is that field, and arms match against the tag constants.
	// The full value is still needed to bind payload fields in pattern
	// arms (`Shape.Rect(w, h)` reads `(value).data.Rect_0` etc).
	fullValue := value
	if t := g.sema.TypeOf(sw.Value); t != nil && t.Kind == KindEnum && t.HasPayload {
		value = "(" + value + ").tag"
	}

	g.writeln("switch (%s) {", value)
	g.indent++
	for _, arm := range sw.Arms {
		if arm == nil {
			continue
		}
		if arm.Value == nil {
			g.writeln("default:")
		} else if g.genSwitchPatternArm(arm, fullValue) {
			// Pattern arm fully emitted (case label + payload bindings).
			continue
		} else {
			// Plain variant arm (`Shape.Point`) or literal. The case label
			// compares against `.tag` for tagged unions, so a fieldless
			// variant reference must emit its bare tag constant -- not the
			// full struct value genExpr would produce for a tagged union.
			g.writeln("case %s:", g.genSwitchArmLabel(arm.Value))
		}
		g.indent++
		g.writeln("{")
		g.indent++
		if arm.Body != nil {
			for _, s := range arm.Body.Statements {
				g.genStatement(s)
			}
		}
		g.writeln("break;")
		g.indent--
		g.writeln("}")
		g.indent--
	}
	// An exhaustive enum switch has no `_` arm by definition; C cannot
	// prove the switch value always matches, so a non-void enclosing
	// function would warn "control reaches end of non-void function".
	// Emit a dead-code default returning the type's zero value — sema
	// guarantees it is unreachable (every variant is matched), so it
	// never changes behavior, but it keeps the generated C warning-free.
	if g.curRetNeedsValue && g.isExhaustiveEnumSwitch(sw) {
		g.writeln("default:")
		g.indent++
		g.writeln("return %s;", cZeroValue(g.curRetC))
		g.indent--
	}
	g.indent--
	g.writeln("}")
}

// isExhaustiveEnumSwitch reports whether sw matches every variant of an
// enum value and has no `_` default arm (so control cannot fall off the
// switch at runtime). Used only to place dead-code C fallbacks.
func (g *Codegen) isExhaustiveEnumSwitch(sw *SwitchStatement) bool {
	t := g.sema.TypeOf(sw.Value)
	if t == nil || t.Kind != KindEnum {
		return false
	}
	hasDefault := false
	matched := make(map[string]bool)
	for _, arm := range sw.Arms {
		if arm == nil {
			continue
		}
		if arm.Value == nil {
			hasDefault = true
		} else if v := switchArmVariant(arm.Value); v != "" {
			matched[v] = true
		}
	}
	if hasDefault {
		return false
	}
	for _, v := range t.EnumVariants {
		if v != nil && !matched[v.Name] {
			return false
		}
	}
	return true
}

// cZeroValue renders the C zero-initializer for a C type name.
func cZeroValue(cType string) string {
	return "(" + cType + "){0}"
}

// genSwitchArmLabel renders a switch arm's case label. For enum variant
// references this is the variant's tag constant (never the full struct
// value, even for tagged unions, since the switch value is `.tag`); for
// literals it is the literal itself.
func (g *Codegen) genSwitchArmLabel(v Expression) string {
	if fa, ok := v.(*FieldAccessExpression); ok && fa.Left != nil && fa.Field != nil {
		if id, isID := fa.Left.(*Identifier); isID {
			if et, ok := g.sema.enumTypes[id.Value]; ok {
				if _, isVariant := et.EnumVariantIdx[fa.Field.Value]; isVariant {
					return sanitizeCIdent(id.Value) + "_" + sanitizeCIdent(fa.Field.Value)
				}
			}
		}
	}
	return g.genExpr(v)
}

// genSwitchPatternArm emits one enum pattern-binding arm of a switch:
// `Shape.Rect(w, h) => { ... }`. It writes the case label (the variant's
// tag constant), binds each pattern name to its payload field, and emits
// the arm body. Returns true when the arm was a pattern arm (fully
// emitted, caller must not emit anything else); false for ordinary value
// arms (plain variant refs, integer literals, `_` handled by the caller).
func (g *Codegen) genSwitchPatternArm(arm *SwitchArm, value string) bool {
	ce, ok := arm.Value.(*CallExpression)
	if !ok {
		return false
	}
	fa, ok := ce.Function.(*FieldAccessExpression)
	if !ok || fa.Left == nil || fa.Field == nil {
		return false
	}
	id, isID := fa.Left.(*Identifier)
	if !isID {
		return false
	}
	et, ok := g.sema.enumTypes[id.Value]
	if !ok {
		return false
	}
	idx, isVariant := et.EnumVariantIdx[fa.Field.Value]
	if !isVariant {
		return false
	}
	info := et.EnumVariants[idx]
	name := sanitizeCIdent(et.Name)
	variant := sanitizeCIdent(info.Name)

	g.writeln("case %s_%s:", name, variant)
	g.indent++
	g.writeln("{")
	g.indent++
	n := len(info.Types)
	if len(ce.Arguments) < n {
		n = len(ce.Arguments)
	}
	for i := 0; i < n; i++ {
		arg, isIdent := ce.Arguments[i].(*Identifier)
		if !isIdent || info.Types[i] == nil {
			continue
		}
		// `_` is a wildcard binding: it consumes the payload slot but
		// doesn't declare a local, so no unused-variable warning.
		if arg.Value == "_" {
			continue
		}
		g.writeln("%s %s = (%s).data.%s.%s_%d;",
			cTypeOrFallback(info.Types[i]), sanitizeCIdent(arg.Value), value, variant, variant, i)
	}
	if arm.Body != nil {
		for _, s := range arm.Body.Statements {
			g.genStatement(s)
		}
	}
	g.writeln("break;")
	g.indent--
	g.writeln("}")
	g.indent--
	return true
}

// === Expressions ===
//
// genExpr renders an expression to a C source fragment. It leans on
// Sema's resolved types (via g.sema.TypeOf) where the distinction matters
// for correct C (e.g. picking str-vs-numeric formatting), but for this
// pass's scope (var/const/fn) most expressions translate near
// syntactically, since Tinoc's expression grammar is deliberately
// C-like.

// genExpr renders an expression as C. Expressions Sema accepted as an
// implicit array -> slice conversion (sliceConvs) render as a
// fat-pointer compound literal wrapping the array storage; expressions
// accepted as an implicit payload -> optional coercion (optWraps) render
// wrapped in a some-value optional compound literal (or, for the null
// literal, an empty optional); everything else goes through
// genExprNoConv.
func (g *Codegen) genExpr(e Expression) string {
	if opt, ok := g.sema.optWraps[e]; ok && opt != nil && opt.Kind == KindOptional {
		if _, isNull := e.(*NullLiteral); isNull {
			return fmt.Sprintf("((%s){ .has_value = false })", optionalTypeName(opt))
		}
		return fmt.Sprintf("((%s){ .value = (%s), .has_value = true })", optionalTypeName(opt), g.genExprNoConv(e))
	}
	if g.sema.sliceConvs[e] {
		return g.genAsSlice(e)
	}
	return g.genExprNoConv(e)
}

// genExprNoConv is genExpr's main switch: every expression kind except
// the implicit array -> slice conversion (which genExpr intercepts).
func (g *Codegen) genExprNoConv(e Expression) string {
	switch ex := e.(type) {
	case *Identifier:
		return sanitizeCIdent(ex.Value)
	case *IntegerLiteral:
		return genIntegerLiteral(ex)
	case *FloatLiteral:
		// Strip underscore digit separators: the lexer keeps them
		// verbatim (1_000.5), but C11 has no underscore separators in
		// numeric literals.
		return strings.ReplaceAll(ex.Raw, "_", "")
	case *StringLiteral:
		// Unescape once, then re-emit C-escaped: the raw Tinoc text is
		// already C-valid, but %q-style double escaping would print a
		// literal \n; emitting raw risks breaking on embedded \" quotes.
		// The length passed to tinoc_str_lit is the unescaped byte count.
		u := unescapeCString(ex.Value)
		return fmt.Sprintf("tinoc_str_lit(%s, %d)", cQuote(u), len(u))
	case *CharLiteral:
		return genCharLiteral(ex)
	case *BoolLiteral:
		if ex.Value {
			return "true"
		}
		return "false"
	case *NullLiteral:
		return "NULL"
	case *PrefixExpression:
		return g.genPrefix(ex)
	case *PostfixExpression:
		return g.genPostfix(ex)
	case *InfixExpression:
		return g.genInfix(ex)
	case *AssignExpression:
		return g.genAssign(ex)
	case *CallExpression:
		return g.genCall(ex)
	case *IndexExpression:
		left := ""
		if ex.Left != nil {
			left = g.genExpr(ex.Left)
		}
		idx := ""
		if ex.Index != nil {
			idx = g.genExpr(ex.Index)
		}
		// Slices are fat pointers: `s[i]` lowers to `s.ptr[i]`. Arrays
		// (and pointers) index directly.
		if lt := g.sema.TypeOf(ex.Left); lt != nil && lt.Kind == KindSlice {
			return fmt.Sprintf("(%s).ptr[%s]", left, idx)
		}
		return fmt.Sprintf("%s[%s]", left, idx)
	case *FieldAccessExpression:
		// #importc member access: cio.EOF / cio.stdin resolve to the C
		// name itself, since the emitted #include declares it.
		if mod, ok := g.sema.moduleAlias(ex.Left); ok && ex.Field != nil {
			if c, ok := mod.Consts[ex.Field.Value]; ok {
				return c.CSymbol
			}
		}
		// Enum variant reference: `Direction.North` -> `Direction_North`
		// (the namespaced C constant emitted in the enum typedef). For a
		// tagged-union enum the value is the full struct, so a fieldless
		// variant reference becomes a compound literal with just its tag
		// set (`((Shape){ .tag = Shape_Point })`); only in switch case
		// labels (which compare against `.tag`) is the bare constant
		// wanted -- genSwitch handles that separately.
		if id, isID := ex.Left.(*Identifier); isID && ex.Field != nil {
			if et, ok := g.sema.enumTypes[id.Value]; ok {
				if _, isVariant := et.EnumVariantIdx[ex.Field.Value]; isVariant {
					constName := sanitizeCIdent(id.Value) + "_" + sanitizeCIdent(ex.Field.Value)
					if et.HasPayload {
						return fmt.Sprintf("((%s){ .tag = %s })", sanitizeCIdent(id.Value), constName)
					}
					return constName
				}
			}
		}
		// `self^.x` renders as `self->x` (the C idiom for accessing a
		// field through the pointer `self`), which is exactly the pattern
		// syntax.md's method bodies use.
		if pe, ok := ex.Left.(*PostfixExpression); ok && pe.Operator == "^" && pe.Left != nil && ex.Field != nil {
			inner := g.genExpr(pe.Left)
			return fmt.Sprintf("%s->%s", inner, sanitizeCIdent(ex.Field.Value))
		}
		// Array/slice `.len`: arrays report their declared element count
		// as a compile-time constant; slices read the runtime len field.
		if lt := g.sema.TypeOf(ex.Left); lt != nil && ex.Field != nil && ex.Field.Value == "len" {
			if lt.Kind == KindArray {
				return fmt.Sprintf("(%d)", lt.ArraySize)
			}
			if lt.Kind == KindSlice {
				// The C field is size_t; cast to i32 so user comparisons
				// like `i < s.len` do not trigger -Wsign-compare.
				return fmt.Sprintf("((i32)(%s).len)", g.genExpr(ex.Left))
			}
		}
		left := ""
		if ex.Left != nil {
			left = g.genExpr(ex.Left)
		}
		field := ""
		if ex.Field != nil {
			field = ex.Field.Value
		}
		return fmt.Sprintf("%s.%s", left, sanitizeCIdent(field))
	case *StructLiteral:
		return g.genStructLiteral(ex)
	case *ArrayLiteral:
		return g.genArrayCompound(ex)
	default:
		g.errorAt(0, 0, "codegen: unsupported expression %T", e)
		return "/* unsupported expression */ 0"
	}
}

// genAsSlice renders an implicit array -> slice conversion: the
// one-dimensional array value is wrapped in a fat-pointer compound
// literal `((tinoc_slice(T)){ .ptr = <array>, .len = N })`. Array
// literals wrap their compound-literal storage; other array values
// decay to a pointer in `.ptr`.
func (g *Codegen) genAsSlice(e Expression) string {
	arr := g.sema.TypeOf(e)
	if arr == nil || arr.Kind != KindArray || arr.Elem == nil || arr.Elem.Kind == KindArray {
		g.errorAt(0, 0, "codegen: invalid array-to-slice conversion (expected a one-dimensional array)")
		return "((tinoc_slice(i32)){ .ptr = NULL, .len = 0 })"
	}
	var inner string
	if al, ok := e.(*ArrayLiteral); ok && al != nil {
		inner = g.genArrayCompound(al)
	} else {
		inner = g.genExprNoConv(e)
	}
	return fmt.Sprintf("((%s){ .ptr = %s, .len = %d })", sliceTypeName(arr), inner, arr.ArraySize)
}

// genArrayCompound renders an array literal as a C11 compound literal
// of its resolved type, e.g. `((i32[3]){1, 2, 3})`, recursing for
// nested (multidimensional) literals and appending the sentinel element
// for `[N:x]T` types. Compound literals are used wherever an array must
// be an *expression* (initializers, call arguments, slice conversions);
// plain brace lists are only valid in initializer position.
func (g *Codegen) genArrayCompound(al *ArrayLiteral) string {
	t := g.sema.TypeOf(al)
	if t == nil || t.Kind != KindArray || t.Elem == nil {
		g.errorAt(0, 0, "codegen: array literal has no resolved array type")
		return "((i32[1]){0})"
	}
	size := t.ArraySize
	var parts []string
	for _, el := range al.Elements {
		parts = append(parts, g.genExpr(el))
	}
	for len(parts) < size {
		parts = append(parts, "0")
	}
	if t.HasSentinel {
		size++ // storage holds N+1 elements
		parts = append(parts, fmt.Sprintf("%d", t.SentinelValue))
	}
	return fmt.Sprintf("((%s[%d]){%s})", cArrayTypeSpelling(t.Elem), size, strings.Join(parts, ", "))
}

// genInit renders a var/const initializer. Array literals bound to an
// array-typed declaration use the plain brace form (valid in every
// initializer position, including block-scope const arrays where GCC
// rejects compound-literal initializers as non-constant); everything
// else (slice conversions, scalars, ...) uses the ordinary expression
// form.
func (g *Codegen) genInit(value Expression, declared *Type) string {
	if declared != nil && declared.Kind == KindArray {
		if al, ok := value.(*ArrayLiteral); ok && al != nil {
			return g.genArrayBraceInit(al)
		}
	}
	return g.genExpr(value)
}

// genArrayBraceInit renders an array literal as a plain brace
// initializer, e.g. `{1, 2, 3, 4, 0}`, recursing for nested
// (multidimensional) literals and appending the sentinel element for
// `[N:x]T` types.
func (g *Codegen) genArrayBraceInit(al *ArrayLiteral) string {
	t := g.sema.TypeOf(al)
	var parts []string
	for _, el := range al.Elements {
		if inner, ok := el.(*ArrayLiteral); ok && inner != nil {
			parts = append(parts, g.genArrayBraceInit(inner))
		} else {
			parts = append(parts, g.genExpr(el))
		}
	}
	if t != nil && t.Kind == KindArray {
		for len(parts) < t.ArraySize {
			parts = append(parts, "0")
		}
		if t.HasSentinel {
			parts = append(parts, fmt.Sprintf("%d", t.SentinelValue))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// collectSliceTypes walks every resolved type the generated C can name
// (function/method signatures, extern "C" declarations, var/const
// declarations, struct/union fields) and records any slice types so
// Generate can emit named typedefs for them (sliceTypeDefs). Recursion
// registers nested slice elements first, so typedef dependencies are
// always emitted before the typedef that uses them.
func (g *Codegen) collectSliceTypes(prog *Program) {
	g.sliceTypeDefs = nil
	seen := make(map[string]bool)
	var register func(t *Type)
	register = func(t *Type) {
		if t == nil {
			return
		}
		switch t.Kind {
		case KindArray, KindOptional:
			register(t.Elem)
		case KindSlice:
			register(t.Elem) // nested slice elements first
			name := sliceTypeName(t)
			if seen[name] {
				return
			}
			seen[name] = true
			elemC := "void"
			if t.Elem != nil {
				elemC = t.Elem.CType()
			}
			g.sliceTypeDefs = append(g.sliceTypeDefs, fmt.Sprintf("typedef tinoc_slice(%s) %s;", elemC, name))
		}
	}
	// Every var/const declaration — top-level and local — contributes
	// its declared type, so a type used only in a local declaration
	// still gets its typedef (e.g. `var s []i32` or `var o ?str` in a
	// function body with no signature mention).
	for _, t := range g.sema.declVarTypes {
		register(t)
	}
	for _, t := range g.sema.declConstTypes {
		register(t)
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *FunctionStatement:
			if s.Name != nil {
				if sym := g.sema.funcs[s.Name.Value]; sym != nil {
					for _, p := range sym.Params {
						register(p)
					}
					register(sym.ReturnType)
				}
			}
		case *ExternCFuncStatement:
			if s.Name != nil {
				if sym := g.sema.externCFuncs[s.Name.Value]; sym != nil {
					for _, p := range sym.Params {
						register(p)
					}
					register(sym.ReturnType)
				}
			}
		case *VarStatement:
			register(g.sema.TypeOfVarDecl(s))
		case *ConstStatement:
			register(g.sema.TypeOfConstDecl(s))
		case *StructStatement:
			if s.Name != nil {
				if st := g.sema.structTypes[s.Name.Value]; st != nil {
					for _, f := range st.Fields {
						register(f.Type)
					}
				}
			}
		case *UnionStatement:
			if s.Name != nil {
				if ut := g.sema.unionTypes[s.Name.Value]; ut != nil {
					for _, f := range ut.Fields {
						register(f.Type)
					}
				}
			}
		}
	}
}

func (g *Codegen) collectOptionalTypes(prog *Program) {
	g.optionalTypeDefs = nil
	seen := make(map[string]bool)
	var register func(t *Type)
	register = func(t *Type) {
		if t == nil {
			return
		}
		switch t.Kind {
		case KindArray, KindSlice, KindOptional:
			register(t.Elem) // nested element types first
		}
		if t.Kind != KindOptional {
			return
		}
		name := optionalTypeName(t)
		if seen[name] {
			return
		}
		seen[name] = true
		elemC := "void"
		if t.Elem != nil {
			elemC = t.Elem.CType()
		}
		g.optionalTypeDefs = append(g.optionalTypeDefs, fmt.Sprintf("typedef struct { %s value; bool has_value; } %s;", elemC, name))
	}
	// Every var/const declaration — top-level and local — contributes
	// its declared type, so a type used only in a local declaration
	// still gets its typedef (e.g. `var s []i32` or `var o ?str` in a
	// function body with no signature mention).
	for _, t := range g.sema.declVarTypes {
		register(t)
	}
	for _, t := range g.sema.declConstTypes {
		register(t)
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *FunctionStatement:
			if s.Name != nil {
				if sym := g.sema.funcs[s.Name.Value]; sym != nil {
					for _, p := range sym.Params {
						register(p)
					}
					register(sym.ReturnType)
				}
			}
		case *ExternCFuncStatement:
			if s.Name != nil {
				if sym := g.sema.externCFuncs[s.Name.Value]; sym != nil {
					for _, p := range sym.Params {
						register(p)
					}
					register(sym.ReturnType)
				}
			}
		case *VarStatement:
			register(g.sema.TypeOfVarDecl(s))
		case *ConstStatement:
			register(g.sema.TypeOfConstDecl(s))
		case *StructStatement:
			if s.Name != nil {
				if st := g.sema.structTypes[s.Name.Value]; st != nil {
					for _, f := range st.Fields {
						register(f.Type)
					}
				}
			}
		case *UnionStatement:
			if s.Name != nil {
				if ut := g.sema.unionTypes[s.Name.Value]; ut != nil {
					for _, f := range ut.Fields {
						register(f.Type)
					}
				}
			}
		}
	}
}

// collectOptionalTypes walks the same declarations as collectSliceTypes
// and registers a named typedef for every distinct optional type used in
// the program (`?i32` -> tnc_opt_i32, `?[]i32` -> tnc_opt_tnc_slice_i32,
// `?^str` -> tnc_opt_strp). Typedefs are emitted after slice typedefs
// (an optional payload can itself be a slice) and before enums/structs
// (struct and enum fields can be optional), mirroring the slice typedef
// pattern: a named typedef keeps every declaration of the same optional
// type identical in C.

// genIntegerLiteral re-emits an integer literal's original base/notation

// (decimal, 0x, 0o -> converted to C's 0 octal form, 0b -> converted since
// C99 has no binary literal syntax) with underscores stripped, since C
// doesn't allow underscore digit separators.
func genIntegerLiteral(lit *IntegerLiteral) string {
	raw := strings.ReplaceAll(lit.Raw, "_", "")
	switch {
	case strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X"):
		return raw // C99 hex literal syntax matches Tinoc's directly
	case strings.HasPrefix(raw, "0b") || strings.HasPrefix(raw, "0B"):
		// C99 has no binary literal syntax; emit the pre-computed decimal
		// value instead (Value was parsed from the same literal in the
		// parser).
		return fmt.Sprintf("%d", lit.Value)
	case strings.HasPrefix(raw, "0o") || strings.HasPrefix(raw, "0O"):
		return "0" + raw[2:] // C octal literals are a bare leading 0
	default:
		return raw
	}
}

func genCharLiteral(cl *CharLiteral) string {
	// Tinoc's char is a Unicode codepoint (u32 underneath, see syntax.md);
	// single ASCII-range char literals map directly to a C char constant,
	// which is sufficient for this pass's scope.
	u := unescapeCString(cl.Value)
	if len(u) == 1 {
		q := cQuote(u)
		return "'" + q[1:len(q)-1] + "'"
	}
	return "'" + cl.Value + "'"
}

// unescapeCString resolves the C-style escape sequences Tinoc's lexer
// keeps verbatim (\n, \t, \", \\, \xHH, ...) into the actual bytes.
// Unknown escapes pass through as backslash + char.
func unescapeCString(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' || i+1 >= len(raw) {
			b.WriteByte(c)
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '0':
			b.WriteByte(0)
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '\\':
			b.WriteByte('\\')
		case '\'':
			b.WriteByte('\'')
		case '"':
			b.WriteByte('"')
		case 'x':
			if i+1 < len(raw) && isHexDigit(raw[i+1]) {
				hi := hexVal(raw[i+1])
				i++
				if i+1 < len(raw) && isHexDigit(raw[i+1]) {
					b.WriteByte(byte(hi*16 + hexVal(raw[i+1])))
					i++
				} else {
					b.WriteByte(byte(hi))
				}
			} else {
				b.WriteString("\\x")
			}
		default:
			b.WriteByte('\\')
			b.WriteByte(raw[i])
		}
	}
	return b.String()
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return 0
	}
}

// cQuote renders a byte string as a C string literal, escaping only the
// characters C requires. Non-ASCII bytes pass through raw (preserving
// UTF-8) rather than being turned into \uXXXX.
func cQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		case 0:
			b.WriteString("\\0")
		case '\a':
			b.WriteString("\\a")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\v':
			b.WriteString("\\v")
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (g *Codegen) genPrefix(pe *PrefixExpression) string {
	right := ""
	if pe.Right != nil {
		right = g.genExpr(pe.Right)
	}
	switch pe.Operator {
	case "-":
		return "(-" + right + ")"
	case "!":
		return "(!" + right + ")"
	case "~":
		return "(~" + right + ")"
	case "&":
		return "(&" + right + ")"
	case "-%":
		return "(-" + right + ")" // wrapping negation: C's signed overflow on unsigned types already wraps
	default:
		g.errorAt(pe.Token.Line, pe.Token.Column, "codegen: unsupported prefix operator %q", pe.Operator)
		return right
	}
}

func (g *Codegen) genPostfix(pe *PostfixExpression) string {
	left := ""
	if pe.Left != nil {
		left = g.genExpr(pe.Left)
	}
	switch pe.Operator {
	case "^": // pointer dereference
		return "(*" + left + ")"
	case "?": // optional unwrap — the optional is a C struct, so the payload is its `value` member
		return "(" + left + ").value"
	default:
		g.errorAt(pe.Token.Line, pe.Token.Column, "codegen: unsupported postfix operator %q", pe.Operator)
		return left
	}
}

// cInfixOp maps Tinoc infix operators to their C spelling. Most pass
// through unchanged since Tinoc's core arithmetic/comparison/bitwise
// operators are deliberately C-shaped; `and`/`or` are the two that need
// translation to `&&`/`||`.
func cInfixOp(op string) (string, bool) {
	switch op {
	case "and":
		return "&&", true
	case "or":
		return "||", true
	case "+", "-", "*", "/", "%", "==", "!=", "<", ">", "<=", ">=", "&", "|", "^", "<<", ">>":
		return op, true
	default:
		return "", false
	}
}

func (g *Codegen) genInfix(ie *InfixExpression) string {
	left := ""
	if ie.Left != nil {
		left = g.genExpr(ie.Left)
	}
	right := ""
	if ie.Right != nil {
		right = g.genExpr(ie.Right)
	}

	// `a orelse b` — defaulting optional unwrap: a's payload when a has
	// one, otherwise the fallback b (C's ternary evaluates only the
	// taken branch, so the fallback is not evaluated when a is some).
	if ie.Operator == "orelse" {
		return "((" + left + ").has_value ? (" + left + ").value : (" + right + "))"
	}

	// `a == null` / `a != null` on an optional become has_value checks:
	// the optional is a C struct, not a pointer, so comparing it with
	// NULL would not compile.
	if ie.Operator == "==" || ie.Operator == "!=" {
		lt := g.sema.TypeOf(ie.Left)
		rt := g.sema.TypeOf(ie.Right)
		if (lt != nil && lt.Kind == KindOptional && isNullLiteral(ie.Right)) ||
			(rt != nil && rt.Kind == KindOptional && isNullLiteral(ie.Left)) {
			optExpr := left
			if rt != nil && rt.Kind == KindOptional {
				optExpr = right
			}
			if ie.Operator == "==" {
				return "(!(" + optExpr + ").has_value)"
			}
			return "((" + optExpr + ").has_value)"
		}
		// str is a struct in C, which cannot be compared with ==/!=; both
		// sides compare by content through the tinoc_str_eq runtime helper.
		if (lt != nil && lt.Kind == KindStr) || (rt != nil && rt.Kind == KindStr) {
			eq := fmt.Sprintf("tinoc_str_eq(%s, %s)", left, right)
			if ie.Operator == "!=" {
				return "(!" + eq + ")"
			}
			return eq
		}
	}

	if cop, ok := cInfixOp(ie.Operator); ok {
		return fmt.Sprintf("(%s %s %s)", left, cop, right)
	}

	switch ie.Operator {
	case "+%", "-%", "*%":
		// Wrapping arithmetic: tinoc.h provides tinoc_wrap_add/sub/mul
		// macros that cast through the operand's own type, matching C's
		// well-defined unsigned wraparound semantics.
		fn := map[string]string{"+%": "tinoc_wrap_add", "-%": "tinoc_wrap_sub", "*%": "tinoc_wrap_mul"}[ie.Operator]
		return fmt.Sprintf("%s(%s, %s)", fn, left, right)
	default:
		g.errorAt(ie.Token.Line, ie.Token.Column, "codegen: unsupported operator %q", ie.Operator)
		return fmt.Sprintf("(%s /* unsupported op %s */ %s)", left, ie.Operator, right)
	}
}

func (g *Codegen) genAssign(ae *AssignExpression) string {
	target := ""
	if ae.Target != nil {
		target = g.genExpr(ae.Target)
	}
	value := ""
	if ae.Value != nil {
		value = g.genExpr(ae.Value)
	}

	switch ae.Operator {
	case "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=":
		return fmt.Sprintf("(%s %s %s)", target, ae.Operator, value)
	default:
		g.errorAt(ae.Token.Line, ae.Token.Column, "codegen: unsupported assignment operator %q", ae.Operator)
		return fmt.Sprintf("(%s = %s)", target, value)
	}
}

func (g *Codegen) genCall(ce *CallExpression) string {
	// #importc module call: cio.printf(...) -> printf(...), with str
	// arguments unwrapped to their data pointer (Tinoc's str is a struct;
	// C's printf wants a const char *).
	if fa, ok := ce.Function.(*FieldAccessExpression); ok {
		if mod, isMod := g.sema.moduleAlias(fa.Left); isMod && fa.Field != nil {
			if fn, ok := mod.Funcs[fa.Field.Value]; ok {
				return g.genCCall(fn.CSymbol, ce)
			}
		}
		// Static method call / enum constructor: `Point.create(...)` ->
		// tnc_Point_create(...); `Shape.Circle(5.0)` -> a tagged-union
		// compound literal; `Shape.kindName(...)` -> tnc_Shape_kindName(...).
		if id, isID := fa.Left.(*Identifier); isID && fa.Field != nil {
			if _, isStruct := g.sema.structTypes[id.Value]; isStruct {
				return g.genStructMethodCall(id.Value, fa.Field.Value, ce, nil)
			}
			if et, isEnum := g.sema.enumTypes[id.Value]; isEnum {
				if _, isVariant := et.EnumVariantIdx[fa.Field.Value]; isVariant {
					return g.genEnumConstructor(id.Value, fa.Field.Value, ce)
				}
				return g.genEnumMethodCall(id.Value, fa.Field.Value, ce, nil)
			}
			if _, isUnion := g.sema.unionTypes[id.Value]; isUnion {
				return g.genUnionMethodCall(id.Value, fa.Field.Value, ce, nil)
			}
		}
		// Instance method call: `p.translate(...)` / `pp.method(...)` ->
		// tnc_Point_translate((&p), ...) / tnc_Point_method(pp, ...);
		// `s.kindName()` / `sp.method(...)` for enums similarly.
		if fa.Left != nil && fa.Field != nil {
			recvType := g.sema.TypeOf(fa.Left)
			switch {
			case recvType != nil && recvType.Kind == KindStruct:
				return g.genStructMethodCall(recvType.Name, fa.Field.Value, ce, fa.Left)
			case recvType != nil && recvType.Kind == KindPointer && recvType.Elem != nil && recvType.Elem.Kind == KindStruct:
				return g.genStructMethodCall(recvType.Elem.Name, fa.Field.Value, ce, fa.Left)
			case recvType != nil && recvType.Kind == KindEnum:
				return g.genEnumMethodCall(recvType.Name, fa.Field.Value, ce, fa.Left)
			case recvType != nil && recvType.Kind == KindPointer && recvType.Elem != nil && recvType.Elem.Kind == KindEnum:
				return g.genEnumMethodCall(recvType.Elem.Name, fa.Field.Value, ce, fa.Left)
			case recvType != nil && recvType.Kind == KindUnion:
				return g.genUnionMethodCall(recvType.Name, fa.Field.Value, ce, fa.Left)
			case recvType != nil && recvType.Kind == KindPointer && recvType.Elem != nil && recvType.Elem.Kind == KindUnion:
				return g.genUnionMethodCall(recvType.Elem.Name, fa.Field.Value, ce, fa.Left)
			}
		}
		g.errorAt(ce.Token.Line, ce.Token.Column, "codegen: unsupported call target (module/method calls are not yet implemented)")
		return "/* unsupported call */ 0"
	}

	ident, isIdent := ce.Function.(*Identifier)
	if !isIdent {
		g.errorAt(ce.Token.Line, ce.Token.Column, "codegen: unsupported call target (module/method calls are not yet implemented)")
		return "/* unsupported call */ 0"
	}
	if len(ce.GenericArgs) > 0 {
		g.errorAt(ce.Token.Line, ce.Token.Column, "codegen: generic calls are not yet supported (%s)", ident.Value)
	}

	// extern "C" fn: printf(...) -> printf(...) under its real C symbol.
	if sym, ok := g.sema.externCFuncs[ident.Value]; ok {
		return g.genCCall(sym.CSymbol, ce)
	}

	var args []string
	for _, a := range ce.Arguments {
		args = append(args, g.genExpr(a))
	}
	return fmt.Sprintf("%s(%s)", cFunctionName(ident.Value), strings.Join(args, ", "))
}

// genCCall renders a call to a real C function (extern "C" fn or #importc
// member), unwrapping str arguments to their data pointer and emitting
// bare string literals directly.
func (g *Codegen) genCCall(cName string, ce *CallExpression) string {
	var args []string
	for _, a := range ce.Arguments {
		args = append(args, g.genCArg(a))
	}
	return fmt.Sprintf("%s(%s)", cName, strings.Join(args, ", "))
}

// genCArg renders one argument of a real C call. A string literal becomes
// a plain C string literal (no tinoc_str_lit wrapper), and any str-typed
// expression Sema flagged for unwrapping passes its data pointer.
func (g *Codegen) genCArg(a Expression) string {
	if sl, ok := a.(*StringLiteral); ok {
		return cQuote(unescapeCString(sl.Value))
	}
	if g.sema.cStrArgs[a] {
		return "(" + g.genExpr(a) + ").data"
	}
	return g.genExpr(a)
}

// genStructLiteral renders `Point { .x = 1.0, .y = 2.0 }` as a C11
// compound literal `(Point){ .x = 1.0, .y = 2.0 }`. Sema guarantees every
// field is present and typed, so the designated initializers are emitted
// in source order as-is.
func (g *Codegen) genStructLiteral(sl *StructLiteral) string {
	t := g.sema.TypeOf(sl)
	if t == nil || t.Kind != KindStruct {
		g.errorAt(sl.Token.Line, sl.Token.Column, "codegen: struct literal has no resolved struct type")
		return "/* unsupported struct literal */ {0}"
	}
	var parts []string
	for _, f := range sl.Fields {
		if f == nil || f.Name == nil {
			continue
		}
		val := "0"
		if f.Value != nil {
			val = g.genExpr(f.Value)
		}
		parts = append(parts, fmt.Sprintf(".%s = %s", sanitizeCIdent(f.Name.Value), val))
	}
	return fmt.Sprintf("(%s){ %s }", t.CType(), strings.Join(parts, ", "))
}

// genEnumConstructor renders a tagged-union enum construction call,
// `Shape.Circle(5.0)` -> `((Shape){ .tag = Shape_Circle, .data.Circle_0 =
// 5.0 })`. Fieldless variants construct a tag-only literal.
func (g *Codegen) genEnumConstructor(enumName, variant string, ce *CallExpression) string {
	et := g.sema.enumTypes[enumName]
	if et == nil {
		g.errorAt(ce.Token.Line, ce.Token.Column, "codegen: no resolved type for enum %s", enumName)
		return "/* unsupported enum construction */ {0}"
	}
	name := sanitizeCIdent(enumName)

	idx, ok := et.EnumVariantIdx[variant]
	if !ok {
		g.errorAt(ce.Token.Line, ce.Token.Column, "codegen: enum %s has no variant %s", enumName, variant)
		return "/* unsupported enum construction */ {0}"
	}
	info := et.EnumVariants[idx]

	var parts []string
	for i, a := range ce.Arguments {
		if i >= len(info.Types) {
			break
		}
		parts = append(parts, fmt.Sprintf(".data.%s.%s_%d = %s", sanitizeCIdent(variant), sanitizeCIdent(variant), i, g.genExpr(a)))
	}
	if len(parts) > 0 {
		return fmt.Sprintf("((%s){ .tag = %s_%s, %s })", name, name, sanitizeCIdent(variant), strings.Join(parts, ", "))
	}
	return fmt.Sprintf("((%s){ .tag = %s_%s })", name, name, sanitizeCIdent(variant))
}

// genEnumMethodCall renders a call to an enum method, delegating to the
// shared genTypeMethodCall (recv is nil for static methods).
func (g *Codegen) genEnumMethodCall(enumName, method string, ce *CallExpression, recv Expression) string {
	return g.genTypeMethodCall(enumName, method, g.sema.enumMethods[enumName], ce, recv)
}

// genStructMethodCall renders a call to a struct method, delegating to the
// shared genTypeMethodCall.
func (g *Codegen) genStructMethodCall(structName, method string, ce *CallExpression, recv Expression) string {
	return g.genTypeMethodCall(structName, method, g.sema.structMethods[structName], ce, recv)
}

// genUnionMethodCall renders a call to a union method, delegating to the
// shared genTypeMethodCall (recv is nil for static methods).
func (g *Codegen) genUnionMethodCall(unionName, method string, ce *CallExpression, recv Expression) string {
	return g.genTypeMethodCall(unionName, method, g.sema.unionMethods[unionName], ce, recv)
}

// genTypeMethodCall renders a call to a struct/enum method. recv is nil
// for static methods (`Point.create(...)` -> `tnc_Point_create(...)`);
// for instance methods the receiver expression becomes the self argument:
// `p.translate(...)` -> `tnc_Point_translate((&p), ...)` when self is a
// pointer and the receiver is a struct value, or passed through directly
// when the receiver is already a pointer or self is by-value.
func (g *Codegen) genTypeMethodCall(typeName, method string, methods map[string]*Symbol, ce *CallExpression, recv Expression) string {
	sym := methods[method]
	if sym == nil {
		g.errorAt(ce.Token.Line, ce.Token.Column, "codegen: no resolved signature for %s.%s", typeName, method)
		return "/* unsupported call */ 0"
	}

	cName := "tnc_" + sanitizeCIdent(typeName) + "_" + sanitizeCIdent(method)

	var args []string
	if recv != nil {
		recvC := g.genExpr(recv)
		if len(sym.Params) > 0 {
			selfType := sym.Params[0]
			recvType := g.sema.TypeOf(recv)
			if selfType != nil && selfType.Kind == KindPointer && (recvType == nil || recvType.Kind != KindPointer) {
				recvC = "(&" + recvC + ")"
			}
		}
		args = append(args, recvC)
	}
	for _, a := range ce.Arguments {
		args = append(args, g.genExpr(a))
	}
	return fmt.Sprintf("%s(%s)", cName, strings.Join(args, ", "))
}

// === CLI Entry Point ===

// GenerateC runs the full front end (parse -> sema -> codegen) over
// source text and returns the generated C source alongside every
// diagnostic collected along the way. If any error-severity diagnostic
// was recorded at any stage, the returned C string is empty: partial/
// best-effort C for a program with real errors would be misleading to
// hand to a C compiler.
func GenerateC(file, source string) (string, *Diagnostics) {
	prog, sema, diags := RunSema(file, source)
	if diags.HasErrors() {
		return "", diags
	}

	gen := NewCodegen(sema, diags)
	gen.sourceDir = filepath.Dir(file)
	code := gen.Generate(prog)

	if diags.HasErrors() {
		return "", diags
	}
	return code, diags
}
