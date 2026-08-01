package src

// This AST intentionally covers only what's needed for variables,
// constants, and basic functions per syntax.md: no structs, enums,
// generics, modules, pointers, arrays, or control flow beyond a plain
// `if`/`else` and `return`. Those are left for later passes.

Type_Expr :: struct {
	name: string, // e.g. "i32", "str", "bool", "void"
	pos:  Pos,
}

// Expr is the tagged union of every expression node in scope.
Expr :: union {
	^Ident_Expr,
	^Int_Lit_Expr,
	^Float_Lit_Expr,
	^String_Lit_Expr,
	^Char_Lit_Expr,
	^Bool_Lit_Expr,
	^Binary_Expr,
	^Unary_Expr,
	^Call_Expr,
	^Paren_Expr,
}

Ident_Expr :: struct {
	name: string,
	pos:  Pos,
}

Int_Lit_Expr :: struct {
	text: string, // raw text, still containing underscores/prefix
	pos:  Pos,
}

Float_Lit_Expr :: struct {
	text: string,
	pos:  Pos,
}

String_Lit_Expr :: struct {
	value: string, // already unescaped by the lexer
	pos:   Pos,
}

Char_Lit_Expr :: struct {
	value: string, // already unescaped by the lexer
	pos:   Pos,
}

Bool_Lit_Expr :: struct {
	value: bool,
	pos:   Pos,
}

Binary_Op :: enum {
	Add,
	Sub,
	Mul,
	Div,
	Mod,
	Eq,
	Not_Eq,
	Less,
	Less_Eq,
	Greater,
	Greater_Eq,
	And,
	Or,
}

Binary_Expr :: struct {
	op:    Binary_Op,
	left:  Expr,
	right: Expr,
	pos:   Pos,
}

Unary_Op :: enum {
	Neg,
	Not,
}

Unary_Expr :: struct {
	op:      Unary_Op,
	operand: Expr,
	pos:     Pos,
}

Call_Expr :: struct {
	callee: string,
	args:   [dynamic]Expr,
	pos:    Pos,
}

Paren_Expr :: struct {
	inner: Expr,
	pos:   Pos,
}

// Stmt is the tagged union of every statement node in scope.
Stmt :: union {
	^Var_Decl_Stmt,
	^Const_Decl_Stmt,
	^Return_Stmt,
	^Expr_Stmt,
	^Assign_Stmt,
	^If_Stmt,
	^Block_Stmt,
}

Var_Decl_Stmt :: struct {
	name:        string,
	type_expr:   Maybe(Type_Expr), // absent means inferred
	init:        Maybe(Expr),      // absent means decl-only
	pos:         Pos,
}

Const_Decl_Stmt :: struct {
	name:      string,
	type_expr: Maybe(Type_Expr),
	init:      Maybe(Expr), // constants generally require init; enforced in semantics
	pos:       Pos,
}

Return_Stmt :: struct {
	value: Maybe(Expr),
	pos:   Pos,
}

Expr_Stmt :: struct {
	expr: Expr,
	pos:  Pos,
}

Assign_Op :: enum {
	Assign,
	Add_Assign,
	Sub_Assign,
	Mul_Assign,
	Div_Assign,
	Mod_Assign,
}

Assign_Stmt :: struct {
	name: string,
	op:   Assign_Op,
	value: Expr,
	pos:  Pos,
}

If_Stmt :: struct {
	cond:      Expr,
	then_body: ^Block_Stmt,
	else_body: Maybe(^Block_Stmt), // else-if is represented as a block containing a single If_Stmt
	pos:       Pos,
}

Block_Stmt :: struct {
	statements: [dynamic]Stmt,
	pos:        Pos,
}

Param :: struct {
	name:      string,
	type_expr: Type_Expr,
	pos:       Pos,
}

Fn_Decl :: struct {
	name:        string,
	params:      [dynamic]Param,
	return_type: Type_Expr,
	body:        ^Block_Stmt,
	pos:         Pos,
}

// Program is the root AST node: a flat list of top-level function
// declarations. Modules, structs, and other top-level forms are out of
// scope for this pass.
Program :: struct {
	functions: [dynamic]^Fn_Decl,
}

expr_pos :: proc(e: Expr) -> Pos {
	switch v in e {
	case ^Ident_Expr: return v.pos
	case ^Int_Lit_Expr: return v.pos
	case ^Float_Lit_Expr: return v.pos
	case ^String_Lit_Expr: return v.pos
	case ^Char_Lit_Expr: return v.pos
	case ^Bool_Lit_Expr: return v.pos
	case ^Binary_Expr: return v.pos
	case ^Unary_Expr: return v.pos
	case ^Call_Expr: return v.pos
	case ^Paren_Expr: return v.pos
	}
	return Pos{}
}

stmt_pos :: proc(s: Stmt) -> Pos {
	switch v in s {
	case ^Var_Decl_Stmt: return v.pos
	case ^Const_Decl_Stmt: return v.pos
	case ^Return_Stmt: return v.pos
	case ^Expr_Stmt: return v.pos
	case ^Assign_Stmt: return v.pos
	case ^If_Stmt: return v.pos
	case ^Block_Stmt: return v.pos
	}
	return Pos{}
}
