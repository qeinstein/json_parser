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
// Start and End are byte offsets into the original input (no string allocation).
// For string tokens, Start/End delimit the content INSIDE the quotes.
// For number/keyword tokens, Start/End span the literal text.
type Token struct {
	Type   TokenType
	Start  int // byte offset into input (inclusive)
	End    int // byte offset into input (exclusive)
	Line   int
	Column int
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%s, [%d:%d], line=%d, col=%d)", t.Type, t.Start, t.End, t.Line, t.Column)
}

// Lexer converts a raw JSON byte slice into a stream of tokens.
type Lexer struct {
	input  []byte
	pos    int    // current character offset in input
	line   int    // current line number (1-based)
	column int    // current column number (1-based)
	err    string // error message for the most recent TokenError
}

// NewLexer creates and initializes a Lexer.
func NewLexer(input []byte) *Lexer {
	return &Lexer{
		input:  input,
		line:   1,
		column: 1,
	}
}

// TokenValue returns the string value of a token. This allocates a new string.
// For error tokens, returns the error message.
func (l *Lexer) TokenValue(t Token) string {
	if t.Type == TokenError {
		return l.err
	}
	if t.Start >= t.End {
		return ""
	}
	return string(l.input[t.Start:t.End])
}

// TokenBytes returns the raw byte slice for a token without allocation.
// The returned slice shares memory with the lexer's input; do not modify it.
func (l *Lexer) TokenBytes(t Token) []byte {
	if t.Start >= t.End {
		return nil
	}
	return l.input[t.Start:t.End]
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
		start := l.pos
		l.advance()
		return Token{Type: TokenBraceOpen, Start: start, End: l.pos, Line: startLine, Column: startCol}
	case '}':
		start := l.pos
		l.advance()
		return Token{Type: TokenBraceClose, Start: start, End: l.pos, Line: startLine, Column: startCol}
	case '[':
		start := l.pos
		l.advance()
		return Token{Type: TokenBracketOpen, Start: start, End: l.pos, Line: startLine, Column: startCol}
	case ']':
		start := l.pos
		l.advance()
		return Token{Type: TokenBracketClose, Start: start, End: l.pos, Line: startLine, Column: startCol}
	case ':':
		start := l.pos
		l.advance()
		return Token{Type: TokenColon, Start: start, End: l.pos, Line: startLine, Column: startCol}
	case ',':
		start := l.pos
		l.advance()
		return Token{Type: TokenComma, Start: start, End: l.pos, Line: startLine, Column: startCol}
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
		l.err = fmt.Sprintf("unexpected character: %q", string(b))
		return Token{Type: TokenError, Line: startLine, Column: startCol}
	}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		b := l.input[l.pos]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			l.pos++
			if b == '\n' {
				l.line++
				l.column = 1
			} else {
				l.column++
			}
		} else {
			break
		}
	}
}

func (l *Lexer) scanString() Token {
	startLine := l.line
	startCol := l.column
	l.advance() // consume opening '"'

	contentStart := l.pos
	for {
		if l.pos >= len(l.input) {
			l.err = "unterminated string literal"
			return Token{Type: TokenError, Line: startLine, Column: startCol}
		}
		b := l.peek()
		if b < 0x20 {
			l.err = "invalid control character in string literal"
			return Token{Type: TokenError, Line: l.line, Column: l.column}
		}
		if b == '"' {
			contentEnd := l.pos
			l.advance() // consume closing '"'
			return Token{Type: TokenString, Start: contentStart, End: contentEnd, Line: startLine, Column: startCol}
		}
		if b == '\\' {
			l.advance() // consume '\\'
			if l.pos >= len(l.input) {
				l.err = "unterminated escape sequence"
				return Token{Type: TokenError, Line: l.line, Column: l.column}
			}
			escapeChar := l.advance()
			switch escapeChar {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				// Valid escape sequence
			case 'u':
				// Must have exactly 4 hex digits after '\u'
				for i := 0; i < 4; i++ {
					if l.pos >= len(l.input) {
						l.err = "unterminated unicode escape sequence"
						return Token{Type: TokenError, Line: l.line, Column: l.column}
					}
					hexDigit := l.advance()
					if !isHexDigit(hexDigit) {
						l.err = "invalid unicode escape sequence"
						return Token{Type: TokenError, Line: l.line, Column: l.column}
					}
				}
			default:
				l.err = fmt.Sprintf("invalid escape character: %q", string(escapeChar))
				return Token{Type: TokenError, Line: l.line, Column: l.column - 1}
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
		l.err = "incomplete number"
		return Token{Type: TokenError, Line: startLine, Column: startCol}
	}
	firstDigit := l.peek()
	if !isDigit(firstDigit) {
		l.err = fmt.Sprintf("invalid number: expected digit, got %q", string(firstDigit))
		return Token{Type: TokenError, Line: l.line, Column: l.column}
	}
	l.advance()

	if firstDigit == '0' {
		// Next digit can't be a digit (e.g. 01 is not allowed in JSON)
		if isDigit(l.peek()) {
			l.err = "invalid number: leading zero not allowed"
			return Token{Type: TokenError, Line: l.line, Column: l.column}
		}
	} else {
		// Consume remaining digits
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	// 3. Fraction part
	if l.peek() == '.' {
		l.advance() // consume '.'
		if !isDigit(l.peek()) {
			l.err = fmt.Sprintf("invalid number: expected decimal digit, got %q", string(l.peek()))
			return Token{Type: TokenError, Line: l.line, Column: l.column}
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
			l.err = fmt.Sprintf("invalid number: expected exponent value, got %q", string(l.peek()))
			return Token{Type: TokenError, Line: l.line, Column: l.column}
		}
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	return Token{Type: TokenNumber, Start: startPos, End: l.pos, Line: startLine, Column: startCol}
}

func (l *Lexer) scanKeyword() Token {
	startLine := l.line
	startCol := l.column
	startPos := l.pos

	for isLetter(l.peek()) {
		l.advance()
	}

	// Go compiler optimizes `switch string(byteSlice)` to avoid allocation
	switch string(l.input[startPos:l.pos]) {
	case "true":
		return Token{Type: TokenTrue, Start: startPos, End: l.pos, Line: startLine, Column: startCol}
	case "false":
		return Token{Type: TokenFalse, Start: startPos, End: l.pos, Line: startLine, Column: startCol}
	case "null":
		return Token{Type: TokenNull, Start: startPos, End: l.pos, Line: startLine, Column: startCol}
	default:
		l.err = fmt.Sprintf("unexpected keyword or identifier: %q", string(l.input[startPos:l.pos]))
		return Token{Type: TokenError, Start: startPos, End: l.pos, Line: startLine, Column: startCol}
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
