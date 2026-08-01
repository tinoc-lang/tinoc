package src

import (
	"fmt"
	"strconv"
	"strings"
)

// === Precedence Table ===
//
// Mirrors (a subset of) the precedence line from syntax.md:
//
//   x() x[] x.y x^ x? a!b x{} !x -x -%x ~x &x ?x
//   * / % ** *% *| || + - ++ +% -% +| -| << >> <<|
//   & ^ | orelse catch == != < > <= >= and or
//   = *= *%= *|= /= %= += +%= +|= -= -%= -|= <<= <<|= >>= &= ^= |=
//
// Only the operators the lexer currently emits are wired up; the rest are
// reserved slots for when the lexer grows support for them (`**`, `||` as
// error-set-merge as opposed to logical or, `++`, etc).

type precedence int

const (
	_ precedence = iota
	LOWEST
	ASSIGN      // = += -= *= /= %= &= |= ^=
	LOGICAL_OR  // or
	LOGICAL_AND // and
	EQUALS      // == !=
	LESSGREATER // < > <= >=
	ORELSE      // orelse catch
	BITWISE     // & ^ |
	SHIFT       // << >>
	SUM         // + -
	PRODUCT     // * / %
	PREFIX      // -x !x ~x &x ?x
	POSTFIX     // x^ x?
	CALL        // x() x[] x.y
)

var precedences = map[TokenType]precedence{
	TOKEN_ASSIGN:       ASSIGN,
	TOKEN_PLUS_ASSIGN:  ASSIGN,
	TOKEN_MINUS_ASSIGN: ASSIGN,
	TOKEN_MUL_ASSIGN:   ASSIGN,
	TOKEN_DIV_ASSIGN:   ASSIGN,
	TOKEN_MOD_ASSIGN:   ASSIGN,
	TOKEN_AMP_ASSIGN:   ASSIGN,
	TOKEN_PIPE_ASSIGN:  ASSIGN,
	TOKEN_CARET_ASSIGN: ASSIGN,

	TOKEN_OR:  LOGICAL_OR,
	TOKEN_AND: LOGICAL_AND,

	TOKEN_EQ:     EQUALS,
	TOKEN_NOT_EQ: EQUALS,

	TOKEN_LT:  LESSGREATER,
	TOKEN_GT:  LESSGREATER,
	TOKEN_LTE: LESSGREATER,
	TOKEN_GTE: LESSGREATER,

	TOKEN_ORELSE: ORELSE,
	TOKEN_CATCH:  ORELSE,

	TOKEN_AMP:   BITWISE,
	TOKEN_PIPE:  BITWISE,
	TOKEN_CARET: BITWISE, // Note: `^` as infix is bitwise-xor-ish usage is
	// reserved; as postfix it is pointer-deref (handled separately below).

	TOKEN_LSHIFT: SHIFT,
	TOKEN_RSHIFT: SHIFT,

	TOKEN_PLUS:          SUM,
	TOKEN_MINUS:         SUM,
	TOKEN_PLUS_PERCENT:  SUM,
	TOKEN_PLUS_PIPE:     SUM,
	TOKEN_MINUS_PERCENT: SUM,
	TOKEN_MINUS_PIPE:    SUM,

	TOKEN_ASTERISK:    PRODUCT,
	TOKEN_SLASH:       PRODUCT,
	TOKEN_PERCENT:     PRODUCT,
	TOKEN_MUL_PERCENT: PRODUCT,
	TOKEN_MUL_PIPE:    PRODUCT,

	TOKEN_LPAREN: CALL,
	TOKEN_LBRACK: CALL,
	TOKEN_DOT:    CALL,
	TOKEN_COLON:  CALL, // generic call: ident:T(...)
	TOKEN_LBRACE: CALL, // struct literal: Type { .field = value, ... }
}

// === Parser ===

type (
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression
)

// ParseError carries a positioned parser diagnostic.
type ParseError struct {
	Message string
	Line    int
	Column  int
}

func (e *ParseError) String() string {
	return fmt.Sprintf("L%d:C%d: %s", e.Line, e.Column, e.Message)
}

// Parser builds an AST by walking the token stream produced by the Lexer.
type Parser struct {
	l *Lexer

	curToken  Token
	peekToken Token

	errors []*ParseError

	prefixParseFns map[TokenType]prefixParseFn
	infixParseFns  map[TokenType]infixParseFn

	// noStructLiterals suppresses the `{` infix (struct literal) parser
	// while parsing if/while/for headers, where `{` unambiguously starts
	// the body block instead. Mirrors the same disambiguation Go and Zig
	// apply to brace-headed conditions.
	noStructLiterals bool
}

// NewParser constructs a Parser over the given source text.
func NewParser(source string) *Parser {
	p := &Parser{
		l:      New(source),
		errors: []*ParseError{},
	}

	p.prefixParseFns = make(map[TokenType]prefixParseFn)
	p.registerPrefix(TOKEN_IDENT, p.parseIdentifier)
	p.registerPrefix(TOKEN_INT, p.parseIntegerLiteral)
	p.registerPrefix(TOKEN_FLOAT, p.parseFloatLiteral)
	p.registerPrefix(TOKEN_STRING, p.parseStringLiteral)
	p.registerPrefix(TOKEN_CHAR, p.parseCharLiteral)
	p.registerPrefix(TOKEN_TRUE, p.parseBoolLiteral)
	p.registerPrefix(TOKEN_FALSE, p.parseBoolLiteral)
	p.registerPrefix(TOKEN_NULL, p.parseNullLiteral)
	p.registerPrefix(TOKEN_LPAREN, p.parseGroupedExpression)
	p.registerPrefix(TOKEN_LBRACK, p.parseArrayLiteral)
	p.registerPrefix(TOKEN_MINUS, p.parsePrefixExpression)
	p.registerPrefix(TOKEN_BANG, p.parsePrefixExpression)
	p.registerPrefix(TOKEN_TILDE, p.parsePrefixExpression)
	p.registerPrefix(TOKEN_AMP, p.parsePrefixExpression)
	p.registerPrefix(TOKEN_QUESTION, p.parsePrefixExpression)
	p.registerPrefix(TOKEN_MINUS_PERCENT, p.parsePrefixExpression)

	p.infixParseFns = make(map[TokenType]infixParseFn)
	for _, tt := range []TokenType{
		TOKEN_PLUS, TOKEN_MINUS, TOKEN_ASTERISK, TOKEN_SLASH, TOKEN_PERCENT,
		TOKEN_PLUS_PERCENT, TOKEN_PLUS_PIPE, TOKEN_MINUS_PERCENT, TOKEN_MINUS_PIPE,
		TOKEN_MUL_PERCENT, TOKEN_MUL_PIPE,
		TOKEN_EQ, TOKEN_NOT_EQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
		TOKEN_AND, TOKEN_OR, TOKEN_ORELSE, TOKEN_CATCH,
		TOKEN_AMP, TOKEN_PIPE, TOKEN_LSHIFT, TOKEN_RSHIFT,
	} {
		p.registerInfix(tt, p.parseInfixExpression)
	}
	for _, tt := range []TokenType{
		TOKEN_ASSIGN, TOKEN_PLUS_ASSIGN, TOKEN_MINUS_ASSIGN, TOKEN_MUL_ASSIGN,
		TOKEN_DIV_ASSIGN, TOKEN_MOD_ASSIGN, TOKEN_AMP_ASSIGN, TOKEN_PIPE_ASSIGN,
		TOKEN_CARET_ASSIGN,
	} {
		p.registerInfix(tt, p.parseAssignExpression)
	}
	p.registerInfix(TOKEN_LPAREN, p.parseCallExpression)
	p.registerInfix(TOKEN_LBRACK, p.parseIndexExpression)
	p.registerInfix(TOKEN_DOT, p.parseFieldAccessExpression)
	p.registerInfix(TOKEN_CARET, p.parsePointerDerefPostfix)
	p.registerInfix(TOKEN_QUESTION, p.parseOptionalUnwrapPostfix)
	p.registerInfix(TOKEN_LBRACE, p.parseStructLiteral)
	p.registerInfix(TOKEN_COLON, p.parseGenericSuffix)

	// Prime curToken/peekToken.
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) registerPrefix(tt TokenType, fn prefixParseFn) { p.prefixParseFns[tt] = fn }
func (p *Parser) registerInfix(tt TokenType, fn infixParseFn)   { p.infixParseFns[tt] = fn }

// Errors returns all diagnostics collected while parsing.
func (p *Parser) Errors() []*ParseError { return p.errors }

func (p *Parser) addError(format string, args ...interface{}) {
	p.errors = append(p.errors, &ParseError{
		Message: fmt.Sprintf(format, args...),
		Line:    p.curToken.Line,
		Column:  p.curToken.Column,
	})
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) curTokenIs(t TokenType) bool  { return p.curToken.Type == t }
func (p *Parser) peekTokenIs(t TokenType) bool { return p.peekToken.Type == t }

// expectPeek advances past the peek token if it matches t, otherwise records
// an error and leaves the cursor in place.
func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.addError("expected next token to be %s, got %s (%q) instead", t, p.peekToken.Type, p.peekToken.Literal)
	return false
}

func (p *Parser) peekPrecedence() precedence {
	if pr, ok := precedences[p.peekToken.Type]; ok {
		return pr
	}
	return LOWEST
}

func (p *Parser) curPrecedence() precedence {
	if pr, ok := precedences[p.curToken.Type]; ok {
		return pr
	}
	return LOWEST
}

// === Program / Statement Dispatch ===

// ParseProgram parses the entire token stream into a *Program.
func (p *Parser) ParseProgram() *Program {
	program := &Program{Statements: []Statement{}}

	for !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() Statement {
	switch p.curToken.Type {
	case TOKEN_VAR:
		return p.parseVarStatement()
	case TOKEN_CONST:
		return p.parseConstStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_FN:
		return p.parseFunctionStatement(false)
	case TOKEN_PUB:
		return p.parsePubStatement()
	case TOKEN_STATIC:
		return p.parseStaticStatement()
	case TOKEN_IF:
		return p.parseIfStatement()
	case TOKEN_WHILE:
		return p.parseWhileStatement()
	case TOKEN_FOR:
		return p.parseForStatement()
	case TOKEN_BREAK:
		return p.parseBreakStatement()
	case TOKEN_CONTINUE:
		return p.parseContinueStatement()
	case TOKEN_IMPORT:
		return p.parseImportStatement()
	case TOKEN_LBRACE:
		return p.parseBlockStatement()
	case TOKEN_SEMICOLON:
		// Stray semicolon; skip silently.
		return nil
	default:
		return p.parseExpressionStatement()
	}
}

// === var / const ===

func (p *Parser) parseVarStatement() Statement {
	stmt := &VarStatement{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional explicit type: `var x i32` / `var x i32 = ...`.
	if p.peekTokenIsTypeStart() {
		p.nextToken()
		stmt.Type = p.parseType()
	}

	if p.peekTokenIs(TOKEN_ASSIGN) {
		p.nextToken() // consume '='
		p.nextToken() // move to expression start
		stmt.Value = p.parseExpression(LOWEST)
	}

	p.skipSemicolon()

	return stmt
}

func (p *Parser) parseConstStatement() Statement {
	stmt := &ConstStatement{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIsTypeStart() {
		p.nextToken()
		stmt.Type = p.parseType()
	}

	if p.peekTokenIs(TOKEN_ASSIGN) {
		p.nextToken()
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
	}

	p.skipSemicolon()

	return stmt
}

// peekTokenIsTypeStart reports whether the peek token can begin a type
// expression (used to distinguish `var x i32 = 1` from `var x = 1`).
func (p *Parser) peekTokenIsTypeStart() bool {
	switch p.peekToken.Type {
	case TOKEN_IDENT, TOKEN_CARET, TOKEN_QUESTION, TOKEN_BANG, TOKEN_LBRACK:
		return true
	default:
		return false
	}
}

func (p *Parser) skipSemicolon() {
	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}
}

// === return / break / continue ===

func (p *Parser) parseReturnStatement() Statement {
	stmt := &ReturnStatement{Token: p.curToken}

	if p.peekTokenIs(TOKEN_SEMICOLON) || p.peekTokenIs(TOKEN_RBRACE) {
		p.skipSemicolon()
		return stmt
	}

	p.nextToken()
	stmt.ReturnValue = p.parseExpression(LOWEST)
	p.skipSemicolon()
	return stmt
}

func (p *Parser) parseBreakStatement() Statement {
	stmt := &BreakStatement{Token: p.curToken}
	p.skipSemicolon()
	return stmt
}

func (p *Parser) parseContinueStatement() Statement {
	stmt := &ContinueStatement{Token: p.curToken}
	p.skipSemicolon()
	return stmt
}

// === #import ===

func (p *Parser) parseImportStatement() Statement {
	stmt := &ImportStatement{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Path = append(stmt.Path, p.curToken.Literal)

	for p.peekTokenIs(TOKEN_DOT) {
		p.nextToken() // consume '.'
		if p.peekTokenIs(TOKEN_ASTERISK) {
			p.nextToken()
			stmt.Wildcard = true
			break
		}
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		stmt.Path = append(stmt.Path, p.curToken.Literal)
	}

	p.skipSemicolon()
	return stmt
}

// === Blocks ===

func (p *Parser) parseBlockStatement() *BlockStatement {
	block := &BlockStatement{Token: p.curToken, Statements: []Statement{}}

	p.nextToken() // consume '{'

	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

// === if / while / for ===

func (p *Parser) parseIfStatement() Statement {
	stmt := &IfStatement{Token: p.curToken}

	p.nextToken() // move past 'if' onto condition
	p.noStructLiterals = true
	stmt.Condition = p.parseExpression(LOWEST)
	p.noStructLiterals = false

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(TOKEN_ELSE) {
		p.nextToken() // consume 'else'
		if p.peekTokenIs(TOKEN_IF) {
			p.nextToken()
			stmt.Alternative = p.parseIfStatement()
		} else if p.expectPeek(TOKEN_LBRACE) {
			stmt.Alternative = p.parseBlockStatement()
		}
	}

	return stmt
}

func (p *Parser) parseWhileStatement() Statement {
	stmt := &WhileStatement{Token: p.curToken}

	p.nextToken()
	p.noStructLiterals = true
	stmt.Condition = p.parseExpression(LOWEST)
	p.noStructLiterals = false

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseForStatement() Statement {
	stmt := &ForStatement{Token: p.curToken}

	p.nextToken() // move past 'for'

	// Parsed at BITWISE precedence (i.e. stopping just below it) rather
	// than LOWEST: the loop's trailing `|<capture>|` binding syntax would
	// otherwise be swallowed by the `|` infix (bitwise-or) parser, since
	// `for 0..10 |i| {...}` has no parentheses around the range/collection
	// to delimit it. noStructLiterals additionally suppresses the `{`
	// infix so a bare `for coll |x| {` isn't misread as a struct literal.
	p.noStructLiterals = true
	first := p.parseExpression(BITWISE)

	if p.peekTokenIs(TOKEN_DOTDOT) {
		p.nextToken() // consume '..'
		p.nextToken()
		stmt.Start = first
		stmt.End = p.parseExpression(BITWISE)
	} else {
		stmt.Collection = first
	}
	p.noStructLiterals = false

	if !p.expectPeek(TOKEN_PIPE) {
		return nil
	}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Capture = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(TOKEN_PIPE) {
		return nil
	}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

// === fn / pub / static ===

func (p *Parser) parsePubStatement() Statement {
	if p.peekTokenIs(TOKEN_FN) {
		p.nextToken()
		fn := p.parseFunctionStatement(false)
		if f, ok := fn.(*FunctionStatement); ok {
			f.IsPub = true
		}
		return fn
	}
	// `pub const` / `pub var` / `pub struct` etc. reuse the same
	// declaration parsers; the pub-ness itself isn't tracked on those
	// nodes yet since this is a partial AST.
	p.nextToken()
	return p.parseStatement()
}

func (p *Parser) parseStaticStatement() Statement {
	if !p.expectPeek(TOKEN_FN) {
		return nil
	}
	fn := p.parseFunctionStatement(true)
	return fn
}

func (p *Parser) parseFunctionStatement(isStatic bool) Statement {
	stmt := &FunctionStatement{Token: p.curToken, IsStatic: isStatic}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional generic parameter list: `:T` or `:(T, U)`.
	if p.peekTokenIs(TOKEN_COLON) {
		p.nextToken() // consume ':'
		if p.peekTokenIs(TOKEN_LPAREN) {
			p.nextToken() // consume '('
			stmt.GenericParams = p.parseIdentList(TOKEN_RPAREN)
		} else if p.expectPeek(TOKEN_IDENT) {
			stmt.GenericParams = []string{p.curToken.Literal}
		}
	}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}
	stmt.Params = p.parseFunctionParams()

	// Return type (required by grammar, but tolerated as void if omitted).
	if !p.peekTokenIs(TOKEN_LBRACE) {
		p.nextToken()
		stmt.ReturnType = p.parseType()
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

// parseIdentList parses a comma-separated identifier list up to (and
// consuming) the closing token.
func (p *Parser) parseIdentList(closing TokenType) []string {
	var idents []string

	if p.peekTokenIs(closing) {
		p.nextToken()
		return idents
	}

	p.nextToken()
	idents = append(idents, p.curToken.Literal)

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		idents = append(idents, p.curToken.Literal)
	}

	if !p.expectPeek(closing) {
		return idents
	}
	return idents
}

func (p *Parser) parseFunctionParams() []*Parameter {
	var params []*Parameter

	if p.peekTokenIs(TOKEN_RPAREN) {
		p.nextToken()
		return params
	}

	p.nextToken()
	params = append(params, p.parseSingleParam())

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		params = append(params, p.parseSingleParam())
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return params
	}
	return params
}

func (p *Parser) parseSingleParam() *Parameter {
	param := &Parameter{Name: &Identifier{Token: p.curToken, Value: p.curToken.Literal}}

	// `self ^Type` / `self Type` receivers and normal `name type` params
	// share the same shape: identifier followed by a type.
	if !p.peekTokenIs(TOKEN_COMMA) && !p.peekTokenIs(TOKEN_RPAREN) {
		p.nextToken()
		param.Type = p.parseType()
	}

	return param
}

// === Expression Statements ===

func (p *Parser) parseExpressionStatement() Statement {
	stmt := &ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(LOWEST)

	p.skipSemicolon()

	return stmt
}

// === Pratt Expression Parsing ===

func (p *Parser) parseExpression(prec precedence) Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.addError("no prefix parse function for %s (%q) found", p.curToken.Type, p.curToken.Literal)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(TOKEN_SEMICOLON) && prec < p.peekPrecedence() {
		if p.noStructLiterals && p.peekTokenIs(TOKEN_LBRACE) {
			// Inside an if/while/for header, `{` always starts the body
			// block, never a struct literal — stop the expression here
			// regardless of the LBRACE entry in the precedence table.
			return leftExp
		}
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() Expression {
	lit := &IntegerLiteral{Token: p.curToken, Raw: p.curToken.Literal}

	cleaned := strings.ReplaceAll(p.curToken.Literal, "_", "")
	base := 10
	switch {
	case strings.HasPrefix(cleaned, "0x") || strings.HasPrefix(cleaned, "0X"):
		base = 16
		cleaned = cleaned[2:]
	case strings.HasPrefix(cleaned, "0o") || strings.HasPrefix(cleaned, "0O"):
		base = 8
		cleaned = cleaned[2:]
	case strings.HasPrefix(cleaned, "0b") || strings.HasPrefix(cleaned, "0B"):
		base = 2
		cleaned = cleaned[2:]
	}

	val, err := strconv.ParseInt(cleaned, base, 64)
	if err != nil {
		// Value may exceed int64 (i128 literal, etc); Value is best-effort
		// and Raw preserves the original text regardless.
		val = 0
	}
	lit.Value = val

	return lit
}

func (p *Parser) parseFloatLiteral() Expression {
	lit := &FloatLiteral{Token: p.curToken, Raw: p.curToken.Literal}

	cleaned := strings.ReplaceAll(p.curToken.Literal, "_", "")
	if strings.HasPrefix(cleaned, "0x") || strings.HasPrefix(cleaned, "0X") {
		// Hex float literals (`0x103.70p-5`) aren't representable via
		// strconv.ParseFloat's decimal path in every Go version uniformly;
		// Raw is kept authoritative and Value is left as a best-effort 0
		// until codegen needs precise conversion.
		lit.Value = 0
		return lit
	}

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		val = 0
	}
	lit.Value = val

	return lit
}

func (p *Parser) parseStringLiteral() Expression {
	return &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseCharLiteral() Expression {
	return &CharLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseBoolLiteral() Expression {
	return &BoolLiteral{Token: p.curToken, Value: p.curTokenIs(TOKEN_TRUE)}
}

func (p *Parser) parseNullLiteral() Expression {
	return &NullLiteral{Token: p.curToken}
}

func (p *Parser) parseGroupedExpression() Expression {
	p.nextToken()
	exp := p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}
	return exp
}

func (p *Parser) parseArrayLiteral() Expression {
	arr := &ArrayLiteral{Token: p.curToken}
	arr.Elements = p.parseExpressionList(TOKEN_RBRACK)
	return arr
}

func (p *Parser) parseExpressionList(closing TokenType) []Expression {
	var list []Expression

	if p.peekTokenIs(closing) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		// Allow a trailing comma before the closing token, as shown in the
		// multidimensional array example in syntax.md.
		if p.peekTokenIs(closing) {
			break
		}
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(closing) {
		return list
	}
	return list
}

func (p *Parser) parsePrefixExpression() Expression {
	exp := &PrefixExpression{Token: p.curToken, Operator: p.curToken.Literal}
	p.nextToken()
	exp.Right = p.parseExpression(PREFIX)
	return exp
}

func (p *Parser) parseInfixExpression(left Expression) Expression {
	exp := &InfixExpression{
		Token:    p.curToken,
		Left:     left,
		Operator: p.curToken.Literal,
	}
	prec := p.curPrecedence()
	p.nextToken()
	exp.Right = p.parseExpression(prec)
	return exp
}

func (p *Parser) parseAssignExpression(left Expression) Expression {
	exp := &AssignExpression{
		Token:    p.curToken,
		Target:   left,
		Operator: p.curToken.Literal,
	}
	// Assignment is right-associative; re-enter at just below ASSIGN so
	// chained assigns (if ever supported) nest correctly. LOWEST is fine
	// for now since Tinoc statements are single assigns.
	p.nextToken()
	exp.Value = p.parseExpression(LOWEST)
	return exp
}

func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpression{Token: p.curToken, Function: function}

	// `Identity:str(...)` parses its callee as a GenericExpression first
	// (via parseGenericSuffix); unwrap it here so CallExpression carries
	// the plain function name plus its generic arguments separately,
	// matching the shape described in syntax.md's function-call grammar.
	if ge, ok := function.(*GenericExpression); ok {
		exp.Function = ge.Base
		exp.GenericArgs = ge.Args
	}

	exp.Arguments = p.parseExpressionList(TOKEN_RPAREN)
	return exp
}

func (p *Parser) parseIndexExpression(left Expression) Expression {
	exp := &IndexExpression{Token: p.curToken, Left: left}
	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_RBRACK) {
		return nil
	}
	return exp
}

func (p *Parser) parseFieldAccessExpression(left Expression) Expression {
	exp := &FieldAccessExpression{Token: p.curToken, Left: left}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	exp.Field = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return exp
}

// parseStructLiteral handles `<Type> { .field = value, ... }` struct
// literals. left is the already-parsed type-position expression (an
// Identifier for `Point { ... }`, or the result of a generic-call-shaped
// parse such as `Pair:i32 { ... }`, which parseNamedOrGenericType would
// otherwise have produced as a type, not an expression — see the
// conversion in exprToType below).
func (p *Parser) parseStructLiteral(left Expression) Expression {
	if p.noStructLiterals {
		// Should not normally be reachable since the infix fn is only
		// invoked when the Pratt loop already decided to consume '{', but
		// kept as a defensive fallback: treat as end of expression.
		return left
	}

	lit := &StructLiteral{Token: p.curToken}
	lit.Type = exprToType(left)

	p.nextToken() // consume '{'

	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		if !p.curTokenIs(TOKEN_DOT) {
			p.addError("expected '.' to start struct literal field, got %s (%q)", p.curToken.Type, p.curToken.Literal)
			break
		}
		if !p.expectPeek(TOKEN_IDENT) {
			break
		}
		field := &StructLiteralField{Name: &Identifier{Token: p.curToken, Value: p.curToken.Literal}}

		if !p.expectPeek(TOKEN_ASSIGN) {
			break
		}
		p.nextToken()
		field.Value = p.parseExpression(LOWEST)

		lit.Fields = append(lit.Fields, field)

		if p.peekTokenIs(TOKEN_COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return lit
}

// exprToType adapts an already-parsed Expression into a TypeExpr for use as
// a struct literal's type position. Handles the plain-identifier case
// (`Point { ... }`) and the generic-qualified case (`Pair:i32 { ... }`)
// directly; anything more elaborate falls back to a NamedType built from
// the expression's own String() rendering so callers still get a usable
// (if less structured) type node instead of a nil.
func exprToType(e Expression) TypeExpr {
	switch v := e.(type) {
	case *Identifier:
		return &NamedType{Token: v.Token, Name: v.Value}
	case *GenericExpression:
		baseName := v.Base.String()
		return &GenericType{Token: v.Token, Base: baseName, Args: v.Args}
	default:
		if e == nil {
			return nil
		}
		return &NamedType{Name: e.String()}
	}
}

// parseGenericSuffix handles the `:T` / `:(T, U)` generic-argument suffix
// when it appears after an expression, e.g. `Pair:i32` or `Identity:str`.
// This is the expression-level counterpart to parseNamedOrGenericType,
// which handles the same syntax in type position.
func (p *Parser) parseGenericSuffix(left Expression) Expression {
	ge := &GenericExpression{Token: p.curToken, Base: left}

	if p.peekTokenIs(TOKEN_LPAREN) {
		p.nextToken() // consume '('
		ge.Args = p.parseTypeList(TOKEN_RPAREN)
	} else {
		p.nextToken()
		ge.Args = []TypeExpr{p.parseType()}
	}

	return ge
}

// parsePointerDerefPostfix handles `x^`, the pointer-dereference postfix
// operator. Registered on TOKEN_CARET as an infix so the Pratt loop picks
// it up after `x`; it ignores the "left-hand" role of an infix fn's second
// operand and treats `left` as the operand being dereferenced.
func (p *Parser) parsePointerDerefPostfix(left Expression) Expression {
	return &PostfixExpression{Token: p.curToken, Operator: "^", Left: left}
}

// parseOptionalUnwrapPostfix handles `x?`, the optional-unwrap postfix
// operator, analogous to parsePointerDerefPostfix above.
func (p *Parser) parseOptionalUnwrapPostfix(left Expression) Expression {
	return &PostfixExpression{Token: p.curToken, Operator: "?", Left: left}
}

// === Type Parsing ===

// parseType parses a type expression starting at curToken. Handles named
// types, generic instantiations (`T:U`, `T:(U,V)`), pointers (`^T`),
// optionals (`?T`), error unions (`!T`, `E!T`), and arrays/slices
// (`[N]T`, `[_]T`, `[N:x]T`, `[]T`).
func (p *Parser) parseType() TypeExpr {
	switch p.curToken.Type {
	case TOKEN_CARET:
		tok := p.curToken
		p.nextToken()
		return &PointerType{Token: tok, Elem: p.parseType()}

	case TOKEN_QUESTION:
		tok := p.curToken
		p.nextToken()
		return &OptionalType{Token: tok, Elem: p.parseType()}

	case TOKEN_BANG:
		tok := p.curToken
		p.nextToken()
		return &ErrorUnionType{Token: tok, Elem: p.parseType()}

	case TOKEN_LBRACK:
		return p.parseArrayType()

	case TOKEN_IDENT:
		return p.parseNamedOrGenericType()

	default:
		p.addError("expected type, got %s (%q)", p.curToken.Type, p.curToken.Literal)
		return nil
	}
}

func (p *Parser) parseNamedOrGenericType() TypeExpr {
	base := &NamedType{Token: p.curToken, Name: p.curToken.Literal}

	// `E!T` explicit error union: an identifier immediately followed by
	// `!` denotes the error set name, not a plain named type.
	if p.peekTokenIs(TOKEN_BANG) {
		bangTok := p.peekToken
		p.nextToken() // consume to '!'
		p.nextToken() // move past '!' onto the value type
		return &ErrorUnionType{Token: bangTok, ErrSet: base, Elem: p.parseType()}
	}

	if !p.peekTokenIs(TOKEN_COLON) {
		return base
	}

	p.nextToken() // consume ':'
	gt := &GenericType{Token: p.curToken, Base: base.Name}

	if p.peekTokenIs(TOKEN_LPAREN) {
		p.nextToken() // consume '('
		gt.Args = p.parseTypeList(TOKEN_RPAREN)
	} else {
		p.nextToken()
		gt.Args = []TypeExpr{p.parseType()}
	}

	return gt
}

func (p *Parser) parseTypeList(closing TokenType) []TypeExpr {
	var list []TypeExpr

	if p.peekTokenIs(closing) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseType())

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseType())
	}

	if !p.expectPeek(closing) {
		return list
	}
	return list
}

// === CLI Entry Points ===

// ParseSource parses the given source text and returns the resulting
// Program along with any parse errors encountered. Errors are collected
// rather than aborting: the parser makes a best effort to keep going so
// callers (e.g. the CLI's "check" and "-a/--ast" flags) can report every
// problem found in one pass instead of stopping at the first one.
func ParseSource(source string) (*Program, []*ParseError) {
	p := NewParser(source)
	program := p.ParseProgram()
	return program, p.Errors()
}

// DumpAST parses source and prints the resulting AST as an indented
// S-expression-like tree, one top-level statement per top-level entry. It
// returns the number of top-level statements parsed and the number of
// parse errors encountered, mirroring DumpTokens' (total, illegal) shape
// so the CLI can decide whether the AST stage succeeded.
func DumpAST(source string) (statements int, errs int) {
	useColor := supportsColor()

	program, errors := ParseSource(source)

	for i, stmt := range program.Statements {
		printASTNode(stmt, 0, useColor)
		if i < len(program.Statements)-1 {
			fmt.Println()
		}
	}

	if len(program.Statements) > 0 {
		fmt.Println()
	}
	fmt.Println("Top-level statements:", len(program.Statements))

	if len(errors) > 0 {
		label := "Parse errors:"
		if useColor {
			fmt.Printf("%s%s %d%s\n", colorRed, label, len(errors), colorReset)
		} else {
			fmt.Printf("%s %d\n", label, len(errors))
		}
		for _, e := range errors {
			if useColor {
				fmt.Printf("  %s%s%s\n", colorRed, e.String(), colorReset)
			} else {
				fmt.Printf("  %s\n", e.String())
			}
		}
	} else {
		fmt.Println("Parse errors: 0")
	}

	return len(program.Statements), len(errors)
}

// printASTNode prints a single node and, for the statement kinds a partial
// parser currently produces, a shallow indented breakdown of its direct
// children. Anything not explicitly broken down here still prints via its
// String() form, so the dump degrades gracefully as more node kinds are
// added to the AST later.
func printASTNode(n Node, depth int, useColor bool) {
	indent := strings.Repeat("  ", depth)
	label := nodeLabel(n)

	if useColor {
		fmt.Printf("%s%s%s%s%s\n", indent, colorCyan, label, colorReset, nodeSummary(n))
	} else {
		fmt.Printf("%s%s%s\n", indent, label, nodeSummary(n))
	}

	switch v := n.(type) {
	case *VarStatement:
		if v.Value != nil {
			printASTNode(&labeledNode{"value", v.Value}, depth+1, useColor)
		}
	case *ConstStatement:
		if v.Value != nil {
			printASTNode(&labeledNode{"value", v.Value}, depth+1, useColor)
		}
	case *FunctionStatement:
		if v.Body != nil {
			for _, s := range v.Body.Statements {
				printASTNode(s, depth+1, useColor)
			}
		}
	case *IfStatement:
		if v.Consequence != nil {
			for _, s := range v.Consequence.Statements {
				printASTNode(s, depth+1, useColor)
			}
		}
		if v.Alternative != nil {
			printASTNode(v.Alternative, depth+1, useColor)
		}
	case *WhileStatement:
		if v.Body != nil {
			for _, s := range v.Body.Statements {
				printASTNode(s, depth+1, useColor)
			}
		}
	case *ForStatement:
		if v.Body != nil {
			for _, s := range v.Body.Statements {
				printASTNode(s, depth+1, useColor)
			}
		}
	case *BlockStatement:
		for _, s := range v.Statements {
			printASTNode(s, depth+1, useColor)
		}
	}
}

// labeledNode is a small adapter so printASTNode can recurse into a named
// child expression (e.g. a var/const's initializer) while still printing a
// useful label instead of the generic type name.
type labeledNode struct {
	label string
	Node
}

func (l *labeledNode) String() string { return l.Node.String() }

func nodeLabel(n Node) string {
	if l, ok := n.(*labeledNode); ok {
		return l.label + ":"
	}
	switch n.(type) {
	case *VarStatement:
		return "VarStatement"
	case *ConstStatement:
		return "ConstStatement"
	case *ReturnStatement:
		return "ReturnStatement"
	case *ExpressionStatement:
		return "ExpressionStatement"
	case *BlockStatement:
		return "BlockStatement"
	case *FunctionStatement:
		return "FunctionStatement"
	case *IfStatement:
		return "IfStatement"
	case *WhileStatement:
		return "WhileStatement"
	case *ForStatement:
		return "ForStatement"
	case *BreakStatement:
		return "BreakStatement"
	case *ContinueStatement:
		return "ContinueStatement"
	case *ImportStatement:
		return "ImportStatement"
	default:
		return fmt.Sprintf("%T", n)
	}
}

func nodeSummary(n Node) string {
	if l, ok := n.(*labeledNode); ok {
		return " " + l.Node.String()
	}
	switch v := n.(type) {
	case *FunctionStatement:
		return " " + v.Name.String() + "(...)"
	case *IfStatement, *WhileStatement, *ForStatement, *BlockStatement:
		return ""
	default:
		return " " + n.String()
	}
}

func (p *Parser) parseArrayType() TypeExpr {
	at := &ArrayType{Token: p.curToken}

	if p.peekTokenIs(TOKEN_RBRACK) {
		// `[]T` slice.
		p.nextToken() // consume ']'
		p.nextToken() // move onto element type
		at.Elem = p.parseType()
		return at
	}

	p.nextToken() // move onto size / '_' / sentinel start

	if p.curTokenIs(TOKEN_IDENT) && p.curToken.Literal == "_" {
		at.Inferred = true
	} else {
		at.Size = p.parseExpression(LOWEST)
	}

	if p.peekTokenIs(TOKEN_COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move onto sentinel expression
		at.Sentinel = p.parseExpression(LOWEST)
	}

	if !p.expectPeek(TOKEN_RBRACK) {
		return at
	}
	p.nextToken() // move onto element type
	at.Elem = p.parseType()

	return at
}
