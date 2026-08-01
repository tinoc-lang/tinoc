package src

// Semantic analysis for the scoped subset of Tinoc: variables, constants,
// and basic functions. This checks:
//   - redeclaration in the same scope
//   - use of undefined identifiers
//   - assignment to a const
//   - assignment/init type mismatches (using a simple inferred type model)
//   - function call arity (argument count vs parameter count)
//   - calling an undefined function
// Full type-system features (generics, structs, pointers, optionals,
// error unions, etc.) are out of scope for this pass.

Sem_Type :: enum {
	Unknown, // could not be determined; suppresses further mismatch noise
	Void,
	Bool,
	Str,
	Char,
	Int,   // any of i8..i128, u8..u128, isize, usize — widths aren't checked yet
	Float, // f32, f64, f128
}

sem_type_from_name :: proc(name: string) -> Sem_Type {
	switch name {
	case "void": return .Void
	case "bool": return .Bool
	case "str": return .Str
	case "char": return .Char
	case "i8", "i16", "i32", "i64", "i128", "isize",
	     "u8", "u16", "u32", "u64", "u128", "usize":
		return .Int
	case "f32", "f64", "f128":
		return .Float
	}
	return .Unknown
}

sem_type_name :: proc(t: Sem_Type) -> string {
	switch t {
	case .Unknown: return "unknown"
	case .Void: return "void"
	case .Bool: return "bool"
	case .Str: return "str"
	case .Char: return "char"
	case .Int: return "integer"
	case .Float: return "float"
	}
	return "unknown"
}

Symbol :: struct {
	name:       string,
	type:       Sem_Type,
	is_const:   bool,
	pos:        Pos,
}

Scope :: struct {
	symbols: map[string]Symbol,
	parent:  ^Scope,
}

scope_make :: proc(parent: ^Scope) -> ^Scope {
	s := new(Scope)
	s.symbols = make(map[string]Symbol)
	s.parent = parent
	return s
}

scope_declare :: proc(s: ^Scope, sym: Symbol, d: ^Diagnostics) -> bool {
	if _, exists := s.symbols[sym.name]; exists {
		diag_error(d, sym.pos, "'%s' is already declared in this scope", sym.name)
		return false
	}
	s.symbols[sym.name] = sym
	return true
}

scope_lookup :: proc(s: ^Scope, name: string) -> (Symbol, bool) {
	cur := s
	for cur != nil {
		if sym, ok := cur.symbols[name]; ok {
			return sym, true
		}
		cur = cur.parent
	}
	return Symbol{}, false
}

Fn_Signature :: struct {
	name:        string,
	param_types: [dynamic]Sem_Type,
	return_type: Sem_Type,
	pos:         Pos,
}

Sem_Checker :: struct {
	diags:     ^Diagnostics,
	functions: map[string]Fn_Signature,
	current_return_type: Sem_Type,
}

// check_program runs semantic analysis over the whole program, recording
// diagnostics on d. Call has_errors(d) afterward to decide whether
// codegen should proceed.
check_program :: proc(program: ^Program, d: ^Diagnostics) {
	checker := Sem_Checker{
		diags     = d,
		functions = make(map[string]Fn_Signature),
	}
	defer delete(checker.functions)

	// First pass: register all function signatures so calls can appear
	// before their definitions in source order.
	for fn in program.functions {
		sig := Fn_Signature{
			name        = fn.name,
			param_types = make([dynamic]Sem_Type),
			return_type = sem_type_from_name(fn.return_type.name),
			pos         = fn.pos,
		}
		if sig.return_type == .Unknown {
			diag_error(d, fn.return_type.pos, "unknown return type '%s'", fn.return_type.name)
		}
		for param in fn.params {
			pt := sem_type_from_name(param.type_expr.name)
			if pt == .Unknown {
				diag_error(d, param.type_expr.pos, "unknown type '%s' for parameter '%s'", param.type_expr.name, param.name)
			}
			append(&sig.param_types, pt)
		}
		if _, exists := checker.functions[fn.name]; exists {
			diag_error(d, fn.pos, "function '%s' is already declared", fn.name)
		} else {
			checker.functions[fn.name] = sig
		}
	}

	// Second pass: check each function body.
	for fn in program.functions {
		check_fn_decl(&checker, fn)
	}
}

check_fn_decl :: proc(c: ^Sem_Checker, fn: ^Fn_Decl) {
	scope := scope_make(nil)
	defer free(scope)

	c.current_return_type = sem_type_from_name(fn.return_type.name)

	for param in fn.params {
		pt := sem_type_from_name(param.type_expr.name)
		scope_declare(scope, Symbol{name = param.name, type = pt, is_const = false, pos = param.pos}, c.diags)
	}

	check_block(c, fn.body, scope, false)
}

// check_block checks all statements in a block. new_scope controls
// whether a fresh child scope is created (false for a function's
// top-level body, since parameters already live in the passed-in scope).
check_block :: proc(c: ^Sem_Checker, block: ^Block_Stmt, parent_scope: ^Scope, new_scope: bool) {
	scope := parent_scope
	if new_scope {
		scope = scope_make(parent_scope)
	}
	defer if new_scope {
		free(scope)
	}

	for stmt in block.statements {
		check_stmt(c, stmt, scope)
	}
}

check_stmt :: proc(c: ^Sem_Checker, stmt: Stmt, scope: ^Scope) {
	switch v in stmt {
	case ^Var_Decl_Stmt:
		check_var_decl(c, v, scope)
	case ^Const_Decl_Stmt:
		check_const_decl(c, v, scope)
	case ^Return_Stmt:
		check_return_stmt(c, v, scope)
	case ^Expr_Stmt:
		infer_expr_type(c, v.expr, scope)
	case ^Assign_Stmt:
		check_assign_stmt(c, v, scope)
	case ^If_Stmt:
		check_if_stmt(c, v, scope)
	case ^Block_Stmt:
		check_block(c, v, scope, true)
	}
}

check_var_decl :: proc(c: ^Sem_Checker, decl: ^Var_Decl_Stmt, scope: ^Scope) {
	declared_type := Sem_Type.Unknown
	if te, has_type := decl.type_expr.?; has_type {
		declared_type = sem_type_from_name(te.name)
		if declared_type == .Unknown {
			diag_error(c.diags, te.pos, "unknown type '%s'", te.name)
		}
	}

	init_type := Sem_Type.Unknown
	if init_expr, has_init := decl.init.?; has_init {
		init_type = infer_expr_type(c, init_expr, scope)
	} else if _, has_type := decl.type_expr.?; !has_type {
		diag_error(c.diags, decl.pos, "variable '%s' needs either a type or an initializer", decl.name)
	}

	final_type := declared_type
	if final_type == .Unknown {
		final_type = init_type
	} else if init_type != .Unknown && init_type != final_type {
		diag_error(c.diags, decl.pos, "cannot assign %s value to variable '%s' of type %s",
			sem_type_name(init_type), decl.name, sem_type_name(final_type))
	}

	scope_declare(scope, Symbol{name = decl.name, type = final_type, is_const = false, pos = decl.pos}, c.diags)
}

check_const_decl :: proc(c: ^Sem_Checker, decl: ^Const_Decl_Stmt, scope: ^Scope) {
	declared_type := Sem_Type.Unknown
	if te, has_type := decl.type_expr.?; has_type {
		declared_type = sem_type_from_name(te.name)
		if declared_type == .Unknown {
			diag_error(c.diags, te.pos, "unknown type '%s'", te.name)
		}
	}

	init_type := Sem_Type.Unknown
	if init_expr, has_init := decl.init.?; has_init {
		init_type = infer_expr_type(c, init_expr, scope)
	} else {
		diag_error(c.diags, decl.pos, "constant '%s' must be initialized", decl.name)
	}

	final_type := declared_type
	if final_type == .Unknown {
		final_type = init_type
	} else if init_type != .Unknown && init_type != final_type {
		diag_error(c.diags, decl.pos, "cannot assign %s value to constant '%s' of type %s",
			sem_type_name(init_type), decl.name, sem_type_name(final_type))
	}

	scope_declare(scope, Symbol{name = decl.name, type = final_type, is_const = true, pos = decl.pos}, c.diags)
}

check_return_stmt :: proc(c: ^Sem_Checker, stmt: ^Return_Stmt, scope: ^Scope) {
	if value, has_value := stmt.value.?; has_value {
		value_type := infer_expr_type(c, value, scope)
		if c.current_return_type == .Void {
			diag_error(c.diags, stmt.pos, "function returning void cannot return a value")
		} else if value_type != .Unknown && c.current_return_type != .Unknown && value_type != c.current_return_type {
			diag_error(c.diags, stmt.pos, "cannot return %s value, function returns %s",
				sem_type_name(value_type), sem_type_name(c.current_return_type))
		}
	} else {
		if c.current_return_type != .Void && c.current_return_type != .Unknown {
			diag_error(c.diags, stmt.pos, "function must return a %s value", sem_type_name(c.current_return_type))
		}
	}
}

check_assign_stmt :: proc(c: ^Sem_Checker, stmt: ^Assign_Stmt, scope: ^Scope) {
	sym, found := scope_lookup(scope, stmt.name)
	if !found {
		diag_error(c.diags, stmt.pos, "assignment to undefined variable '%s'", stmt.name)
	} else if sym.is_const {
		diag_error(c.diags, stmt.pos, "cannot assign to constant '%s'", stmt.name)
	}

	value_type := infer_expr_type(c, stmt.value, scope)
	if found && sym.type != .Unknown && value_type != .Unknown && sym.type != value_type {
		diag_error(c.diags, stmt.pos, "cannot assign %s value to '%s' of type %s",
			sem_type_name(value_type), stmt.name, sem_type_name(sym.type))
	}
}

check_if_stmt :: proc(c: ^Sem_Checker, stmt: ^If_Stmt, scope: ^Scope) {
	cond_type := infer_expr_type(c, stmt.cond, scope)
	if cond_type != .Unknown && cond_type != .Bool {
		diag_error(c.diags, stmt_pos_of_expr(stmt.cond), "if condition must be bool, found %s", sem_type_name(cond_type))
	}
	check_block(c, stmt.then_body, scope, true)
	if else_body, has_else := stmt.else_body.?; has_else {
		check_block(c, else_body, scope, true)
	}
}

stmt_pos_of_expr :: proc(e: Expr) -> Pos {
	return expr_pos(e)
}

// infer_expr_type walks an expression, checking identifier/call usage
// along the way, and returns its inferred type (Unknown if it can't be
// determined, e.g. due to an earlier error).
infer_expr_type :: proc(c: ^Sem_Checker, e: Expr, scope: ^Scope) -> Sem_Type {
	switch v in e {
	case ^Ident_Expr:
		if v.name == "<error>" {
			return .Unknown
		}
		sym, found := scope_lookup(scope, v.name)
		if !found {
			diag_error(c.diags, v.pos, "undefined identifier '%s'", v.name)
			return .Unknown
		}
		return sym.type

	case ^Int_Lit_Expr:
		return .Int

	case ^Float_Lit_Expr:
		return .Float

	case ^String_Lit_Expr:
		return .Str

	case ^Char_Lit_Expr:
		return .Char

	case ^Bool_Lit_Expr:
		return .Bool

	case ^Binary_Expr:
		left_type := infer_expr_type(c, v.left, scope)
		right_type := infer_expr_type(c, v.right, scope)
		return infer_binary_type(c, v, left_type, right_type)

	case ^Unary_Expr:
		operand_type := infer_expr_type(c, v.operand, scope)
		if v.op == .Not {
			if operand_type != .Unknown && operand_type != .Bool {
				diag_error(c.diags, v.pos, "'!' requires a bool operand, found %s", sem_type_name(operand_type))
			}
			return .Bool
		}
		// Neg
		if operand_type != .Unknown && operand_type != .Int && operand_type != .Float {
			diag_error(c.diags, v.pos, "unary '-' requires a numeric operand, found %s", sem_type_name(operand_type))
		}
		return operand_type

	case ^Call_Expr:
		return infer_call_type(c, v, scope)

	case ^Paren_Expr:
		return infer_expr_type(c, v.inner, scope)
	}
	return .Unknown
}

infer_binary_type :: proc(c: ^Sem_Checker, bin: ^Binary_Expr, left, right: Sem_Type) -> Sem_Type {
	#partial switch bin.op {
	case .Add, .Sub, .Mul, .Div, .Mod:
		if left == .Unknown || right == .Unknown {
			return .Unknown
		}
		if left != right {
			diag_error(c.diags, bin.pos, "type mismatch in arithmetic expression: %s vs %s", sem_type_name(left), sem_type_name(right))
			return .Unknown
		}
		if left != .Int && left != .Float {
			diag_error(c.diags, bin.pos, "arithmetic operator requires numeric operands, found %s", sem_type_name(left))
			return .Unknown
		}
		return left

	case .Eq, .Not_Eq, .Less, .Less_Eq, .Greater, .Greater_Eq:
		if left != .Unknown && right != .Unknown && left != right {
			diag_error(c.diags, bin.pos, "cannot compare %s with %s", sem_type_name(left), sem_type_name(right))
		}
		return .Bool

	case .And, .Or:
		if left != .Unknown && left != .Bool {
			diag_error(c.diags, bin.pos, "'%s' requires bool operands, found %s", bin.op == .And ? "and" : "or", sem_type_name(left))
		}
		if right != .Unknown && right != .Bool {
			diag_error(c.diags, bin.pos, "'%s' requires bool operands, found %s", bin.op == .And ? "and" : "or", sem_type_name(right))
		}
		return .Bool
	}
	return .Unknown
}

infer_call_type :: proc(c: ^Sem_Checker, call: ^Call_Expr, scope: ^Scope) -> Sem_Type {
	sig, found := c.functions[call.callee]
	if !found {
		diag_error(c.diags, call.pos, "call to undefined function '%s'", call.callee)
		// Still type-check arguments so nested errors surface too.
		for arg in call.args {
			infer_expr_type(c, arg, scope)
		}
		return .Unknown
	}

	if len(call.args) != len(sig.param_types) {
		diag_error(c.diags, call.pos, "'%s' expects %d argument(s), got %d", call.callee, len(sig.param_types), len(call.args))
	}

	limit := min(len(call.args), len(sig.param_types))
	for i in 0 ..< limit {
		arg_type := infer_expr_type(c, call.args[i], scope)
		expected := sig.param_types[i]
		if arg_type != .Unknown && expected != .Unknown && arg_type != expected {
			diag_error(c.diags, expr_pos(call.args[i]), "argument %d to '%s' should be %s, got %s",
				i + 1, call.callee, sem_type_name(expected), sem_type_name(arg_type))
		}
	}
	// Type-check any extra/missing args positions that exist beyond the
	// shorter list, so their inner identifiers are still validated.
	for i in limit ..< len(call.args) {
		infer_expr_type(c, call.args[i], scope)
	}

	return sig.return_type
}
