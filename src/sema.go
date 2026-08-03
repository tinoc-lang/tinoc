package src

import (
	"fmt"
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
	// Pass 1: register every top-level function signature first, so calls
	// can appear textually before the function they call (Tinoc, like C
	// via forward declarations, allows this -- main() calling helpers
	// defined further down the file is the common case, see samples/*).
	for _, stmt := range prog.Statements {
		if fn, ok := stmt.(*FunctionStatement); ok {
			s.registerFunctionSignature(fn)
		}
	}

	// Pass 2: check bodies and top-level var/const statements in order.
	for _, stmt := range prog.Statements {
		s.checkStatement(stmt)
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
	case *BreakStatement, *ContinueStatement, *ImportStatement:
		// Nothing to resolve for this pass.
	case nil:
		// Stray/skipped statement (e.g. parser recovered from an error).
	default:
		// Struct/enum/union/switch/etc bodies aren't checked by this pass
		// yet; intentionally silent so partial programs using them don't
		// spam unrelated diagnostics for constructs Sema doesn't cover.
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

	prevFn := s.currentFn
	prevScope := s.current
	s.currentFn = sym
	s.current = newScope(s.global)

	for i, p := range fn.Params {
		if p.Name == nil {
			continue
		}
		var pt *Type
		if i < len(sym.Params) {
			pt = sym.Params[i]
		}
		s.current.Define(&Symbol{Name: p.Name.Value, Kind: SymParam, Type: pt, Mutable: false})
	}

	if fn.Body != nil {
		for _, st := range fn.Body.Statements {
			s.checkStatement(st)
		}
		if sym.ReturnType != nil && sym.ReturnType.Kind != KindVoid && !blockAlwaysReturns(fn.Body) {
			s.errorAt(fn.Token.Line, fn.Token.Column, "missing return at end of function %s", fn.Name.Value)
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
		// Module member access (io.println) and struct field access both
		// parse to this node; neither is resolved by this pass.
		if ex.Left != nil {
			s.checkExpression(ex.Left)
		}
		return &Type{Kind: KindUnknown, Name: "field"}
	case *GenericExpression:
		if ex.Base != nil {
			s.checkExpression(ex.Base)
		}
		return &Type{Kind: KindUnknown, Name: "generic"}
	case *StructLiteral:
		for _, f := range ex.Fields {
			if f.Value != nil {
				s.checkExpression(f.Value)
			}
		}
		return &Type{Kind: KindUnknown, Name: "struct"}
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
	s.errorAt(id.Token.Line, id.Token.Column, "undefined: %s", id.Value)
	return &Type{Kind: KindInvalid}
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
		s.checkOperandsCompatible(ie, lt, rt)
		return typeBool

	default: // arithmetic / bitwise
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
	ident, isIdent := ce.Function.(*Identifier)
	if !isIdent {
		// Method calls / module calls (io.println(...), self.foo(...)) are
		// FieldAccessExpression callees, not yet resolved by this pass.
		if ce.Function != nil {
			s.checkExpression(ce.Function)
		}
		for _, a := range ce.Arguments {
			s.checkExpression(a)
		}
		return &Type{Kind: KindUnknown, Name: "call"}
	}

	sym, ok := s.funcs[ident.Value]
	if !ok {
		// Also allow calling a local variable of function type is not
		// supported (Tinoc has no first-class function values in this
		// pass); treat as unknown-callee.
		for _, a := range ce.Arguments {
			s.checkExpression(a)
		}
		s.errorAt(ce.Token.Line, ce.Token.Column, "undefined: %s", ident.Value)
		return &Type{Kind: KindInvalid}
	}

	if len(ce.GenericArgs) > 0 {
		s.errorAt(ce.Token.Line, ce.Token.Column, "generic calls are not yet supported (%s)", ident.Value)
	}

	if len(ce.Arguments) != len(sym.Params) {
		s.errorAt(ce.Token.Line, ce.Token.Column, "not enough arguments in call to %s\n\thave (%d args)\n\twant (%d args)",
			ident.Value, len(ce.Arguments), len(sym.Params))
	}

	n := len(ce.Arguments)
	if len(sym.Params) < n {
		n = len(sym.Params)
	}
	for i := 0; i < n; i++ {
		argType := s.checkExpression(ce.Arguments[i])
		want := sym.Params[i]
		if want == nil || argType == nil {
			continue
		}
		if isUntypedLiteral(ce.Arguments[i]) && argType.isNumeric() && want.isNumeric() {
			s.resolvedTypes[ce.Arguments[i]] = want
			continue
		}
		if want.Kind == KindUnknown || argType.Kind == KindUnknown || argType.Kind == KindInvalid {
			continue
		}
		if !assignable(want, argType) {
			s.errorAt(ce.Token.Line, ce.Token.Column, "%s", describeMismatch(
				fmt.Sprintf("argument %d to %s", i+1, ident.Value), argType, want))
		}
	}
	// Extra arguments beyond the param count still get checked so their
	// own inner expressions are resolved (useful for codegen robustness
	// even though the call itself is already flagged above).
	for i := n; i < len(ce.Arguments); i++ {
		s.checkExpression(ce.Arguments[i])
	}

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
	if len(parseErrs) == 0 {
		sema.Check(prog)
	}

	return prog, sema, diags
}
