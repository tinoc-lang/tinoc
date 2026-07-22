package src

import (
	"fmt"
	"unicode"
)

// TokenType represents the type of a lexical token in TinocLang.
type TokenType string

const (
	// Special Tokens
	TOKEN_EOF     TokenType = "EOF"
	TOKEN_ILLEGAL TokenType = "ILLEGAL"

	// Identifiers & Literals
	TOKEN_IDENT  TokenType = "IDENT"  // main, score, Point
	TOKEN_INT    TokenType = "INT"    // 10, 0xFF, 0b1101, 1_000_000
	TOKEN_FLOAT  TokenType = "FLOAT"  // 123.0, 1e10, 0x103.70p-5
	TOKEN_STRING TokenType = "STRING" // "hello"
	TOKEN_CHAR   TokenType = "CHAR"   // 'a'

	// Keywords
	TOKEN_VAR      TokenType = "var"
	TOKEN_CONST    TokenType = "const"
	TOKEN_FN       TokenType = "fn"
	TOKEN_STRUCT   TokenType = "struct"
	TOKEN_ENUM     TokenType = "enum"
	TOKEN_UNION    TokenType = "union"
	TOKEN_IF       TokenType = "if"
	TOKEN_ELSE     TokenType = "else"
	TOKEN_SWITCH   TokenType = "switch"
	TOKEN_FOR      TokenType = "for"
	TOKEN_WHILE    TokenType = "while"
	TOKEN_BREAK    TokenType = "break"
	TOKEN_CONTINUE TokenType = "continue"
	TOKEN_RETURN   TokenType = "return"
	TOKEN_PUB      TokenType = "pub"
	TOKEN_SELF     TokenType = "self"
	TOKEN_STATIC   TokenType = "static"
	TOKEN_TEST     TokenType = "test"
	TOKEN_TRY      TokenType = "try"
	TOKEN_TRUE     TokenType = "true"
	TOKEN_FALSE    TokenType = "false"
	TOKEN_NULL     TokenType = "null"
    TOKEN_DEFER    TokenType = "defer"

	// Keyword Operators
	TOKEN_AND    TokenType = "and"
	TOKEN_OR     TokenType = "or"
	TOKEN_ORELSE TokenType = "orelse"
	TOKEN_CATCH  TokenType = "catch"

	// Preprocessor Directives
	TOKEN_IMPORT  TokenType = "#import"
	TOKEN_RUN     TokenType = "#run"
	TOKEN_PARTIAL TokenType = "#partial"

	// Standard Operators
	TOKEN_ASSIGN   TokenType = "="
	TOKEN_PLUS     TokenType = "+"
	TOKEN_MINUS    TokenType = "-"
	TOKEN_ASTERISK TokenType = "*"
	TOKEN_SLASH    TokenType = "/"
	TOKEN_PERCENT  TokenType = "%"
	TOKEN_BANG     TokenType = "!"
	TOKEN_QUESTION TokenType = "?"
	TOKEN_CARET    TokenType = "^" // Dereference or Bitwise XOR
	TOKEN_AMP      TokenType = "&" // Address of or Bitwise AND
	TOKEN_PIPE     TokenType = "|"
	TOKEN_TILDE    TokenType = "~"

	// Compound Assignment
	TOKEN_PLUS_ASSIGN  TokenType = "+="
	TOKEN_MINUS_ASSIGN TokenType = "-="
	TOKEN_MUL_ASSIGN   TokenType = "*="
	TOKEN_DIV_ASSIGN   TokenType = "/="
	TOKEN_MOD_ASSIGN   TokenType = "%="
	TOKEN_AMP_ASSIGN   TokenType = "&="
	TOKEN_PIPE_ASSIGN  TokenType = "|="
	TOKEN_CARET_ASSIGN TokenType = "^="

	// Tinoc Special Arithmetic (Wrapping & Saturating)
	TOKEN_PLUS_PERCENT  TokenType = "+%"  // Wrapping Add
	TOKEN_PLUS_PIPE     TokenType = "+|"  // Saturating Add
	TOKEN_MINUS_PERCENT TokenType = "-%"  // Wrapping Sub
	TOKEN_MINUS_PIPE    TokenType = "-|"  // Saturating Sub
	TOKEN_MUL_PERCENT   TokenType = "*%"  // Wrapping Mul
	TOKEN_MUL_PIPE      TokenType = "*|"  // Saturating Mul
	TOKEN_LSHIFT_PIPE   TokenType = "<<|" // Saturating Shift

	// Shift & Logical Comparisons
	TOKEN_LSHIFT TokenType = "<<"
	TOKEN_RSHIFT TokenType = ">>"
	TOKEN_EQ     TokenType = "=="
	TOKEN_NOT_EQ TokenType = "!="
	TOKEN_LT     TokenType = "<"
	TOKEN_GT     TokenType = ">"
	TOKEN_LTE    TokenType = "<="
	TOKEN_GTE    TokenType = ">="

	// Syntax Elements
	TOKEN_ARROW  TokenType = "=>"
	TOKEN_DOTDOT TokenType = ".."

	// Delimiters
	TOKEN_COMMA     TokenType = ","
	TOKEN_DOT       TokenType = "."
	TOKEN_COLON     TokenType = ":"
	TOKEN_SEMICOLON TokenType = ";"
	TOKEN_HASH      TokenType = "#"

	TOKEN_LPAREN   TokenType = "("
	TOKEN_RPAREN   TokenType = ")"
	TOKEN_LBRACE   TokenType = "{"
	TOKEN_RBRACE   TokenType = "}"
	TOKEN_LBRACK   TokenType = "["
	TOKEN_RBRACK   TokenType = "]"
)

// Keywords mapping
var keywords = map[string]TokenType{
	"var":      TOKEN_VAR,
	"const":    TOKEN_CONST,
	"fn":       TOKEN_FN,
	"struct":   TOKEN_STRUCT,
	"enum":     TOKEN_ENUM,
	"union":    TOKEN_UNION,
	"if":       TOKEN_IF,
	"else":     TOKEN_ELSE,
	"switch":   TOKEN_SWITCH,
	"for":      TOKEN_FOR,
	"while":    TOKEN_WHILE,
	"break":    TOKEN_BREAK,
	"continue": TOKEN_CONTINUE,
	"return":   TOKEN_RETURN,
	"pub":      TOKEN_PUB,
	"self":     TOKEN_SELF,
	"static":   TOKEN_STATIC,
	"test":     TOKEN_TEST,
	"try":      TOKEN_TRY,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
	"null":     TOKEN_NULL,
	"and":      TOKEN_AND,
	"or":       TOKEN_OR,
	"orelse":   TOKEN_ORELSE,
	"catch":    TOKEN_CATCH,
	"defer":    TOKEN_DEFER,
}



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


// New creates a initialized Lexer instance.
func New(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}
