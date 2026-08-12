package src

// === Generic Body Type Substitution ===
//
// When a generic function or struct method is instantiated with concrete
// type arguments, its *signature* types are substituted by
// substituteFunctionTypes — but the body is cloned wholesale and its own
// type expressions still reference the template's type parameters. A
// generic fn body like
//
//	fn makePair:(K, V)(a K, b V) Pair:(K, V) {
//		return Pair:(K, V) { .first = a, .second = b };
//	}
//
// therefore failed to re-check after instantiation: `Pair:(K, V)` in the
// return statement still carried K and V, which no longer resolve.
// substituteBodyTypes walks the cloned body and replaces every reachable
// type expression (var/const declarations, struct literals, generic-call
// arguments, the generic suffix of `Pair:i32`-style expressions) from
// the same env, so instantiated bodies check exactly like the template
// author wrote them.

// substituteBodyTypes applies env to every type expression reachable
// inside a statement block (function bodies and struct-method bodies).
// Only the types are replaced — value expressions are walked so nested
// struct literals and generic calls get substituted, but identifiers,
// literals, and control flow are left intact.
func substituteBodyTypes(body *BlockStatement, env map[string]TypeExpr) {
	if body == nil {
		return
	}
	for _, st := range body.Statements {
		substituteStatementTypes(st, env)
	}
}

func substituteStatementTypes(st Statement, env map[string]TypeExpr) {
	if st == nil {
		return
	}
	switch s := st.(type) {
	case *VarStatement:
		if s.Type != nil {
			s.Type = substituteTypeExpr(s.Type, env)
		}
		if s.Value != nil {
			s.Value = substituteExprTypes(s.Value, env)
		}
	case *ConstStatement:
		if s.Type != nil {
			s.Type = substituteTypeExpr(s.Type, env)
		}
		if s.Value != nil {
			s.Value = substituteExprTypes(s.Value, env)
		}
	case *ExpressionStatement:
		if s.Expression != nil {
			s.Expression = substituteExprTypes(s.Expression, env)
		}
	case *ReturnStatement:
		if s.ReturnValue != nil {
			s.ReturnValue = substituteExprTypes(s.ReturnValue, env)
		}
	case *IfStatement:
		if s.Condition != nil {
			s.Condition = substituteExprTypes(s.Condition, env)
		}
		substituteBodyTypes(s.Consequence, env)
		if s.Alternative != nil {
			substituteStatementTypes(s.Alternative, env)
		}
	case *WhileStatement:
		if s.Condition != nil {
			s.Condition = substituteExprTypes(s.Condition, env)
		}
		substituteBodyTypes(s.Body, env)
	case *ForStatement:
		if s.Start != nil {
			s.Start = substituteExprTypes(s.Start, env)
		}
		if s.End != nil {
			s.End = substituteExprTypes(s.End, env)
		}
		if s.Collection != nil {
			s.Collection = substituteExprTypes(s.Collection, env)
		}
		substituteBodyTypes(s.Body, env)
	case *SwitchStatement:
		if s.Value != nil {
			s.Value = substituteExprTypes(s.Value, env)
		}
		for _, arm := range s.Arms {
			if arm != nil {
				substituteBodyTypes(arm.Body, env)
			}
		}
	case *BlockStatement:
		substituteBodyTypes(s, env)
	}
}

// substituteExprTypes applies env to the type expressions embedded in an
// expression tree: struct-literal types, generic-call arguments, and the
// generic suffix of a `Pair:i32`-style GenericExpression. The walk is
// destructive but only ever runs on freshly cloned generic instances.
func substituteExprTypes(e Expression, env map[string]TypeExpr) Expression {
	if e == nil {
		return nil
	}
	switch ex := e.(type) {
	case *CallExpression:
		for i, g := range ex.GenericArgs {
			ex.GenericArgs[i] = substituteTypeExpr(g, env)
		}
		if ex.Function != nil {
			ex.Function = substituteExprTypes(ex.Function, env)
		}
		for i, a := range ex.Arguments {
			ex.Arguments[i] = substituteExprTypes(a, env)
		}
	case *GenericExpression:
		for i, g := range ex.Args {
			ex.Args[i] = substituteTypeExpr(g, env)
		}
		if ex.Base != nil {
			ex.Base = substituteExprTypes(ex.Base, env)
		}
	case *StructLiteral:
		if ex.Type != nil {
			ex.Type = substituteTypeExpr(ex.Type, env)
		}
		for _, f := range ex.Fields {
			if f != nil && f.Value != nil {
				f.Value = substituteExprTypes(f.Value, env)
			}
		}
	case *ArrayLiteral:
		for i, el := range ex.Elements {
			ex.Elements[i] = substituteExprTypes(el, env)
		}
	case *PrefixExpression:
		if ex.Right != nil {
			ex.Right = substituteExprTypes(ex.Right, env)
		}
	case *PostfixExpression:
		if ex.Left != nil {
			ex.Left = substituteExprTypes(ex.Left, env)
		}
	case *InfixExpression:
		if ex.Left != nil {
			ex.Left = substituteExprTypes(ex.Left, env)
		}
		if ex.Right != nil {
			ex.Right = substituteExprTypes(ex.Right, env)
		}
	case *AssignExpression:
		if ex.Target != nil {
			ex.Target = substituteExprTypes(ex.Target, env)
		}
		if ex.Value != nil {
			ex.Value = substituteExprTypes(ex.Value, env)
		}
	case *IndexExpression:
		if ex.Left != nil {
			ex.Left = substituteExprTypes(ex.Left, env)
		}
		if ex.Index != nil {
			ex.Index = substituteExprTypes(ex.Index, env)
		}
	case *FieldAccessExpression:
		if ex.Left != nil {
			ex.Left = substituteExprTypes(ex.Left, env)
		}
	}
	return e
}
