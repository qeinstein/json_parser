package json_parser

import (
	"fmt"
)

// TokenType represents the type of a JSON token.
type TokenType int

const (
	TokenError TokenType = iota
	TokenEOF
	TokenBraceOpen    // {
	TokenBraceClose   // }
	TokenBracketOpen  // [
	TokenBracketClose // ]
	TokenColon        // :
	TokenComma        // ,
	TokenString       // "string"
	TokenNumber       // 123, -3.14, 1e10
	TokenTrue         // true
	TokenFalse        // false
	TokenNull         // null
)

func (t TokenType) String() string {
	switch t {
	case TokenError:
		return "Error"
	case TokenEOF:
		return "EOF"
	case TokenBraceOpen:
		return "{"
	case TokenBraceClose:
		return "}"
	case TokenBracketOpen:
		return "["
	case TokenBracketClose:
		return "]"
	case TokenColon:
		return ":"
	case TokenComma:
		return ","
	case TokenString:
		return "String"
	case TokenNumber:
		return "Number"
	case TokenTrue:
		return "true"
	case TokenFalse:
		return "false"
	case TokenNull:
		return "null"
	default:
		return "Unknown"
	}
}

// Token represents a single JSON token scanned from the input.
type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%s, %q, line=%d, col=%d)", t.Type, t.Value, t.Line, t.Column)
}

// Lexer converts a raw JSON byte slice into a stream of tokens.
type Lexer struct {
	input  []byte
	pos    int // current character offset in input
	line   int // current line number (1-based)
	column int // current column number (1-based)
}

// NewLexer creates and initializes a Lexer.
func NewLexer(input []byte) *Lexer {
	return &Lexer{
		input:  input,
		line:   1,
		column: 1,
	}
}

// peek returns the byte at the current position without consuming it.
// Returns 0 if end of input is reached.
func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

// advance consumes and returns the byte at the current position.
// Returns 0 if end of input is reached.
func (l *Lexer) advance() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	b := l.input[l.pos]
	l.pos++
	if b == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	return b
}

// NextToken scans and returns the next token.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Line: l.line, Column: l.column}
	}

	startLine := l.line
	startCol := l.column
	b := l.peek()

	switch b {
	case '{':
		l.advance()
		return Token{Type: TokenBraceOpen, Value: "{", Line: startLine, Column: startCol}
	case '}':
		l.advance()
		return Token{Type: TokenBraceClose, Value: "}", Line: startLine, Column: startCol}
	case '[':
		l.advance()
		return Token{Type: TokenBracketOpen, Value: "[", Line: startLine, Column: startCol}
	case ']':
		l.advance()
		return Token{Type: TokenBracketClose, Value: "]", Line: startLine, Column: startCol}
	case ':':
		l.advance()
		return Token{Type: TokenColon, Value: ":", Line: startLine, Column: startCol}
	case ',':
		l.advance()
		return Token{Type: TokenComma, Value: ",", Line: startLine, Column: startCol}
	case '"':
		return l.scanString()
	default:
		if isDigit(b) || b == '-' {
			return l.scanNumber()
		}
		if isLetter(b) {
			return l.scanKeyword()
		}
		l.advance()
		return Token{Type: TokenError, Value: fmt.Sprintf("unexpected character: %q", string(b)), Line: startLine, Column: startCol}
	}
}

func (l *Lexer) skipWhitespace() {
	for {
		b := l.peek()
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) scanString() Token {
	startLine := l.line
	startCol := l.column
	l.advance() // consume opening '"'

	startPos := l.pos
	for {
		if l.pos >= len(l.input) {
			return Token{Type: TokenError, Value: "unterminated string literal", Line: startLine, Column: startCol}
		}
		b := l.peek()
		if b < 0x20 {
			return Token{Type: TokenError, Value: "invalid control character in string literal", Line: l.line, Column: l.column}
		}
		if b == '"' {
			val := string(l.input[startPos:l.pos])
			l.advance() // consume closing '"'
			return Token{Type: TokenString, Value: val, Line: startLine, Column: startCol}
		}
		if b == '\\' {
			l.advance() // consume '\\'
			if l.pos >= len(l.input) {
				return Token{Type: TokenError, Value: "unterminated escape sequence", Line: l.line, Column: l.column}
			}
			escapeChar := l.advance()
			switch escapeChar {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				// Valid escape sequence
			case 'u':
				// Must have exactly 4 hex digits after '\u'
				for i := 0; i < 4; i++ {
					if l.pos >= len(l.input) {
						return Token{Type: TokenError, Value: "unterminated unicode escape sequence", Line: l.line, Column: l.column}
					}
					hexDigit := l.advance()
					if !isHexDigit(hexDigit) {
						return Token{Type: TokenError, Value: "invalid unicode escape sequence", Line: l.line, Column: l.column}
					}
				}
			default:
				return Token{Type: TokenError, Value: fmt.Sprintf("invalid escape character: %q", string(escapeChar)), Line: l.line, Column: l.column - 1}
			}
		} else {
			l.advance()
		}
	}
}

func (l *Lexer) scanNumber() Token {
	startLine := l.line
	startCol := l.column
	startPos := l.pos

	// 1. Optional negative sign
	if l.peek() == '-' {
		l.advance()
	}

	// 2. Integer part
	if l.pos >= len(l.input) {
		return Token{Type: TokenError, Value: "incomplete number", Line: startLine, Column: startCol}
	}
	firstDigit := l.peek()
	if !isDigit(firstDigit) {
		return Token{Type: TokenError, Value: fmt.Sprintf("invalid number: expected digit, got %q", string(firstDigit)), Line: l.line, Column: l.column}
	}
	l.advance()

	if firstDigit == '0' {
		// Next digit can't be a digit (e.g. 01 is not allowed in JSON, must be decimal point or exponent or end)
		if isDigit(l.peek()) {
			return Token{Type: TokenError, Value: "invalid number: leading zero not allowed", Line: l.line, Column: l.column}
		}
	} else {
		// Consume digits
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	// 3. Fraction part
	if l.peek() == '.' {
		l.advance() // consume '.'
		if !isDigit(l.peek()) {
			return Token{Type: TokenError, Value: fmt.Sprintf("invalid number: expected decimal digit, got %q", string(l.peek())), Line: l.line, Column: l.column}
		}
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	// 4. Exponent part
	if b := l.peek(); b == 'e' || b == 'E' {
		l.advance() // consume 'e' or 'E'
		next := l.peek()
		if next == '+' || next == '-' {
			l.advance()
		}
		if !isDigit(l.peek()) {
			return Token{Type: TokenError, Value: fmt.Sprintf("invalid number: expected exponent value, got %q", string(l.peek())), Line: l.line, Column: l.column}
		}
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	val := string(l.input[startPos:l.pos])
	return Token{Type: TokenNumber, Value: val, Line: startLine, Column: startCol}
}

func (l *Lexer) scanKeyword() Token {
	startLine := l.line
	startCol := l.column
	startPos := l.pos

	for isLetter(l.peek()) {
		l.advance()
	}

	val := string(l.input[startPos:l.pos])
	switch val {
	case "true":
		return Token{Type: TokenTrue, Value: val, Line: startLine, Column: startCol}
	case "false":
		return Token{Type: TokenFalse, Value: val, Line: startLine, Column: startCol}
	case "null":
		return Token{Type: TokenNull, Value: val, Line: startLine, Column: startCol}
	default:
		return Token{Type: TokenError, Value: fmt.Sprintf("unexpected keyword or identifier: %q", val), Line: startLine, Column: startCol}
	}
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}
