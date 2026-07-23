package src

import (
	"fmt"
	"os"
	"unicode"
)

// ANSI color codes used to categorize tokens in the printed output.
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorOrange = "\033[38;5;208m"
	colorBlue2  = "\033[38;5;74m"
)

// colorForToken returns the color associated with a token's category.
func colorForToken(t TokenType) string {
	switch t {
	case TOKEN_EOF:
		return colorGray
	case TOKEN_ILLEGAL:
		return colorRed
	case TOKEN_IDENT:
		return colorCyan
	case TOKEN_INT, TOKEN_FLOAT:
		return colorOrange
	case TOKEN_STRING, TOKEN_CHAR:
		return colorGreen
	case TOKEN_VAR, TOKEN_CONST, TOKEN_FN, TOKEN_STRUCT, TOKEN_ENUM, TOKEN_UNION,
		TOKEN_IF, TOKEN_ELSE, TOKEN_SWITCH, TOKEN_FOR, TOKEN_WHILE, TOKEN_BREAK,
		TOKEN_CONTINUE, TOKEN_RETURN, TOKEN_PUB, TOKEN_SELF, TOKEN_STATIC,
		TOKEN_TEST, TOKEN_TRY, TOKEN_TRUE, TOKEN_FALSE, TOKEN_NULL,
		TOKEN_AND, TOKEN_OR, TOKEN_ORELSE, TOKEN_CATCH:
		return colorPurple
	case TOKEN_IMPORT, TOKEN_RUN, TOKEN_PARTIAL:
		return colorYellow
	case TOKEN_LPAREN, TOKEN_RPAREN, TOKEN_LBRACE, TOKEN_RBRACE, TOKEN_LBRACK, TOKEN_RBRACK,
		TOKEN_COMMA, TOKEN_DOT, TOKEN_COLON, TOKEN_SEMICOLON, TOKEN_HASH:
		return colorGray
	default:
		return colorBlue2 // operators
	}
}

func supportsColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	return true
}

type TokenType string

const (
	TOKEN_EOF     TokenType = "EOF"
	TOKEN_ILLEGAL TokenType = "ILLEGAL"

	TOKEN_IDENT  TokenType = "IDENT"
	TOKEN_INT    TokenType = "INT"
	TOKEN_FLOAT  TokenType = "FLOAT"
	TOKEN_STRING TokenType = "STRING"
	TOKEN_CHAR   TokenType = "CHAR"

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

	TOKEN_AND    TokenType = "and"
	TOKEN_OR     TokenType = "or"
	TOKEN_ORELSE TokenType = "orelse"
	TOKEN_CATCH  TokenType = "catch"

	TOKEN_IMPORT  TokenType = "#import"
	TOKEN_RUN     TokenType = "#run"
	TOKEN_PARTIAL TokenType = "#partial"

	TOKEN_ASSIGN   TokenType = "="
	TOKEN_PLUS     TokenType = "+"
	TOKEN_MINUS    TokenType = "-"
	TOKEN_ASTERISK TokenType = "*"
	TOKEN_SLASH    TokenType = "/"
	TOKEN_PERCENT  TokenType = "%"
	TOKEN_BANG     TokenType = "!"
	TOKEN_QUESTION TokenType = "?"
	TOKEN_CARET    TokenType = "^"
	TOKEN_AMP      TokenType = "&"
	TOKEN_PIPE     TokenType = "|"
	TOKEN_TILDE    TokenType = "~"

	TOKEN_PLUS_ASSIGN  TokenType = "+="
	TOKEN_MINUS_ASSIGN TokenType = "-="
	TOKEN_MUL_ASSIGN   TokenType = "*="
	TOKEN_DIV_ASSIGN   TokenType = "/="
	TOKEN_MOD_ASSIGN   TokenType = "%="
	TOKEN_AMP_ASSIGN   TokenType = "&="
	TOKEN_PIPE_ASSIGN  TokenType = "|="
	TOKEN_CARET_ASSIGN TokenType = "^="

	TOKEN_PLUS_PERCENT  TokenType = "+%"
	TOKEN_PLUS_PIPE     TokenType = "+|"
	TOKEN_MINUS_PERCENT TokenType = "-%"
	TOKEN_MINUS_PIPE    TokenType = "-|"
	TOKEN_MUL_PERCENT   TokenType = "*%"
	TOKEN_MUL_PIPE      TokenType = "*|"
	TOKEN_LSHIFT_PIPE   TokenType = "<<|"

	TOKEN_LSHIFT TokenType = "<<"
	TOKEN_RSHIFT TokenType = ">>"
	TOKEN_EQ     TokenType = "=="
	TOKEN_NOT_EQ TokenType = "!="
	TOKEN_LT     TokenType = "<"
	TOKEN_GT     TokenType = ">"
	TOKEN_LTE    TokenType = "<="
	TOKEN_GTE    TokenType = ">="

	TOKEN_ARROW  TokenType = "=>"
	TOKEN_DOTDOT TokenType = ".."

	TOKEN_COMMA     TokenType = ","
	TOKEN_DOT       TokenType = "."
	TOKEN_COLON     TokenType = ":"
	TOKEN_SEMICOLON TokenType = ";"
	TOKEN_HASH      TokenType = "#"

	TOKEN_LPAREN TokenType = "("
	TOKEN_RPAREN TokenType = ")"
	TOKEN_LBRACE TokenType = "{"
	TOKEN_RBRACE TokenType = "}"
	TOKEN_LBRACK TokenType = "["
	TOKEN_RBRACK TokenType = "]"
)

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
}

// Token is a single lexical token with its source position.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// Lexer scans Tinoc source code into tokens.
type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
	line         int
	column       int
}

func New(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}

	l.position = l.readPosition
	l.readPosition++

	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// NextToken scans and returns the next token from the source code.
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespaceAndComments()

	tokStartLine := l.line
	tokStartCol := l.column

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_EQ, Literal: "=="}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TOKEN_ARROW, Literal: "=>"}
		} else {
			tok = l.newToken(TOKEN_ASSIGN, l.ch)
		}
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_PLUS_ASSIGN, Literal: "+="}
		} else if l.peekChar() == '%' {
			l.readChar()
			tok = Token{Type: TOKEN_PLUS_PERCENT, Literal: "+%"}
		} else if l.peekChar() == '|' {
			l.readChar()
			tok = Token{Type: TOKEN_PLUS_PIPE, Literal: "+|"}
		} else {
			tok = l.newToken(TOKEN_PLUS, l.ch)
		}
	case '-':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MINUS_ASSIGN, Literal: "-="}
		} else if l.peekChar() == '%' {
			l.readChar()
			tok = Token{Type: TOKEN_MINUS_PERCENT, Literal: "-%"}
		} else if l.peekChar() == '|' {
			l.readChar()
			tok = Token{Type: TOKEN_MINUS_PIPE, Literal: "-|"}
		} else {
			tok = l.newToken(TOKEN_MINUS, l.ch)
		}
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MUL_ASSIGN, Literal: "*="}
		} else if l.peekChar() == '%' {
			l.readChar()
			tok = Token{Type: TOKEN_MUL_PERCENT, Literal: "*%"}
		} else if l.peekChar() == '|' {
			l.readChar()
			tok = Token{Type: TOKEN_MUL_PIPE, Literal: "*|"}
		} else {
			tok = l.newToken(TOKEN_ASTERISK, l.ch)
		}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_DIV_ASSIGN, Literal: "/="}
		} else {
			tok = l.newToken(TOKEN_SLASH, l.ch)
		}
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MOD_ASSIGN, Literal: "%="}
		} else {
			tok = l.newToken(TOKEN_PERCENT, l.ch)
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_NOT_EQ, Literal: "!="}
		} else {
			tok = l.newToken(TOKEN_BANG, l.ch)
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_LTE, Literal: "<="}
		} else if l.peekChar() == '<' {
			l.readChar()
			if l.peekChar() == '|' {
				l.readChar()
				tok = Token{Type: TOKEN_LSHIFT_PIPE, Literal: "<<|"}
			} else {
				tok = Token{Type: TOKEN_LSHIFT, Literal: "<<"}
			}
		} else {
			tok = l.newToken(TOKEN_LT, l.ch)
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_GTE, Literal: ">="}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TOKEN_RSHIFT, Literal: ">>"}
		} else {
			tok = l.newToken(TOKEN_GT, l.ch)
		}
	case '&':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_AMP_ASSIGN, Literal: "&="}
		} else {
			tok = l.newToken(TOKEN_AMP, l.ch)
		}
	case '|':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_PIPE_ASSIGN, Literal: "|="}
		} else {
			tok = l.newToken(TOKEN_PIPE, l.ch)
		}
	case '^':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_CARET_ASSIGN, Literal: "^="}
		} else {
			tok = l.newToken(TOKEN_CARET, l.ch)
		}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			tok = Token{Type: TOKEN_DOTDOT, Literal: ".."}
		} else {
			tok = l.newToken(TOKEN_DOT, l.ch)
		}
	case '#':
		if isLetter(l.peekChar()) {
			l.readChar()
			ident := l.readIdentifier()
			fullDirective := "#" + ident
			switch fullDirective {
			case "#import":
				tok = Token{Type: TOKEN_IMPORT, Literal: fullDirective}
			case "#run":
				tok = Token{Type: TOKEN_RUN, Literal: fullDirective}
			case "#partial":
				tok = Token{Type: TOKEN_PARTIAL, Literal: fullDirective}
			default:
				tok = Token{Type: TOKEN_ILLEGAL, Literal: fullDirective}
			}
			tok.Line = tokStartLine
			tok.Column = tokStartCol
			return tok
		}
		tok = l.newToken(TOKEN_HASH, l.ch)
	case '?':
		tok = l.newToken(TOKEN_QUESTION, l.ch)
	case '~':
		tok = l.newToken(TOKEN_TILDE, l.ch)
	case ';':
		tok = l.newToken(TOKEN_SEMICOLON, l.ch)
	case ':':
		tok = l.newToken(TOKEN_COLON, l.ch)
	case ',':
		tok = l.newToken(TOKEN_COMMA, l.ch)
	case '(':
		tok = l.newToken(TOKEN_LPAREN, l.ch)
	case ')':
		tok = l.newToken(TOKEN_RPAREN, l.ch)
	case '{':
		tok = l.newToken(TOKEN_LBRACE, l.ch)
	case '}':
		tok = l.newToken(TOKEN_RBRACE, l.ch)
	case '[':
		tok = l.newToken(TOKEN_LBRACK, l.ch)
	case ']':
		tok = l.newToken(TOKEN_RBRACK, l.ch)
	case '"':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString()
	case '\'':
		tok.Type = TOKEN_CHAR
		tok.Literal = l.readCharLiteral()
	case 0:
		tok.Literal = ""
		tok.Type = TOKEN_EOF
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = lookupIdent(tok.Literal)
			tok.Line = tokStartLine
			tok.Column = tokStartCol
			return tok
		} else if isDigit(l.ch) {
			literal, tokType := l.readNumber()
			tok.Literal = literal
			tok.Type = tokType
			tok.Line = tokStartLine
			tok.Column = tokStartCol
			return tok
		} else {
			tok = l.newToken(TOKEN_ILLEGAL, l.ch)
		}
	}

	tok.Line = tokStartLine
	tok.Column = tokStartCol

	l.readChar()
	return tok
}

func (l *Lexer) newToken(tokenType TokenType, ch byte) Token {
	return Token{Type: tokenType, Literal: string(ch)}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() (string, TokenType) {
	position := l.position
	tokType := TOKEN_INT

	if l.ch == '0' {
		peek := l.peekChar()
		if peek == 'x' || peek == 'X' {
			l.readChar()
			l.readChar()
			for isHexDigit(l.ch) || l.ch == '_' || l.ch == '.' || l.ch == 'p' || l.ch == 'P' {
				if l.ch == 'p' || l.ch == 'P' {
					tokType = TOKEN_FLOAT
					l.readChar()
					if l.ch == '+' || l.ch == '-' {
						l.readChar()
					}
					continue
				}
				if l.ch == '.' {
					tokType = TOKEN_FLOAT
				}
				l.readChar()
			}
			return l.input[position:l.position], tokType
		} else if peek == 'b' || peek == 'B' {
			l.readChar()
			l.readChar()
			for l.ch == '0' || l.ch == '1' || l.ch == '_' {
				l.readChar()
			}
			return l.input[position:l.position], TOKEN_INT
		} else if peek == 'o' || peek == 'O' {
			l.readChar()
			l.readChar()
			for isOctalDigit(l.ch) || l.ch == '_' {
				l.readChar()
			}
			return l.input[position:l.position], TOKEN_INT
		}
	}

	for isDigit(l.ch) || l.ch == '_' || l.ch == '.' || l.ch == 'e' || l.ch == 'E' {
		if l.ch == '.' {
			// Distinguishes the range operator ('0..10') from a float ('0.10').
			if l.peekChar() == '.' {
				break
			}
			tokType = TOKEN_FLOAT
		}
		if l.ch == 'e' || l.ch == 'E' {
			tokType = TOKEN_FLOAT
			l.readChar()
			if l.ch == '+' || l.ch == '-' {
				l.readChar()
			}
			continue
		}
		l.readChar()
	}

	return l.input[position:l.position], tokType
}

func (l *Lexer) readString() string {
	l.readChar() // skip opening quote
	position := l.position
	for {
		if l.ch == '"' || l.ch == 0 {
			break
		}
		if l.ch == '\\' {
			l.readChar()
		}
		l.readChar()
	}
	// Closing quote (if present) is consumed by the trailing readChar in NextToken.
	return l.input[position:l.position]
}

func (l *Lexer) readCharLiteral() string {
	l.readChar() // skip opening quote
	position := l.position
	if l.ch == '\\' {
		l.readChar()
	}
	l.readChar()
	// Closing quote (if present) is consumed by the trailing readChar in NextToken.
	return l.input[position:l.position]
}

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		if l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
			l.readChar()
		} else if l.ch == '/' && l.peekChar() == '/' {
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
		} else if l.ch == '/' && l.peekChar() == '*' {
			l.readChar()
			l.readChar()
			for !(l.ch == '*' && l.peekChar() == '/') && l.ch != 0 {
				l.readChar()
			}
			if l.ch != 0 {
				l.readChar()
				l.readChar()
			}
		} else {
			break
		}
	}
}

func lookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return isDigit(ch) || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
}

func isOctalDigit(ch byte) bool {
	return '0' <= ch && ch <= '7'
}

// printHeader prints the column headers for the token table.
func printHeader(useColor bool) {
	if useColor {
		fmt.Printf("%s%-14s %-22s %-10s%s\n", colorBold, "TYPE", "LITERAL", "POSITION", colorReset)
	} else {
		fmt.Printf("%-14s %-22s %-10s\n", "TYPE", "LITERAL", "POSITION")
	}
	fmt.Println("------------------------------------------------------")
}

// printToken prints a single token row, colorized by category.
func printToken(tok Token, useColor bool) {
	pos := fmt.Sprintf("L%d:C%d", tok.Line, tok.Column)
	literal := fmt.Sprintf("%q", tok.Literal)

	if !useColor {
		fmt.Printf("%-14s %-22s %-10s\n", tok.Type, literal, pos)
		return
	}

	c := colorForToken(tok.Type)
	if tok.Type == TOKEN_ILLEGAL {
		c = colorBold + colorRed
	}
	fmt.Printf("%s%-14s%s %s%-22s%s %s%-10s%s\n",
		c, string(tok.Type), colorReset,
		c, literal, colorReset,
		colorDim, pos, colorReset,
	)
}

// printSummary prints token-count statistics at the end of the dump.
func printSummary(counts map[TokenType]int, total int, useColor bool) {
	fmt.Println()
	fmt.Println("Total tokens:", total)

	illegal := counts[TOKEN_ILLEGAL]
	if illegal > 0 {
		if useColor {
			fmt.Printf("%sIllegal tokens: %d%s\n", colorRed, illegal, colorReset)
		} else {
			fmt.Printf("Illegal tokens: %d\n", illegal)
		}
	} else {
		fmt.Println("Illegal tokens: 0")
	}
}

// DumpTokens lexes source and prints every token in a table. It returns the
// total token count and the illegal token count so callers (e.g. the CLI's
// "check" command) can decide whether lexing succeeded.
func DumpTokens(source string) (total int, illegal int) {
	useColor := supportsColor()

	printHeader(useColor)

	lexer := New(source)
	counts := make(map[TokenType]int)

	for {
		tok := lexer.NextToken()
		printToken(tok, useColor)
		counts[tok.Type]++
		total++

		if tok.Type == TOKEN_EOF {
			break
		}
	}

	printSummary(counts, total, useColor)
	return total, counts[TOKEN_ILLEGAL]
}
