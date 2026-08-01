package src

import "core:strings"

// Lexer walks a source string and produces tokens on demand.
// It only supports what's needed for variables, constants, and basic
// functions: identifiers/keywords, int/float/string/char literals,
// the small operator set from syntax.md's precedence table that basic
// expressions need, and structural punctuation.
Lexer :: struct {
	source: string,
	diags:  ^Diagnostics,

	offset:   int, // byte offset of current char
	read_offset: int, // byte offset of next char to read
	ch:       rune, // current char, 0 for EOF
	line:     int,
	col:      int,
	line_start_offset: int,
}

lexer_make :: proc(source: string, diags: ^Diagnostics) -> Lexer {
	l := Lexer{
		source = source,
		diags  = diags,
		line   = 1,
		col    = 0,
	}
	lexer_advance(&l)
	return l
}

lexer_advance :: proc(l: ^Lexer) {
	if l.read_offset < len(l.source) {
		l.offset = l.read_offset
		c := l.source[l.read_offset]
		if c == '\n' {
			l.line += 1
			l.col = 0
			l.line_start_offset = l.read_offset + 1
		}
		l.ch = rune(c)
		l.read_offset += 1
		l.col += 1
	} else {
		l.offset = len(l.source)
		l.ch = 0
	}
}

lexer_peek :: proc(l: ^Lexer) -> rune {
	if l.read_offset < len(l.source) {
		return rune(l.source[l.read_offset])
	}
	return 0
}

is_letter :: proc(ch: rune) -> bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

is_digit :: proc(ch: rune) -> bool {
	return ch >= '0' && ch <= '9'
}

is_hex_digit :: proc(ch: rune) -> bool {
	return is_digit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

lexer_skip_whitespace_and_comments :: proc(l: ^Lexer) {
	for {
		switch l.ch {
		case ' ', '\t', '\r', '\n':
			lexer_advance(l)
		case '/':
			if lexer_peek(l) == '/' {
				for l.ch != '\n' && l.ch != 0 {
					lexer_advance(l)
				}
			} else {
				return
			}
		case:
			return
		}
	}
}

current_pos :: proc(l: ^Lexer) -> Pos {
	return Pos{line = l.line, col = l.col}
}

// next_token scans and returns the next token from the source.
next_token :: proc(l: ^Lexer) -> Token {
	lexer_skip_whitespace_and_comments(l)

	pos := current_pos(l)
	start := l.offset

	if l.ch == 0 {
		return Token{kind = .EOF, text = "", pos = pos}
	}

	ch := l.ch

	if is_letter(ch) {
		for is_letter(l.ch) || is_digit(l.ch) {
			lexer_advance(l)
		}
		text := l.source[start:l.offset]
		if kind, is_kw := keyword_lookup(text); is_kw {
			return Token{kind = kind, text = text, pos = pos}
		}
		return Token{kind = .Ident, text = text, pos = pos}
	}

	if is_digit(ch) {
		return lex_number(l, pos, start)
	}

	switch ch {
	case '"':
		return lex_string(l, pos)
	case '\'':
		return lex_char(l, pos)
	case '(':
		lexer_advance(l)
		return Token{kind = .L_Paren, text = "(", pos = pos}
	case ')':
		lexer_advance(l)
		return Token{kind = .R_Paren, text = ")", pos = pos}
	case '{':
		lexer_advance(l)
		return Token{kind = .L_Brace, text = "{", pos = pos}
	case '}':
		lexer_advance(l)
		return Token{kind = .R_Brace, text = "}", pos = pos}
	case ',':
		lexer_advance(l)
		return Token{kind = .Comma, text = ",", pos = pos}
	case ';':
		lexer_advance(l)
		return Token{kind = .Semicolon, text = ";", pos = pos}
	case ':':
		lexer_advance(l)
		return Token{kind = .Colon, text = ":", pos = pos}
	case '+':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Plus_Assign, text = "+=", pos = pos}
		}
		return Token{kind = .Plus, text = "+", pos = pos}
	case '-':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Minus_Assign, text = "-=", pos = pos}
		}
		return Token{kind = .Minus, text = "-", pos = pos}
	case '*':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Star_Assign, text = "*=", pos = pos}
		}
		return Token{kind = .Star, text = "*", pos = pos}
	case '/':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Slash_Assign, text = "/=", pos = pos}
		}
		return Token{kind = .Slash, text = "/", pos = pos}
	case '%':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Percent_Assign, text = "%=", pos = pos}
		}
		return Token{kind = .Percent, text = "%", pos = pos}
	case '=':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Eq, text = "==", pos = pos}
		}
		return Token{kind = .Assign, text = "=", pos = pos}
	case '!':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Not_Eq, text = "!=", pos = pos}
		}
		return Token{kind = .Not, text = "!", pos = pos}
	case '<':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Less_Eq, text = "<=", pos = pos}
		}
		return Token{kind = .Less, text = "<", pos = pos}
	case '>':
		lexer_advance(l)
		if l.ch == '=' {
			lexer_advance(l)
			return Token{kind = .Greater_Eq, text = ">=", pos = pos}
		}
		return Token{kind = .Greater, text = ">", pos = pos}
	}

	// Unknown/unsupported character.
	text := l.source[start:l.offset + 1]
	diag_error(l.diags, pos, "unexpected character '%c'", ch)
	lexer_advance(l)
	return Token{kind = .Illegal, text = text, pos = pos}
}

// lex_number scans integer and float literals: decimal, hex (0x), octal (0o),
// binary (0b), underscore separators, and float exponents. Hex floats are
// out of scope (not needed for var/const/basic-fn support).
lex_number :: proc(l: ^Lexer, pos: Pos, start: int) -> Token {
	is_float := false

	if l.ch == '0' && (lexer_peek(l) == 'x' || lexer_peek(l) == 'X') {
		lexer_advance(l) // 0
		lexer_advance(l) // x
		for is_hex_digit(l.ch) || l.ch == '_' {
			lexer_advance(l)
		}
		return Token{kind = .Int_Lit, text = l.source[start:l.offset], pos = pos}
	}

	if l.ch == '0' && (lexer_peek(l) == 'o' || lexer_peek(l) == 'O') {
		lexer_advance(l)
		lexer_advance(l)
		for (l.ch >= '0' && l.ch <= '7') || l.ch == '_' {
			lexer_advance(l)
		}
		return Token{kind = .Int_Lit, text = l.source[start:l.offset], pos = pos}
	}

	if l.ch == '0' && (lexer_peek(l) == 'b' || lexer_peek(l) == 'B') {
		lexer_advance(l)
		lexer_advance(l)
		for l.ch == '0' || l.ch == '1' || l.ch == '_' {
			lexer_advance(l)
		}
		return Token{kind = .Int_Lit, text = l.source[start:l.offset], pos = pos}
	}

	for is_digit(l.ch) || l.ch == '_' {
		lexer_advance(l)
	}

	if l.ch == '.' && is_digit(lexer_peek(l)) {
		is_float = true
		lexer_advance(l) // consume '.'
		for is_digit(l.ch) || l.ch == '_' {
			lexer_advance(l)
		}
	}

	if l.ch == 'e' || l.ch == 'E' {
		is_float = true
		lexer_advance(l)
		if l.ch == '+' || l.ch == '-' {
			lexer_advance(l)
		}
		for is_digit(l.ch) {
			lexer_advance(l)
		}
	}

	kind := Token_Kind.Int_Lit
	if is_float {
		kind = .Float_Lit
	}
	return Token{kind = kind, text = l.source[start:l.offset], pos = pos}
}

// lex_string scans a double-quoted string literal, supporting the common
// backslash escapes. text returned excludes the surrounding quotes.
lex_string :: proc(l: ^Lexer, pos: Pos) -> Token {
	lexer_advance(l) // consume opening quote
	sb := strings.builder_make()
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			lexer_advance(l)
			switch l.ch {
			case 'n':
				strings.write_byte(&sb, '\n')
			case 't':
				strings.write_byte(&sb, '\t')
			case 'r':
				strings.write_byte(&sb, '\r')
			case '"':
				strings.write_byte(&sb, '"')
			case '\\':
				strings.write_byte(&sb, '\\')
			case '0':
				strings.write_byte(&sb, 0)
			case:
				strings.write_rune(&sb, l.ch)
			}
			lexer_advance(l)
		} else {
			strings.write_rune(&sb, l.ch)
			lexer_advance(l)
		}
	}
	if l.ch == '"' {
		lexer_advance(l)
	} else {
		diag_error(l.diags, pos, "unterminated string literal")
	}
	return Token{kind = .String_Lit, text = strings.to_string(sb), pos = pos}
}

// lex_char scans a single-quoted character literal.
lex_char :: proc(l: ^Lexer, pos: Pos) -> Token {
	lexer_advance(l) // consume opening quote
	sb := strings.builder_make()
	if l.ch == '\\' {
		lexer_advance(l)
		switch l.ch {
		case 'n':
			strings.write_byte(&sb, '\n')
		case 't':
			strings.write_byte(&sb, '\t')
		case 'r':
			strings.write_byte(&sb, '\r')
		case '\'':
			strings.write_byte(&sb, '\'')
		case '\\':
			strings.write_byte(&sb, '\\')
		case:
			strings.write_rune(&sb, l.ch)
		}
		lexer_advance(l)
	} else if l.ch != '\'' && l.ch != 0 {
		strings.write_rune(&sb, l.ch)
		lexer_advance(l)
	}
	if l.ch == '\'' {
		lexer_advance(l)
	} else {
		diag_error(l.diags, pos, "unterminated char literal")
	}
	return Token{kind = .Char_Lit, text = strings.to_string(sb), pos = pos}
}

// tokenize_all runs the lexer to completion and returns every token,
// including a trailing EOF token.
tokenize_all :: proc(source: string, diags: ^Diagnostics) -> [dynamic]Token {
	l := lexer_make(source, diags)
	tokens := make([dynamic]Token)
	for {
		tok := next_token(&l)
		append(&tokens, tok)
		if tok.kind == .EOF {
			break
		}
	}
	return tokens
}
