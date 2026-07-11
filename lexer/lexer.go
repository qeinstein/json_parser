package lexer

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
	Input  []byte // Raw JSON input
	Pos    int    // Current character offset in input
	Line   int    // Current line number (1-based)
	Column int    // Current column number (1-based)
	errMsg string // Error message for the most recent TokenError
}

// NewLexer creates and initializes a Lexer.
func NewLexer(input []byte) *Lexer {
	return &Lexer{
		Input:  input,
		Line:   1,
		Column: 1,
	}
}

// Error returns the error message for the most recent TokenError.
func (l *Lexer) Error() string {
	return l.errMsg
}

// TokenValue returns the string value of a token. This allocates a new string.
// For error tokens, returns the error message.
func (l *Lexer) TokenValue(t Token) string {
	if t.Type == TokenError {
		return l.errMsg
	}
	if t.Start >= t.End {
		return ""
	}
	return string(l.Input[t.Start:t.End])
}

// TokenBytes returns the raw byte slice for a token without allocation.
// The returned slice shares memory with the lexer's input; do not modify it.
func (l *Lexer) TokenBytes(t Token) []byte {
	if t.Start >= t.End {
		return nil
	}
	return l.Input[t.Start:t.End]
}

// peek returns the byte at the current position without consuming it.
// Returns 0 if end of input is reached.
func (l *Lexer) peek() byte {
	if l.Pos >= len(l.Input) {
		return 0
	}
	return l.Input[l.Pos]
}

// advance consumes and returns the byte at the current position.
// Returns 0 if end of input is reached.
func (l *Lexer) advance() byte {
	if l.Pos >= len(l.Input) {
		return 0
	}
	b := l.Input[l.Pos]
	l.Pos++
	if b == '\n' {
		l.Line++
		l.Column = 1
	} else {
		l.Column++
	}
	return b
}

// NextToken scans and returns the next token.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.Pos >= len(l.Input) {
		return Token{Type: TokenEOF, Line: l.Line, Column: l.Column}
	}

	startLine := l.Line
	startCol := l.Column
	b := l.peek()

	switch b {
	case '{':
		start := l.Pos
		l.advance()
		return Token{Type: TokenBraceOpen, Start: start, End: l.Pos, Line: startLine, Column: startCol}
	case '}':
		start := l.Pos
		l.advance()
		return Token{Type: TokenBraceClose, Start: start, End: l.Pos, Line: startLine, Column: startCol}
	case '[':
		start := l.Pos
		l.advance()
		return Token{Type: TokenBracketOpen, Start: start, End: l.Pos, Line: startLine, Column: startCol}
	case ']':
		start := l.Pos
		l.advance()
		return Token{Type: TokenBracketClose, Start: start, End: l.Pos, Line: startLine, Column: startCol}
	case ':':
		start := l.Pos
		l.advance()
		return Token{Type: TokenColon, Start: start, End: l.Pos, Line: startLine, Column: startCol}
	case ',':
		start := l.Pos
		l.advance()
		return Token{Type: TokenComma, Start: start, End: l.Pos, Line: startLine, Column: startCol}
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
		l.errMsg = fmt.Sprintf("unexpected character: %q", string(b))
		return Token{Type: TokenError, Line: startLine, Column: startCol}
	}
}

func (l *Lexer) skipWhitespace() {
	for l.Pos < len(l.Input) {
		b := l.Input[l.Pos]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			l.Pos++
			if b == '\n' {
				l.Line++
				l.Column = 1
			} else {
				l.Column++
			}
		} else {
			break
		}
	}
}

func (l *Lexer) scanString() Token {
	startLine := l.Line
	startCol := l.Column
	l.advance() // consume opening '"'

	contentStart := l.Pos
	for {
		if l.Pos >= len(l.Input) {
			l.errMsg = "unterminated string literal"
			return Token{Type: TokenError, Line: startLine, Column: startCol}
		}
		b := l.peek()
		if b < 0x20 {
			l.errMsg = "invalid control character in string literal"
			return Token{Type: TokenError, Line: l.Line, Column: l.Column}
		}
		if b == '"' {
			contentEnd := l.Pos
			l.advance() // consume closing '"'
			return Token{Type: TokenString, Start: contentStart, End: contentEnd, Line: startLine, Column: startCol}
		}
		if b == '\\' {
			l.advance() // consume '\\'
			if l.Pos >= len(l.Input) {
				l.errMsg = "unterminated escape sequence"
				return Token{Type: TokenError, Line: l.Line, Column: l.Column}
			}
			escapeChar := l.advance()
			switch escapeChar {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				// Valid escape sequence
			case 'u':
				// Must have exactly 4 hex digits after '\u'
				for i := 0; i < 4; i++ {
					if l.Pos >= len(l.Input) {
						l.errMsg = "unterminated unicode escape sequence"
						return Token{Type: TokenError, Line: l.Line, Column: l.Column}
					}
					hexDigit := l.advance()
					if !isHexDigit(hexDigit) {
						l.errMsg = "invalid unicode escape sequence"
						return Token{Type: TokenError, Line: l.Line, Column: l.Column}
					}
				}
			default:
				l.errMsg = fmt.Sprintf("invalid escape character: %q", string(escapeChar))
				return Token{Type: TokenError, Line: l.Line, Column: l.Column - 1}
			}
		} else {
			l.advance()
		}
	}
}

func (l *Lexer) scanNumber() Token {
	startLine := l.Line
	startCol := l.Column
	startPos := l.Pos

	// 1. Optional negative sign
	if l.peek() == '-' {
		l.advance()
	}

	// 2. Integer part
	if l.Pos >= len(l.Input) {
		l.errMsg = "incomplete number"
		return Token{Type: TokenError, Line: startLine, Column: startCol}
	}
	firstDigit := l.peek()
	if !isDigit(firstDigit) {
		l.errMsg = fmt.Sprintf("invalid number: expected digit, got %q", string(firstDigit))
		return Token{Type: TokenError, Line: l.Line, Column: l.Column}
	}
	l.advance()

	if firstDigit == '0' {
		// Next digit can't be a digit (e.g. 01 is not allowed in JSON)
		if isDigit(l.peek()) {
			l.errMsg = "invalid number: leading zero not allowed"
			return Token{Type: TokenError, Line: l.Line, Column: l.Column}
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
			l.errMsg = fmt.Sprintf("invalid number: expected decimal digit, got %q", string(l.peek()))
			return Token{Type: TokenError, Line: l.Line, Column: l.Column}
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
			l.errMsg = fmt.Sprintf("invalid number: expected exponent value, got %q", string(l.peek()))
			return Token{Type: TokenError, Line: l.Line, Column: l.Column}
		}
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	return Token{Type: TokenNumber, Start: startPos, End: l.Pos, Line: startLine, Column: startCol}
}

func (l *Lexer) scanKeyword() Token {
	startLine := l.Line
	startCol := l.Column
	startPos := l.Pos

	for isLetter(l.peek()) {
		l.advance()
	}

	// Go compiler optimizes `switch string(byteSlice)` to avoid allocation
	switch string(l.Input[startPos:l.Pos]) {
	case "true":
		return Token{Type: TokenTrue, Start: startPos, End: l.Pos, Line: startLine, Column: startCol}
	case "false":
		return Token{Type: TokenFalse, Start: startPos, End: l.Pos, Line: startLine, Column: startCol}
	case "null":
		return Token{Type: TokenNull, Start: startPos, End: l.Pos, Line: startLine, Column: startCol}
	default:
		l.errMsg = fmt.Sprintf("unexpected keyword or identifier: %q", string(l.Input[startPos:l.Pos]))
		return Token{Type: TokenError, Start: startPos, End: l.Pos, Line: startLine, Column: startCol}
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
