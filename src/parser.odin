package src

// Parser is a recursive-descent parser over the token stream produced by
// the lexer. It only implements the grammar needed for top-level `fn`
// declarations containing variables, constants, basic expressions,
// `return`, plain assignment, and a minimal `if`/`else`. Anything else
// in syntax.md (structs, generics, modules, loops, switch, etc.) is
// intentionally left unimplemented for this pass.
Parser :: struct {
	tokens:  []Token,
	pos:     int,
	diags:   ^Diagnostics,
}

parser_make :: proc(tokens: []Token, diags: ^Diagnostics) -> Parser {
	return Parser{tokens = tokens, pos = 0, diags = diags}
}

p_current :: proc(p: ^Parser) -> Token {
	return p.tokens[p.pos]
}

p_peek_next :: proc(p: ^Parser) -> Token {
	if p.pos + 1 < len(p.tokens) {
		return p.tokens[p.pos + 1]
	}
	return p.tokens[len(p.tokens) - 1]
}

p_advance :: proc(p: ^Parser) -> Token {
	tok := p.tokens[p.pos]
	if p.pos < len(p.tokens) - 1 {
		p.pos += 1
	}
	return tok
}

p_check :: proc(p: ^Parser, kind: Token_Kind) -> bool {
	return p_current(p).kind == kind
}

p_match :: proc(p: ^Parser, kind: Token_Kind) -> bool {
	if p_check(p, kind) {
		p_advance(p)
		return true
	}
	return false
}

// p_expect consumes the current token if it matches kind, otherwise
// records a diagnostic and returns a zero-value token without advancing
// past EOF (so parsing can still terminate).
p_expect :: proc(p: ^Parser, kind: Token_Kind, context_msg: string) -> Token {
	if p_check(p, kind) {
		return p_advance(p)
	}
	tok := p_current(p)
	diag_error(p.diags, tok.pos, "expected %s %s, found %s", token_kind_name(kind), context_msg, token_kind_name(tok.kind))
	return tok
}

// p_synchronize skips tokens until a likely statement/declaration
// boundary, so a single parse error doesn't cascade into a wall of
// follow-on errors. It always advances past at least the current token,
// so callers can never spin in place on a token that happens to look
// like a boundary (e.g. a top-level 'var' when only 'fn' is expected).
p_synchronize :: proc(p: ^Parser) {
	if p_check(p, .EOF) {
		return
	}
	if p_check(p, .Semicolon) {
		p_advance(p)
		return
	}
	p_advance(p)

	for !p_check(p, .EOF) {
		if p_check(p, .Semicolon) {
			p_advance(p)
			return
		}
		#partial switch p_current(p).kind {
		case .Kw_Fn, .Kw_Var, .Kw_Const, .Kw_Return, .R_Brace:
			return
		}
		p_advance(p)
	}
}

// parse_program parses the whole token stream as a sequence of top-level
// function declarations.
parse_program :: proc(p: ^Parser) -> ^Program {
	program := new(Program)
	program.functions = make([dynamic]^Fn_Decl)

	for !p_check(p, .EOF) {
		if p_check(p, .Kw_Fn) {
			fn := parse_fn_decl(p)
			if fn != nil {
				append(&program.functions, fn)
			}
		} else {
			tok := p_current(p)
			diag_error(p.diags, tok.pos, "expected top-level 'fn' declaration, found %s", token_kind_name(tok.kind))
			p_synchronize(p)
		}
	}

	return program
}

parse_type_expr :: proc(p: ^Parser) -> Type_Expr {
	tok := p_expect(p, .Ident, "as a type name")
	return Type_Expr{name = tok.text, pos = tok.pos}
}

parse_fn_decl :: proc(p: ^Parser) -> ^Fn_Decl {
	fn_pos := p_current(p).pos
	p_expect(p, .Kw_Fn, "to start a function declaration")

	name_tok := p_expect(p, .Ident, "as the function name")

	// Generic function forms (`fn name:T(...)`) are recognized just
	// enough to skip past without a hard parse failure, but generics
	// themselves are out of scope for this pass.
	if p_check(p, .Colon) {
		diag_error(p.diags, p_current(p).pos, "generic functions are not supported yet")
		p_advance(p) // ':'
		if p_check(p, .Ident) {
			p_advance(p)
		} else if p_check(p, .L_Paren) {
			depth := 0
			for {
				if p_check(p, .L_Paren) {
					depth += 1
				} else if p_check(p, .R_Paren) {
					depth -= 1
					p_advance(p)
					if depth == 0 {
						break
					}
					continue
				} else if p_check(p, .EOF) {
					break
				}
				p_advance(p)
			}
		}
	}

	fn := new(Fn_Decl)
	fn.name = name_tok.text
	fn.params = make([dynamic]Param)
	fn.pos = fn_pos

	p_expect(p, .L_Paren, "to open the parameter list")
	if !p_check(p, .R_Paren) {
		for {
			param_name := p_expect(p, .Ident, "as a parameter name")
			param_type := parse_type_expr(p)
			append(&fn.params, Param{name = param_name.text, type_expr = param_type, pos = param_name.pos})
			if !p_match(p, .Comma) {
				break
			}
		}
	}
	p_expect(p, .R_Paren, "to close the parameter list")

	fn.return_type = parse_type_expr(p)
	fn.body = parse_block(p)

	return fn
}

parse_block :: proc(p: ^Parser) -> ^Block_Stmt {
	block_pos := p_current(p).pos
	p_expect(p, .L_Brace, "to open a block")

	block := new(Block_Stmt)
	block.statements = make([dynamic]Stmt)
	block.pos = block_pos

	for !p_check(p, .R_Brace) && !p_check(p, .EOF) {
		stmt := parse_stmt(p)
		if stmt != nil {
			append(&block.statements, stmt)
		}
	}
	p_expect(p, .R_Brace, "to close a block")

	return block
}

parse_stmt :: proc(p: ^Parser) -> Stmt {
	#partial switch p_current(p).kind {
	case .Kw_Var:
		return parse_var_decl(p)
	case .Kw_Const:
		return parse_const_decl(p)
	case .Kw_Return:
		return parse_return_stmt(p)
	case .Kw_If:
		return parse_if_stmt(p)
	case .L_Brace:
		return parse_block(p)
	case .Ident:
		// Distinguish `name = expr;` / `name += expr;` (assignment)
		// from a plain expression statement like a call `name(...)`.
		if is_assign_op_ahead(p) {
			return parse_assign_stmt(p)
		}
		return parse_expr_stmt(p)
	case:
		return parse_expr_stmt(p)
	}
}

is_assign_op_ahead :: proc(p: ^Parser) -> bool {
	next := p_peek_next(p)
	#partial switch next.kind {
	case .Assign, .Plus_Assign, .Minus_Assign, .Star_Assign, .Slash_Assign, .Percent_Assign:
		return true
	}
	return false
}

parse_var_decl :: proc(p: ^Parser) -> Stmt {
	decl_pos := p_current(p).pos
	p_advance(p) // 'var'

	name_tok := p_expect(p, .Ident, "as a variable name")

	decl := new(Var_Decl_Stmt)
	decl.name = name_tok.text
	decl.pos = decl_pos

	// `var x type;` / `var x type = expr;` / `var x = expr;`
	if p_check(p, .Ident) {
		decl.type_expr = parse_type_expr(p)
	}

	if p_match(p, .Assign) {
		decl.init = parse_expr(p)
	}

	p_expect(p, .Semicolon, "after variable declaration")
	return decl
}

parse_const_decl :: proc(p: ^Parser) -> Stmt {
	decl_pos := p_current(p).pos
	p_advance(p) // 'const'

	name_tok := p_expect(p, .Ident, "as a constant name")

	decl := new(Const_Decl_Stmt)
	decl.name = name_tok.text
	decl.pos = decl_pos

	if p_check(p, .Ident) {
		decl.type_expr = parse_type_expr(p)
	}

	if p_match(p, .Assign) {
		decl.init = parse_expr(p)
	}

	p_expect(p, .Semicolon, "after constant declaration")
	return decl
}

parse_return_stmt :: proc(p: ^Parser) -> Stmt {
	ret_pos := p_current(p).pos
	p_advance(p) // 'return'

	ret := new(Return_Stmt)
	ret.pos = ret_pos

	if !p_check(p, .Semicolon) {
		ret.value = parse_expr(p)
	}

	p_expect(p, .Semicolon, "after return statement")
	return ret
}

parse_if_stmt :: proc(p: ^Parser) -> Stmt {
	if_pos := p_current(p).pos
	p_advance(p) // 'if'

	stmt := new(If_Stmt)
	stmt.pos = if_pos
	stmt.cond = parse_expr(p)
	stmt.then_body = parse_block(p)

	if p_match(p, .Kw_Else) {
		if p_check(p, .Kw_If) {
			// Represent `else if` as an else-block containing a single
			// nested if statement.
			nested := parse_if_stmt(p)
			wrapper := new(Block_Stmt)
			wrapper.statements = make([dynamic]Stmt)
			append(&wrapper.statements, nested)
			wrapper.pos = stmt_pos(nested)
			stmt.else_body = wrapper
		} else {
			stmt.else_body = parse_block(p)
		}
	}

	return stmt
}

parse_assign_stmt :: proc(p: ^Parser) -> Stmt {
	name_tok := p_advance(p) // ident

	op: Assign_Op
	#partial switch p_current(p).kind {
	case .Assign:
		op = .Assign
	case .Plus_Assign:
		op = .Add_Assign
	case .Minus_Assign:
		op = .Sub_Assign
	case .Star_Assign:
		op = .Mul_Assign
	case .Slash_Assign:
		op = .Div_Assign
	case .Percent_Assign:
		op = .Mod_Assign
	case:
		op = .Assign
	}
	p_advance(p) // the assign-op token

	stmt := new(Assign_Stmt)
	stmt.name = name_tok.text
	stmt.op = op
	stmt.pos = name_tok.pos
	stmt.value = parse_expr(p)

	p_expect(p, .Semicolon, "after assignment")
	return stmt
}

parse_expr_stmt :: proc(p: ^Parser) -> Stmt {
	e := parse_expr(p)
	stmt := new(Expr_Stmt)
	stmt.expr = e
	stmt.pos = expr_pos(e)
	p_expect(p, .Semicolon, "after expression statement")
	return stmt
}

// === Expression parsing (precedence climbing) ===
//
// Only the subset of syntax.md's precedence table needed for basic
// arithmetic/comparison/logical expressions is implemented, from lowest
// to highest binding:
//   or
//   and
//   == !=
//   < > <= >=
//   + -
//   * / %
//   unary - !
//   primary (literals, identifiers, calls, parens)

parse_expr :: proc(p: ^Parser) -> Expr {
	return parse_or_expr(p)
}

parse_or_expr :: proc(p: ^Parser) -> Expr {
	left := parse_and_expr(p)
	for p_check(p, .Or_Or) {
		op_pos := p_current(p).pos
		p_advance(p)
		right := parse_and_expr(p)
		bin := new(Binary_Expr)
		bin.op = .Or
		bin.left = left
		bin.right = right
		bin.pos = op_pos
		left = bin
	}
	return left
}

parse_and_expr :: proc(p: ^Parser) -> Expr {
	left := parse_equality_expr(p)
	for p_check(p, .And_And) {
		op_pos := p_current(p).pos
		p_advance(p)
		right := parse_equality_expr(p)
		bin := new(Binary_Expr)
		bin.op = .And
		bin.left = left
		bin.right = right
		bin.pos = op_pos
		left = bin
	}
	return left
}

parse_equality_expr :: proc(p: ^Parser) -> Expr {
	left := parse_relational_expr(p)
	for p_check(p, .Eq) || p_check(p, .Not_Eq) {
		op_tok := p_advance(p)
		op := Binary_Op.Eq if op_tok.kind == .Eq else Binary_Op.Not_Eq
		right := parse_relational_expr(p)
		bin := new(Binary_Expr)
		bin.op = op
		bin.left = left
		bin.right = right
		bin.pos = op_tok.pos
		left = bin
	}
	return left
}

parse_relational_expr :: proc(p: ^Parser) -> Expr {
	left := parse_additive_expr(p)
	for {
		kind := p_current(p).kind
		op: Binary_Op
		matched := true
		#partial switch kind {
		case .Less: op = .Less
		case .Less_Eq: op = .Less_Eq
		case .Greater: op = .Greater
		case .Greater_Eq: op = .Greater_Eq
		case: matched = false
		}
		if !matched {
			break
		}
		op_tok := p_advance(p)
		right := parse_additive_expr(p)
		bin := new(Binary_Expr)
		bin.op = op
		bin.left = left
		bin.right = right
		bin.pos = op_tok.pos
		left = bin
	}
	return left
}

parse_additive_expr :: proc(p: ^Parser) -> Expr {
	left := parse_multiplicative_expr(p)
	for {
		kind := p_current(p).kind
		op: Binary_Op
		matched := true
		#partial switch kind {
		case .Plus: op = .Add
		case .Minus: op = .Sub
		case: matched = false
		}
		if !matched {
			break
		}
		op_tok := p_advance(p)
		right := parse_multiplicative_expr(p)
		bin := new(Binary_Expr)
		bin.op = op
		bin.left = left
		bin.right = right
		bin.pos = op_tok.pos
		left = bin
	}
	return left
}

parse_multiplicative_expr :: proc(p: ^Parser) -> Expr {
	left := parse_unary_expr(p)
	for {
		kind := p_current(p).kind
		op: Binary_Op
		matched := true
		#partial switch kind {
		case .Star: op = .Mul
		case .Slash: op = .Div
		case .Percent: op = .Mod
		case: matched = false
		}
		if !matched {
			break
		}
		op_tok := p_advance(p)
		right := parse_unary_expr(p)
		bin := new(Binary_Expr)
		bin.op = op
		bin.left = left
		bin.right = right
		bin.pos = op_tok.pos
		left = bin
	}
	return left
}

parse_unary_expr :: proc(p: ^Parser) -> Expr {
	if p_check(p, .Minus) || p_check(p, .Not) {
		op_tok := p_advance(p)
		op := Unary_Op.Neg if op_tok.kind == .Minus else Unary_Op.Not
		operand := parse_unary_expr(p)
		u := new(Unary_Expr)
		u.op = op
		u.operand = operand
		u.pos = op_tok.pos
		return u
	}
	return parse_primary_expr(p)
}

parse_primary_expr :: proc(p: ^Parser) -> Expr {
	tok := p_current(p)

	#partial switch tok.kind {
	case .Int_Lit:
		p_advance(p)
		lit := new(Int_Lit_Expr)
		lit.text = tok.text
		lit.pos = tok.pos
		return lit
	case .Float_Lit:
		p_advance(p)
		lit := new(Float_Lit_Expr)
		lit.text = tok.text
		lit.pos = tok.pos
		return lit
	case .String_Lit:
		p_advance(p)
		lit := new(String_Lit_Expr)
		lit.value = tok.text
		lit.pos = tok.pos
		return lit
	case .Char_Lit:
		p_advance(p)
		lit := new(Char_Lit_Expr)
		lit.value = tok.text
		lit.pos = tok.pos
		return lit
	case .Kw_True:
		p_advance(p)
		lit := new(Bool_Lit_Expr)
		lit.value = true
		lit.pos = tok.pos
		return lit
	case .Kw_False:
		p_advance(p)
		lit := new(Bool_Lit_Expr)
		lit.value = false
		lit.pos = tok.pos
		return lit
	case .L_Paren:
		p_advance(p)
		inner := parse_expr(p)
		p_expect(p, .R_Paren, "to close a parenthesized expression")
		paren := new(Paren_Expr)
		paren.inner = inner
		paren.pos = tok.pos
		return paren
	case .Ident:
		p_advance(p)
		if p_check(p, .L_Paren) {
			return parse_call_expr(p, tok)
		}
		ident := new(Ident_Expr)
		ident.name = tok.text
		ident.pos = tok.pos
		return ident
	case:
		diag_error(p.diags, tok.pos, "expected expression, found %s", token_kind_name(tok.kind))
		// Return a placeholder identifier so callers can keep going
		// without a nil expression; semantics will flag it as unknown.
		p_advance(p)
		placeholder := new(Ident_Expr)
		placeholder.name = "<error>"
		placeholder.pos = tok.pos
		return placeholder
	}
}

parse_call_expr :: proc(p: ^Parser, name_tok: Token) -> Expr {
	call := new(Call_Expr)
	call.callee = name_tok.text
	call.args = make([dynamic]Expr)
	call.pos = name_tok.pos

	p_expect(p, .L_Paren, "to open a call's argument list")
	if !p_check(p, .R_Paren) {
		for {
			arg := parse_expr(p)
			append(&call.args, arg)
			if !p_match(p, .Comma) {
				break
			}
		}
	}
	p_expect(p, .R_Paren, "to close a call's argument list")

	return call
}

// parse_source runs the full lex + parse pipeline and returns the
// resulting AST. Diagnostics are recorded on d; check has_errors(d)
// before trusting the returned program.
parse_source :: proc(source: string, d: ^Diagnostics) -> ^Program {
	tokens := tokenize_all(source, d)
	defer delete(tokens)
	p := parser_make(tokens[:], d)
	return parse_program(&p)
}
