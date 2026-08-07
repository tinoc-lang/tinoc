package src

import (
	"bytes"
	"strings"
)

// === Core Node Interfaces ===

// Node is the base interface implemented by every AST node.
type Node interface {
	TokenLiteral() string
	String() string
}

// Statement is a node that represents a statement (produces no value).
type Statement interface {
	Node
	statementNode()
}

// Expression is a node that represents an expression (produces a value).
type Expression interface {
	Node
	expressionNode()
}

// TypeExpr is a node that represents a type annotation (e.g. `i32`, `^str`,
// `[5]u8`, `?i32`). It is intentionally kept separate from Expression since
// Tinoc's grammar treats types syntactically differently from expressions
// in most positions (var/const decls, fn params/returns, etc).
type TypeExpr interface {
	Node
	typeNode()
}

// === Program (root node) ===

// Program is the root node of every parsed Tinoc source file.
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
		out.WriteString("\n")
	}
	return out.String()
}

// === Identifiers ===

// Identifier represents a bare name reference, e.g. `x`, `add`, `io`.
type Identifier struct {
	Token Token // TOKEN_IDENT
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// === Literal Expressions ===

// IntegerLiteral represents an integer literal in any base (decimal, hex, octal, binary), e.g. `98222`, `0xff`, `0o755`, `0b11110000`, `1_000_000`.
// The raw token literal (with underscores/prefix intact) is preserved in
// Raw; Value holds the parsed numeric value once evaluated.
type IntegerLiteral struct {
	Token Token // TOKEN_INT
	Raw   string
	Value int64
}

func (il *IntegerLiteral) expressionNode()      {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return il.Raw }

// FloatLiteral represents a floating point literal, including hex float
// literals such as `0x103.70p-5`, e.g. `123.0`, `123.0E+77`.
type FloatLiteral struct {
	Token Token // TOKEN_FLOAT
	Raw   string
	Value float64
}

func (fl *FloatLiteral) expressionNode()      {}
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return fl.Raw }

// StringLiteral represents a string literal, e.g. `"Lucifer"`.
type StringLiteral struct {
	Token Token // TOKEN_STRING
	Value string
}

func (sl *StringLiteral) expressionNode()      {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string       { return "\"" + sl.Value + "\"" }

// CharLiteral represents a single character literal, e.g. `'h'`.
type CharLiteral struct {
	Token Token // TOKEN_CHAR
	Value string
}

func (cl *CharLiteral) expressionNode()      {}
func (cl *CharLiteral) TokenLiteral() string { return cl.Token.Literal }
func (cl *CharLiteral) String() string       { return "'" + cl.Value + "'" }

// BoolLiteral represents `true` or `false`.
type BoolLiteral struct {
	Token Token // TOKEN_TRUE or TOKEN_FALSE
	Value bool
}

func (bl *BoolLiteral) expressionNode()      {}
func (bl *BoolLiteral) TokenLiteral() string { return bl.Token.Literal }
func (bl *BoolLiteral) String() string       { return bl.Token.Literal }

// NullLiteral represents the `null` literal (used with optional types).
type NullLiteral struct {
	Token Token // TOKEN_NULL
}

func (nl *NullLiteral) expressionNode()      {}
func (nl *NullLiteral) TokenLiteral() string { return nl.Token.Literal }
func (nl *NullLiteral) String() string       { return "null" }

// ArrayLiteral represents an array literal, e.g. `[1, 2, 3]` or the nested
// form used for multidimensional arrays, e.g. `[[1,0],[0,1]]`.
type ArrayLiteral struct {
	Token    Token // TOKEN_LBRACK
	Elements []Expression
}

func (al *ArrayLiteral) expressionNode()      {}
func (al *ArrayLiteral) TokenLiteral() string { return al.Token.Literal }
func (al *ArrayLiteral) String() string {
	var out bytes.Buffer
	elems := make([]string, 0, len(al.Elements))
	for _, e := range al.Elements {
		elems = append(elems, e.String())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elems, ", "))
	out.WriteString("]")
	return out.String()
}

// === Other Basic Expressions ===

// PrefixExpression represents a unary prefix operator applied to an
// expression, e.g. `-x`, `!a`, `&a`, `~x`, `-%x`.
type PrefixExpression struct {
	Token    Token // the prefix operator token
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(pe.Operator)
	if pe.Right != nil {
		out.WriteString(pe.Right.String())
	}
	out.WriteString(")")
	return out.String()
}

// PostfixExpression represents a unary postfix operator, e.g. `a?` (optional
// unwrap) and `a^` (pointer dereference).
type PostfixExpression struct {
	Token    Token // the postfix operator token
	Operator string
	Left     Expression
}

func (pe *PostfixExpression) expressionNode()      {}
func (pe *PostfixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PostfixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	if pe.Left != nil {
		out.WriteString(pe.Left.String())
	}
	out.WriteString(pe.Operator)
	out.WriteString(")")
	return out.String()
}

// InfixExpression represents a binary operator expression, e.g. `a + b`,
// `a == b`, `a and b`.
type InfixExpression struct {
	Token    Token // the operator token, e.g. TOKEN_PLUS
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode()      {}
func (ie *InfixExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *InfixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	if ie.Left != nil {
		out.WriteString(ie.Left.String())
	}
	out.WriteString(" " + ie.Operator + " ")
	if ie.Right != nil {
		out.WriteString(ie.Right.String())
	}
	out.WriteString(")")
	return out.String()
}

// AssignExpression represents `a = b` and the compound-assign family
// (`+=`, `-=`, `*=`, `/=`, `%=`, `&=`, `|=`, `^=`, ...).
type AssignExpression struct {
	Token    Token // the assignment operator token
	Target   Expression
	Operator string
	Value    Expression
}

func (ae *AssignExpression) expressionNode()      {}
func (ae *AssignExpression) TokenLiteral() string { return ae.Token.Literal }
func (ae *AssignExpression) String() string {
	var out bytes.Buffer
	if ae.Target != nil {
		out.WriteString(ae.Target.String())
	}
	out.WriteString(" " + ae.Operator + " ")
	if ae.Value != nil {
		out.WriteString(ae.Value.String())
	}
	return out.String()
}

// CallExpression represents a function call, e.g. `add(10, 25)` or a
// generic call `Identity:str("Tinoc")`.
type CallExpression struct {
	Token       Token // TOKEN_LPAREN
	Function    Expression
	GenericArgs []TypeExpr // present for `ident:T(...)` / `ident:(T,U)(...)` calls
	Arguments   []Expression
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	args := make([]string, 0, len(ce.Arguments))
	for _, a := range ce.Arguments {
		args = append(args, a.String())
	}
	if ce.Function != nil {
		out.WriteString(ce.Function.String())
	}
	if len(ce.GenericArgs) > 0 {
		out.WriteString(":")
		gargs := make([]string, 0, len(ce.GenericArgs))
		for _, g := range ce.GenericArgs {
			gargs = append(gargs, g.String())
		}
		if len(ce.GenericArgs) == 1 {
			out.WriteString(gargs[0])
		} else {
			out.WriteString("(" + strings.Join(gargs, ", ") + ")")
		}
	}
	out.WriteString("(")
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	return out.String()
}

// IndexExpression represents array/slice indexing, e.g. `nums[0]`.
type IndexExpression struct {
	Token Token // TOKEN_LBRACK
	Left  Expression
	Index Expression
}

func (ie *IndexExpression) expressionNode()      {}
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	if ie.Left != nil {
		out.WriteString(ie.Left.String())
	}
	out.WriteString("[")
	if ie.Index != nil {
		out.WriteString(ie.Index.String())
	}
	out.WriteString("])")
	return out.String()
}

// FieldAccessExpression represents dotted field/module access, e.g.
// `self.x`, `io.println`, `Direction.North`.
type FieldAccessExpression struct {
	Token Token // TOKEN_DOT
	Left  Expression
	Field *Identifier
}

func (fa *FieldAccessExpression) expressionNode()      {}
func (fa *FieldAccessExpression) TokenLiteral() string { return fa.Token.Literal }
func (fa *FieldAccessExpression) String() string {
	var out bytes.Buffer
	if fa.Left != nil {
		out.WriteString(fa.Left.String())
	}
	out.WriteString(".")
	if fa.Field != nil {
		out.WriteString(fa.Field.String())
	}
	return out.String()
}

// GenericExpression represents an identifier qualified with explicit
// generic arguments used in expression position, e.g. the `Pair:i32` in
// `Pair:i32 { ... }` or the `Identity:str` in `Identity:str("Tinoc")`
// before the call parentheses are applied.
type GenericExpression struct {
	Token Token // TOKEN_COLON
	Base  Expression
	Args  []TypeExpr
}

func (ge *GenericExpression) expressionNode()      {}
func (ge *GenericExpression) TokenLiteral() string { return ge.Token.Literal }
func (ge *GenericExpression) String() string {
	var out bytes.Buffer
	if ge.Base != nil {
		out.WriteString(ge.Base.String())
	}
	out.WriteString(":")
	args := make([]string, 0, len(ge.Args))
	for _, a := range ge.Args {
		args = append(args, a.String())
	}
	if len(args) == 1 {
		out.WriteString(args[0])
	} else {
		out.WriteString("(" + strings.Join(args, ", ") + ")")
	}
	return out.String()
}

// StructLiteralField represents a single `.field = value` pair inside a
// struct literal.
type StructLiteralField struct {
	Name  *Identifier
	Value Expression
}

func (f *StructLiteralField) String() string {
	name := ""
	if f.Name != nil {
		name = f.Name.String()
	}
	val := ""
	if f.Value != nil {
		val = f.Value.String()
	}
	return "." + name + " = " + val
}

// StructLiteral represents a struct literal, e.g.
// `Point { .x = 1.0, .y = 2.0 }` or a generic instantiation form like
// `Pair:i32 { .first = 10, .second = 20 }`.
type StructLiteral struct {
	Token  Token // TOKEN_LBRACE
	Type   TypeExpr
	Fields []*StructLiteralField
}

func (sl *StructLiteral) expressionNode()      {}
func (sl *StructLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StructLiteral) String() string {
	var out bytes.Buffer
	if sl.Type != nil {
		out.WriteString(sl.Type.String())
	}
	out.WriteString(" { ")
	fields := make([]string, 0, len(sl.Fields))
	for _, f := range sl.Fields {
		fields = append(fields, f.String())
	}
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString(" }")
	return out.String()
}

// StructField is a single named field inside a struct body, e.g. the
// `x f32;` in `struct Point { x f32; y f32; }`.
type StructField struct {
	Name *Identifier
	Type TypeExpr
}

func (f *StructField) String() string {
	if f.Type == nil {
		return f.Name.String()
	}
	return f.Name.String() + " " + f.Type.String()
}

// StructStatement represents a struct declaration:
//
//	struct <identifier> {
//	    <field> <type>;
//	    ...
//	    fn <identifier>(self ^<type-of-struct>, ...) <type> {...}
//	    static fn <identifier>(<param> <type>, ...) <type> {...}
//	}
//
// Methods (including `static fn` ones, distinguished via
// FunctionStatement.IsStatic) are collected into Methods; generics
// (`struct Pair:T { ... }`) are rejected during parsing with a clear
// "not yet supported" diagnostic.
type StructStatement struct {
	Token   Token // TOKEN_STRUCT
	Name    *Identifier
	Fields  []*StructField
	Methods []*FunctionStatement
	IsPub   bool
}

func (ss *StructStatement) statementNode()       {}
func (ss *StructStatement) TokenLiteral() string { return ss.Token.Literal }
func (ss *StructStatement) String() string {
	var out bytes.Buffer
	if ss.IsPub {
		out.WriteString("pub ")
	}
	out.WriteString("struct ")
	if ss.Name != nil {
		out.WriteString(ss.Name.String())
	}
	out.WriteString(" {\n")
	for _, f := range ss.Fields {
		out.WriteString("\t")
		if f != nil {
			out.WriteString(f.String())
		}
		out.WriteString(";\n")
	}
	for _, m := range ss.Methods {
		if m == nil {
			continue
		}
		if m.IsStatic {
			out.WriteString("\tstatic ")
		}
		out.WriteString("\t")
		out.WriteString(m.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// UnionStatement represents a union declaration:
//
//	union <identifier> {
//	    <field> <type>;
//	    ...
//	    fn <identifier>(self ^<type-of-union>, ...) <type> {...}
//	    static fn <identifier>(<param> <type>, ...) <type> {...}
//	}
//
// Fields follow the same `name type;` shape as struct fields (they share
// memory at runtime, per C union semantics); methods mirror struct
// methods exactly, including `static fn`. Generic union headers
// (`union Pair:T { ... }`) are rejected during parsing with a clear
// "not yet supported" diagnostic, matching structs/enums.
type UnionStatement struct {
	Token   Token // TOKEN_UNION
	Name    *Identifier
	Fields  []*StructField
	Methods []*FunctionStatement
	IsPub   bool
}

func (us *UnionStatement) statementNode()       {}
func (us *UnionStatement) TokenLiteral() string { return us.Token.Literal }
func (us *UnionStatement) String() string {
	var out bytes.Buffer
	if us.IsPub {
		out.WriteString("pub ")
	}
	out.WriteString("union ")
	if us.Name != nil {
		out.WriteString(us.Name.String())
	}
	out.WriteString(" {\n")
	for _, f := range us.Fields {
		out.WriteString("\t")
		if f != nil {
			out.WriteString(f.String())
		}
		out.WriteString(";\n")
	}
	for _, m := range us.Methods {
		if m == nil {
			continue
		}
		if m.IsStatic {
			out.WriteString("\tstatic ")
		}
		out.WriteString("\t")
		out.WriteString(m.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// === Type Expressions ===

// NamedType represents a plain named type, e.g. `i32`, `str`, `bool`,
// `Point`, or the void return type.
type NamedType struct {
	Token Token // TOKEN_IDENT (or a primitive keyword-like ident)
	Name  string
}

func (nt *NamedType) typeNode()            {}
func (nt *NamedType) TokenLiteral() string { return nt.Token.Literal }
func (nt *NamedType) String() string       { return nt.Name }

// GenericType represents a generic instantiation, e.g. `Pair:i32`,
// `vec:i32`, `map:(K, V)`.
type GenericType struct {
	Token Token // TOKEN_COLON
	Base  string
	Args  []TypeExpr
}

func (gt *GenericType) typeNode()            {}
func (gt *GenericType) TokenLiteral() string { return gt.Token.Literal }
func (gt *GenericType) String() string {
	args := make([]string, 0, len(gt.Args))
	for _, a := range gt.Args {
		args = append(args, a.String())
	}
	if len(args) == 1 {
		return gt.Base + ":" + args[0]
	}
	return gt.Base + ":(" + strings.Join(args, ", ") + ")"
}

// PointerType represents `^T`.
type PointerType struct {
	Token Token // TOKEN_CARET
	Elem  TypeExpr
}

func (pt *PointerType) typeNode()            {}
func (pt *PointerType) TokenLiteral() string { return pt.Token.Literal }
func (pt *PointerType) String() string {
	if pt.Elem == nil {
		return "^"
	}
	return "^" + pt.Elem.String()
}

// OptionalType represents `?T`.
type OptionalType struct {
	Token Token // TOKEN_QUESTION
	Elem  TypeExpr
}

func (ot *OptionalType) typeNode()            {}
func (ot *OptionalType) TokenLiteral() string { return ot.Token.Literal }
func (ot *OptionalType) String() string {
	if ot.Elem == nil {
		return "?"
	}
	return "?" + ot.Elem.String()
}

// ErrorUnionType represents `!T` (inferred error set) or `E!T` (explicit
// error set).
type ErrorUnionType struct {
	Token  Token    // TOKEN_BANG
	ErrSet TypeExpr // nil for the inferred `!T` form
	Elem   TypeExpr
}

func (et *ErrorUnionType) typeNode()            {}
func (et *ErrorUnionType) TokenLiteral() string { return et.Token.Literal }
func (et *ErrorUnionType) String() string {
	elem := ""
	if et.Elem != nil {
		elem = et.Elem.String()
	}
	if et.ErrSet != nil {
		return et.ErrSet.String() + "!" + elem
	}
	return "!" + elem
}

// CQualType wraps a type with a C qualifier (`const`/`volatile`/`restrict`)
// in the C-facing declaration contexts (extern "C" fn params/returns and
// #importc declarations). The qualifier only matters for reconstructing a
// faithful C prototype in codegen; Tinoc's type system ignores it, so
// resolveTypeExpr simply resolves the inner element.
type CQualType struct {
	Token Token // TOKEN_CONST or TOKEN_IDENT (volatile/restrict)
	Qual  string
	Elem  TypeExpr
}

func (qt *CQualType) typeNode()            {}
func (qt *CQualType) TokenLiteral() string { return qt.Token.Literal }
func (qt *CQualType) String() string {
	if qt.Elem == nil {
		return qt.Qual
	}
	return qt.Qual + " " + qt.Elem.String()
}

// ArrayType represents fixed-size (`[N]T`), inferred-size (`[_]T`), and
// sentinel-terminated (`[N:x]T`) arrays, as well as slices (`[]T`).
type ArrayType struct {
	Token    Token      // TOKEN_LBRACK
	Size     Expression // nil for slice `[]T`
	Inferred bool       // true for `[_]T`
	Sentinel Expression // non-nil for `[N:x]T`
	Elem     TypeExpr
}

func (at *ArrayType) typeNode()            {}
func (at *ArrayType) TokenLiteral() string { return at.Token.Literal }
func (at *ArrayType) String() string {
	var out bytes.Buffer
	out.WriteString("[")
	switch {
	case at.Inferred:
		out.WriteString("_")
	case at.Size != nil:
		out.WriteString(at.Size.String())
	}
	if at.Sentinel != nil {
		out.WriteString(":")
		out.WriteString(at.Sentinel.String())
	}
	out.WriteString("]")
	if at.Elem != nil {
		out.WriteString(at.Elem.String())
	}
	return out.String()
}

// === Statements ===

// VarStatement represents a `var` declaration, with or without an explicit
// type and/or initializer:
//
//	var <identifier> <type>;
//	var <identifier> <type> = <expression>;
//	var <identifier> = <expression>;
type VarStatement struct {
	Token    Token // TOKEN_VAR
	Name     *Identifier
	Type     TypeExpr   // nil when the type is inferred
	Value    Expression // nil for decl-only
	IsStatic bool       // set for `static var`
}

func (vs *VarStatement) statementNode()       {}
func (vs *VarStatement) TokenLiteral() string { return vs.Token.Literal }
func (vs *VarStatement) String() string {
	var out bytes.Buffer
	if vs.IsStatic {
		out.WriteString("static ")
	}
	out.WriteString(vs.TokenLiteral() + " ")
	out.WriteString(vs.Name.String())
	if vs.Type != nil {
		out.WriteString(" " + vs.Type.String())
	}
	if vs.Value != nil {
		out.WriteString(" = ")
		out.WriteString(vs.Value.String())
	}
	out.WriteString(";")
	return out.String()
}

// ConstStatement represents a `const` declaration. Same shape as
// VarStatement, but the binding cannot be mutated afterward.
type ConstStatement struct {
	Token    Token // TOKEN_CONST
	Name     *Identifier
	Type     TypeExpr   // nil when the type is inferred
	Value    Expression // nil for decl-only
	IsStatic bool       // set for `static const`
}

func (cs *ConstStatement) statementNode()       {}
func (cs *ConstStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ConstStatement) String() string {
	var out bytes.Buffer
	if cs.IsStatic {
		out.WriteString("static ")
	}
	out.WriteString(cs.TokenLiteral() + " ")
	out.WriteString(cs.Name.String())
	if cs.Type != nil {
		out.WriteString(" " + cs.Type.String())
	}
	if cs.Value != nil {
		out.WriteString(" = ")
		out.WriteString(cs.Value.String())
	}
	out.WriteString(";")
	return out.String()
}

// ReturnStatement represents `return <expression>;` or a bare `return;`.
type ReturnStatement struct {
	Token       Token      // TOKEN_RETURN
	ReturnValue Expression // nil for bare `return;`
}

func (rs *ReturnStatement) statementNode()       {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString(rs.TokenLiteral())
	if rs.ReturnValue != nil {
		out.WriteString(" ")
		out.WriteString(rs.ReturnValue.String())
	}
	out.WriteString(";")
	return out.String()
}

// ExpressionStatement wraps an expression used in statement position, e.g.
// a bare call `greet();` or an assignment `name = "Julius";`.
type ExpressionStatement struct {
	Token      Token // the first token of the expression
	Expression Expression
}

func (es *ExpressionStatement) statementNode()       {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// BlockStatement represents a brace-delimited `{ ... }` block of statements.
type BlockStatement struct {
	Token      Token // TOKEN_LBRACE
	Statements []Statement
}

func (bs *BlockStatement) statementNode()       {}
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{ ")
	for _, s := range bs.Statements {
		out.WriteString(s.String())
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

// Parameter represents a single function parameter, e.g. `a i32`.
type Parameter struct {
	Name *Identifier
	Type TypeExpr
}

func (p *Parameter) String() string {
	if p.Type == nil {
		return p.Name.String()
	}
	return p.Name.String() + " " + p.Type.String()
}

// FunctionStatement represents a top-level or nested function declaration:
//
//	fn <identifier>(<param> <type>, ...) <type> {...}
//	fn <identifier>:T(<param> <type>, ...) <type> {...}
//	fn <identifier>:(T, ...)(<param> <type>, ...) <type> {...}
type FunctionStatement struct {
	Token         Token // TOKEN_FN
	Name          *Identifier
	GenericParams []string // e.g. ["T"] or ["A", "B"]; empty when non-generic
	Params        []*Parameter
	Variadic      bool // ends with `...` (only meaningful for extern "C" fns; definitions are rejected in sema)
	ReturnType    TypeExpr
	Body          *BlockStatement
	IsPub         bool
	IsStatic      bool // set for `static fn` inside struct/union/enum bodies
}

func (fs *FunctionStatement) statementNode()       {}
func (fs *FunctionStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *FunctionStatement) String() string {
	var out bytes.Buffer
	if fs.IsPub {
		out.WriteString("pub ")
	}
	if fs.IsStatic {
		out.WriteString("static ")
	}
	out.WriteString("fn ")
	out.WriteString(fs.Name.String())
	if len(fs.GenericParams) == 1 {
		out.WriteString(":" + fs.GenericParams[0])
	} else if len(fs.GenericParams) > 1 {
		out.WriteString(":(" + strings.Join(fs.GenericParams, ", ") + ")")
	}
	out.WriteString("(")
	params := make([]string, 0, len(fs.Params))
	for _, p := range fs.Params {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") ")
	if fs.ReturnType != nil {
		out.WriteString(fs.ReturnType.String())
		out.WriteString(" ")
	}
	if fs.Body != nil {
		out.WriteString(fs.Body.String())
	}
	return out.String()
}

// IfStatement represents `if <cond> { ... }`, optionally followed by
// `else { ... }` or `else if <cond> { ... }` chains (Alternative holds
// either a *BlockStatement or a nested *IfStatement).
type IfStatement struct {
	Token       Token // TOKEN_IF
	Condition   Expression
	Consequence *BlockStatement
	Alternative Statement // *BlockStatement, *IfStatement, or nil
}

func (is *IfStatement) statementNode()       {}
func (is *IfStatement) TokenLiteral() string { return is.Token.Literal }
func (is *IfStatement) String() string {
	var out bytes.Buffer
	out.WriteString("if ")
	if is.Condition != nil {
		out.WriteString(is.Condition.String())
	}
	out.WriteString(" ")
	if is.Consequence != nil {
		out.WriteString(is.Consequence.String())
	}
	if is.Alternative != nil {
		out.WriteString(" else ")
		out.WriteString(is.Alternative.String())
	}
	return out.String()
}

// WhileStatement represents `while <condition> { ... }`.
type WhileStatement struct {
	Token     Token // TOKEN_WHILE
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode()       {}
func (ws *WhileStatement) TokenLiteral() string { return ws.Token.Literal }
func (ws *WhileStatement) String() string {
	var out bytes.Buffer
	out.WriteString("while ")
	if ws.Condition != nil {
		out.WriteString(ws.Condition.String())
	}
	out.WriteString(" ")
	if ws.Body != nil {
		out.WriteString(ws.Body.String())
	}
	return out.String()
}

// ForStatement represents both range-based (`for 0..10 |i| {...}`) and
// collection-based (`for nums |n| {...}`) for-loops.
type ForStatement struct {
	Token      Token      // TOKEN_FOR
	Start      Expression // range start; nil for collection form
	End        Expression // range end; nil for collection form
	Collection Expression // iterable; nil for range form
	Capture    *Identifier
	Body       *BlockStatement
}

func (fs *ForStatement) statementNode()       {}
func (fs *ForStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for ")
	if fs.Collection != nil {
		out.WriteString(fs.Collection.String())
	} else {
		if fs.Start != nil {
			out.WriteString(fs.Start.String())
		}
		out.WriteString("..")
		if fs.End != nil {
			out.WriteString(fs.End.String())
		}
	}
	if fs.Capture != nil {
		out.WriteString(" |" + fs.Capture.String() + "| ")
	}
	if fs.Body != nil {
		out.WriteString(fs.Body.String())
	}
	return out.String()
}

// BreakStatement represents `break;`.
type BreakStatement struct {
	Token Token // TOKEN_BREAK
}

func (bs *BreakStatement) statementNode()       {}
func (bs *BreakStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BreakStatement) String() string       { return "break;" }

// ContinueStatement represents `continue;`.
type ContinueStatement struct {
	Token Token // TOKEN_CONTINUE
}

func (cs *ContinueStatement) statementNode()       {}
func (cs *ContinueStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ContinueStatement) String() string       { return "continue;" }

// ImportStatement represents `#import <module.path>;` or the wildcard form
// `#import <module.path>.*;`.
type ImportStatement struct {
	Token    Token    // TOKEN_IMPORT
	Path     []string // dotted path segments, e.g. ["std", "io"]
	Wildcard bool
}

func (is *ImportStatement) statementNode()       {}
func (is *ImportStatement) TokenLiteral() string { return is.Token.Literal }
func (is *ImportStatement) String() string {
	var out bytes.Buffer
	out.WriteString("#import ")
	out.WriteString(strings.Join(is.Path, "."))
	if is.Wildcard {
		out.WriteString(".*")
	}
	out.WriteString(";")
	return out.String()
}

// ImportCStatement represents `#importc "header.h" ["other.h"...] [as alias];`.
// It asks the compiler to parse the given C headers (via clang's JSON AST
// dump, or gcc's -aux-info fallback) and register every function, extern
// variable, enum constant, typedef, and simple macro under `alias`, so the
// rest of the file can call e.g. `cio.printf(...)` with full type safety.
// Codegen emits a matching `#include` for each header so the declarations
// resolve at C compile time.
type ImportCStatement struct {
	Token   Token    // TOKEN_IMPORTC
	Headers []string // header names exactly as written, e.g. ["stdio.h"]
	Alias   string   // namespace used for member access, e.g. "cio"
}

func (ics *ImportCStatement) statementNode()       {}
func (ics *ImportCStatement) TokenLiteral() string { return ics.Token.Literal }
func (ics *ImportCStatement) String() string {
	var out bytes.Buffer
	out.WriteString("#importc ")
	for i, h := range ics.Headers {
		if i > 0 {
			out.WriteString(" ")
		}
		out.WriteString("\"" + h + "\"")
	}
	if ics.Alias != "" {
		out.WriteString(" as " + ics.Alias)
	}
	out.WriteString(";")
	return out.String()
}

// ExternCFuncStatement represents `extern "C" fn name(.c_symbol)?(params, ...) ReturnType;`.
// It declares a C function by hand — no header parsing — so the rest of
// the file can call it by its Tinoc name with full type checking. Codegen
// emits a C prototype for it and calls it by its real C symbol.
type ExternCFuncStatement struct {
	Token      Token // TOKEN_EXTERN
	Name       *Identifier
	CSymbol    string // real C symbol; defaults to Name when the `.alias` form is unused
	Params     []*Parameter
	Variadic   bool
	ReturnType TypeExpr
}

func (ecs *ExternCFuncStatement) statementNode()       {}
func (ecs *ExternCFuncStatement) TokenLiteral() string { return ecs.Token.Literal }
func (ecs *ExternCFuncStatement) String() string {
	var out bytes.Buffer
	out.WriteString("extern \"C\" fn ")
	out.WriteString(ecs.Name.String())
	if ecs.CSymbol != "" && ecs.CSymbol != ecs.Name.Value {
		out.WriteString("." + ecs.CSymbol)
	}
	out.WriteString("(")
	params := make([]string, 0, len(ecs.Params))
	for _, p := range ecs.Params {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	if ecs.Variadic {
		if len(params) > 0 {
			out.WriteString(", ")
		}
		out.WriteString("...")
	}
	out.WriteString(") ")
	if ecs.ReturnType != nil {
		out.WriteString(ecs.ReturnType.String())
	}
	out.WriteString(";")
	return out.String()
}

// === Enums ===

// EnumVariant is a single variant of an enum declaration, e.g. `North`
// (fieldless) or `Circle(f32)` (with a payload type list).
type EnumVariant struct {
	Name  *Identifier
	Types []TypeExpr // payload types, e.g. [f32] for `Circle(f32)`; nil for fieldless
}

func (ev *EnumVariant) String() string {
	if ev == nil || ev.Name == nil {
		return ""
	}
	if len(ev.Types) == 0 {
		return ev.Name.String()
	}
	types := make([]string, 0, len(ev.Types))
	for _, t := range ev.Types {
		types = append(types, t.String())
	}
	return ev.Name.String() + "(" + strings.Join(types, ", ") + ")"
}

// EnumStatement represents an enum declaration:
//
//	enum <identifier> {
//	    <Variant>,
//	    <Variant>(<type>, ...),
//	    ...
//	    fn <identifier>(self ^<type-of-enum>, ...) <type> {...}
//	    static fn <identifier>(...) <type> {...}
//	}
//
// Variants with payload types (e.g. `Circle(f32)`) become tagged-union
// values in C; fieldless variants are plain C enum constants. Methods
// (including `static fn` ones, distinguished via
// FunctionStatement.IsStatic) are collected into Methods. Generics
// (`enum Something:T { ... }`) are rejected during parsing with a clear
// "not yet supported" diagnostic.
type EnumStatement struct {
	Token    Token // TOKEN_ENUM
	Name     *Identifier
	Variants []*EnumVariant
	Methods  []*FunctionStatement
	IsPub    bool
}

func (es *EnumStatement) statementNode()       {}
func (es *EnumStatement) TokenLiteral() string { return es.Token.Literal }
func (es *EnumStatement) String() string {
	var out bytes.Buffer
	if es.IsPub {
		out.WriteString("pub ")
	}
	out.WriteString("enum ")
	if es.Name != nil {
		out.WriteString(es.Name.String())
	}
	out.WriteString(" {\n")
	for _, v := range es.Variants {
		if v == nil {
			continue
		}
		out.WriteString("\t")
		out.WriteString(v.String())
		out.WriteString(",\n")
	}
	for _, m := range es.Methods {
		if m == nil {
			continue
		}
		if m.IsStatic {
			out.WriteString("\tstatic ")
		}
		out.WriteString("\t")
		out.WriteString(m.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// === Switch ===

// SwitchArm is one `=> { ... }` arm of a switch statement. Value is nil
// for the default arm (`_ => { ... }`).
type SwitchArm struct {
	Value Expression // nil for the `_` default arm
	Body  *BlockStatement
}

func (sa *SwitchArm) String() string {
	var out bytes.Buffer
	if sa.Value != nil {
		out.WriteString(sa.Value.String())
	} else {
		out.WriteString("_")
	}
	out.WriteString(" => ")
	if sa.Body != nil {
		out.WriteString(sa.Body.String())
	}
	return out.String()
}

// SwitchStatement represents Tinoc's switch statement:
//
//	switch <expression> {
//	    <value> => { ... }
//	    <value> => { ... }
//	    _       => { ... }
//	}
//
// Values are integer/char literals or enum variant references
// (`Direction.North`); `_` is the default arm. No fallthrough: each arm
// is independent, matching syntax.md's notes.
type SwitchStatement struct {
	Token Token // TOKEN_SWITCH
	Value Expression
	Arms  []*SwitchArm
}

func (ss *SwitchStatement) statementNode()       {}
func (ss *SwitchStatement) TokenLiteral() string { return ss.Token.Literal }
func (ss *SwitchStatement) String() string {
	var out bytes.Buffer
	out.WriteString("switch ")
	if ss.Value != nil {
		out.WriteString(ss.Value.String())
	}
	out.WriteString(" {\n")
	for _, arm := range ss.Arms {
		if arm == nil {
			continue
		}
		out.WriteString("\t")
		out.WriteString(arm.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}
