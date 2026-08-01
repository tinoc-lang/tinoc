package src

// Token_Kind enumerates every lexical token category the lexer can produce.
// This is intentionally scoped to what's needed for variables, constants,
// and basic functions (per syntax.md) — not the full language grammar.
Token_Kind :: enum {
	Illegal,
	EOF,

	// Literals
	Ident,
	Int_Lit,
	Float_Lit,
	String_Lit,
	Char_Lit,

	// Keywords
	Kw_Fn,
	Kw_Var,
	Kw_Const,
	Kw_Return,
	Kw_If,
	Kw_Else,
	Kw_True,
	Kw_False,

	// Punctuation
	L_Paren,   // (
	R_Paren,   // )
	L_Brace,   // {
	R_Brace,   // }
	Comma,     // ,
	Semicolon, // ;
	Colon,     // :

	// Operators
	Plus,        // +
	Minus,       // -
	Star,        // *
	Slash,       // /
	Percent,     // %
	Assign,      // =
	Plus_Assign, // +=
	Minus_Assign,// -=
	Star_Assign, // *=
	Slash_Assign,// /=
	Percent_Assign, // %=
	Eq,          // ==
	Not_Eq,      // !=
	Less,        // <
	Less_Eq,     // <=
	Greater,     // >
	Greater_Eq,  // >=
	Not,         // !
	And_And,     // and (keyword-operator, kept as token for precedence table)
	Or_Or,       // or
}

// Token is a single lexical unit with its source text and starting position.
Token :: struct {
	kind: Token_Kind,
	text: string, // raw slice of source text for this token
	pos:  Pos,
}

keyword_lookup :: proc(ident: string) -> (Token_Kind, bool) {
	switch ident {
	case "fn":
		return .Kw_Fn, true
	case "var":
		return .Kw_Var, true
	case "const":
		return .Kw_Const, true
	case "return":
		return .Kw_Return, true
	case "if":
		return .Kw_If, true
	case "else":
		return .Kw_Else, true
	case "true":
		return .Kw_True, true
	case "false":
		return .Kw_False, true
	case "and":
		return .And_And, true
	case "or":
		return .Or_Or, true
	}
	return .Illegal, false
}

token_kind_name :: proc(k: Token_Kind) -> string {
	switch k {
	case .Illegal: return "illegal"
	case .EOF: return "eof"
	case .Ident: return "identifier"
	case .Int_Lit: return "integer literal"
	case .Float_Lit: return "float literal"
	case .String_Lit: return "string literal"
	case .Char_Lit: return "char literal"
	case .Kw_Fn: return "'fn'"
	case .Kw_Var: return "'var'"
	case .Kw_Const: return "'const'"
	case .Kw_Return: return "'return'"
	case .Kw_If: return "'if'"
	case .Kw_Else: return "'else'"
	case .Kw_True: return "'true'"
	case .Kw_False: return "'false'"
	case .L_Paren: return "'('"
	case .R_Paren: return "')'"
	case .L_Brace: return "'{'"
	case .R_Brace: return "'}'"
	case .Comma: return "','"
	case .Semicolon: return "';'"
	case .Colon: return "':'"
	case .Plus: return "'+'"
	case .Minus: return "'-'"
	case .Star: return "'*'"
	case .Slash: return "'/'"
	case .Percent: return "'%'"
	case .Assign: return "'='"
	case .Plus_Assign: return "'+='"
	case .Minus_Assign: return "'-='"
	case .Star_Assign: return "'*='"
	case .Slash_Assign: return "'/='"
	case .Percent_Assign: return "'%='"
	case .Eq: return "'=='"
	case .Not_Eq: return "'!='"
	case .Less: return "'<'"
	case .Less_Eq: return "'<='"
	case .Greater: return "'>'"
	case .Greater_Eq: return "'>='"
	case .Not: return "'!'"
	case .And_And: return "'and'"
	case .Or_Or: return "'or'"
	}
	return "unknown"
}
