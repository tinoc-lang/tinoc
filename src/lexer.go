package src

import (
	"fmf"
	"unicode"
)

// TokenType represents the type of a lexical token in TinocLang.
type TokenType string


// Token represents a single lexical token.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// Lexer scans TinocLang source code into tokens.
type Lexer struct {
	input        string
	position     int  // current character position
	readPosition int  // next character position
	ch           byte // current character under inspection
	line         int  // current line number (1-indexed)
	column       int  // current column number (1-indexed)
}
