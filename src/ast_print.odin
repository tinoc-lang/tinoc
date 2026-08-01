package src

import "core:fmt"
import "core:strings"

// dump_tokens prints every token in source (skipping EOF) and returns
// (total_token_count, illegal_token_count).
dump_tokens :: proc(source: string, d: ^Diagnostics) -> (int, int) {
	tokens := tokenize_all(source, d)
	defer delete(tokens)

	total := 0
	illegal := 0
	for tok in tokens {
		if tok.kind == .EOF {
			continue
		}
		total += 1
		if tok.kind == .Illegal {
			illegal += 1
		}
		fmt.printf("  %-16s %-4d:%-4d  %s\n", token_kind_name(tok.kind), tok.pos.line, tok.pos.col, tok.text)
	}
	return total, illegal
}

// dump_tokens_quiet tokenizes source without printing anything, returning
// (total_token_count, illegal_token_count). Used by pipeline stages that
// only need to validate lexing before moving on.
dump_tokens_quiet :: proc(source: string, d: ^Diagnostics) -> (int, int) {
	tokens := tokenize_all(source, d)
	defer delete(tokens)

	total := 0
	illegal := 0
	for tok in tokens {
		if tok.kind == .EOF {
			continue
		}
		total += 1
		if tok.kind == .Illegal {
			illegal += 1
		}
	}
	return total, illegal
}

// dump_ast pretty-prints the parsed AST tree for a source file.
dump_ast :: proc(program: ^Program) {
	for fn in program.functions {
		print_fn_decl(fn, 0)
	}
}

print_indent :: proc(depth: int) {
	for i := 0; i < depth; i += 1 {
		fmt.print("  ")
	}
}

print_fn_decl :: proc(fn: ^Fn_Decl, depth: int) {
	print_indent(depth)
	params_sb := strings.builder_make()
	defer strings.builder_destroy(&params_sb)
	for param, i in fn.params {
		if i > 0 {
			strings.write_string(&params_sb, ", ")
		}
		strings.write_string(&params_sb, param.name)
		strings.write_string(&params_sb, " ")
		strings.write_string(&params_sb, param.type_expr.name)
	}
	fmt.printf("Fn %s(%s) %s\n", fn.name, strings.to_string(params_sb), fn.return_type.name)
	print_block(fn.body, depth + 1)
}

print_block :: proc(block: ^Block_Stmt, depth: int) {
	for stmt in block.statements {
		print_stmt(stmt, depth)
	}
}

print_stmt :: proc(stmt: Stmt, depth: int) {
	print_indent(depth)
	switch v in stmt {
	case ^Var_Decl_Stmt:
		type_str := "<inferred>"
		if te, ok := v.type_expr.?; ok {
			type_str = te.name
		}
		fmt.printf("VarDecl %s : %s\n", v.name, type_str)
		if init_expr, ok := v.init.?; ok {
			print_indent(depth + 1)
			fmt.print("init: ")
			print_expr_inline(init_expr)
			fmt.println()
		}
	case ^Const_Decl_Stmt:
		type_str := "<inferred>"
		if te, ok := v.type_expr.?; ok {
			type_str = te.name
		}
		fmt.printf("ConstDecl %s : %s\n", v.name, type_str)
		if init_expr, ok := v.init.?; ok {
			print_indent(depth + 1)
			fmt.print("init: ")
			print_expr_inline(init_expr)
			fmt.println()
		}
	case ^Return_Stmt:
		fmt.print("Return")
		if value, ok := v.value.?; ok {
			fmt.print(" ")
			print_expr_inline(value)
		}
		fmt.println()
	case ^Expr_Stmt:
		fmt.print("ExprStmt ")
		print_expr_inline(v.expr)
		fmt.println()
	case ^Assign_Stmt:
		fmt.printf("Assign %s ", v.name)
		print_expr_inline(v.value)
		fmt.println()
	case ^If_Stmt:
		fmt.print("If ")
		print_expr_inline(v.cond)
		fmt.println()
		print_block(v.then_body, depth + 1)
		if else_body, ok := v.else_body.?; ok {
			print_indent(depth)
			fmt.println("Else")
			print_block(else_body, depth + 1)
		}
	case ^Block_Stmt:
		fmt.println("Block")
		print_block(v, depth + 1)
	}
}

print_expr_inline :: proc(e: Expr) {
	switch v in e {
	case ^Ident_Expr:
		fmt.print(v.name)
	case ^Int_Lit_Expr:
		fmt.print(v.text)
	case ^Float_Lit_Expr:
		fmt.print(v.text)
	case ^String_Lit_Expr:
		fmt.printf("\"%s\"", v.value)
	case ^Char_Lit_Expr:
		fmt.printf("'%s'", v.value)
	case ^Bool_Lit_Expr:
		fmt.print(v.value)
	case ^Binary_Expr:
		fmt.print("(")
		print_expr_inline(v.left)
		fmt.printf(" %s ", binary_op_c_str(v.op))
		print_expr_inline(v.right)
		fmt.print(")")
	case ^Unary_Expr:
		fmt.print(unary_op_c_str(v.op))
		print_expr_inline(v.operand)
	case ^Call_Expr:
		fmt.printf("%s(", v.callee)
		for arg, i in v.args {
			if i > 0 {
				fmt.print(", ")
			}
			print_expr_inline(arg)
		}
		fmt.print(")")
	case ^Paren_Expr:
		fmt.print("(")
		print_expr_inline(v.inner)
		fmt.print(")")
	}
}
