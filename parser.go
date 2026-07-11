package json_parser

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// ParseError records a parsing error with positional information.
type ParseError struct {
	Message string
	Line    int
	Column  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d, column %d: %s", e.Line, e.Column, e.Message)
}

// Parser parses a JSON string from a Lexer into Go structures.
type Parser struct {
	lexer        *Lexer
	currentToken Token
	peekToken    Token
}

// NewParser creates a new Parser instance.
func NewParser(lexer *Lexer) *Parser {
	return &Parser{
		lexer: lexer,
	}
}

func (p *Parser) nextToken() {
	p.currentToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

func (p *Parser) error(msg string) error {
	return &ParseError{
		Message: msg,
		Line:    p.currentToken.Line,
		Column:  p.currentToken.Column,
	}
}

// Parse runs the parser and returns the resulting value or a ParseError.
func (p *Parser) Parse() (any, error) {
	// Initialize token lookahead
	p.currentToken = p.lexer.NextToken()
	p.peekToken = p.lexer.NextToken()

	if p.currentToken.Type == TokenError {
		return nil, p.error(p.currentToken.Value)
	}
	if p.currentToken.Type == TokenEOF {
		return nil, p.error("empty JSON input")
	}

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	if p.currentToken.Type == TokenError {
		return nil, p.error(p.currentToken.Value)
	}
	if p.currentToken.Type != TokenEOF {
		return nil, p.error(fmt.Sprintf("unexpected token at end of input: %s", p.currentToken.Type))
	}

	return val, nil
}

func (p *Parser) parseValue() (any, error) {
	switch p.currentToken.Type {
	case TokenTrue:
		p.nextToken()
		return true, nil
	case TokenFalse:
		p.nextToken()
		return false, nil
	case TokenNull:
		p.nextToken()
		return nil, nil
	case TokenNumber:
		val, err := strconv.ParseFloat(p.currentToken.Value, 64)
		if err != nil {
			return nil, p.error(fmt.Sprintf("invalid number format: %s", err.Error()))
		}
		p.nextToken()
		return val, nil
	case TokenString:
		val, err := unescapeString(p.currentToken.Value)
		if err != nil {
			return nil, p.error(fmt.Sprintf("invalid string: %s", err.Error()))
		}
		p.nextToken()
		return val, nil
	case TokenBraceOpen:
		return p.parseObject()
	case TokenBracketOpen:
		return p.parseArray()
	case TokenError:
		return nil, p.error(p.currentToken.Value)
	default:
		return nil, p.error(fmt.Sprintf("unexpected token %s", p.currentToken.Type))
	}
}

func (p *Parser) parseArray() ([]any, error) {
	// consume '['
	if p.currentToken.Type != TokenBracketOpen {
		return nil, p.error("expected '['")
	}
	p.nextToken()

	arr := []any{}
	if p.currentToken.Type == TokenBracketClose {
		p.nextToken()
		return arr, nil
	}

	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)

		if p.currentToken.Type == TokenBracketClose {
			p.nextToken()
			break
		}

		if p.currentToken.Type == TokenError {
			return nil, p.error(p.currentToken.Value)
		}

		if p.currentToken.Type != TokenComma {
			return nil, p.error(fmt.Sprintf("expected ',' or ']' after array element, got %s", p.currentToken.Type))
		}
		p.nextToken() // consume ','

		// Trailing commas are invalid in strict JSON
		if p.currentToken.Type == TokenBracketClose {
			return nil, p.error("trailing comma in array is not allowed")
		}
	}
	return arr, nil
}

func (p *Parser) parseObject() (map[string]any, error) {
	// consume '{'
	if p.currentToken.Type != TokenBraceOpen {
		return nil, p.error("expected '{'")
	}
	p.nextToken()

	obj := make(map[string]any)
	if p.currentToken.Type == TokenBraceClose {
		p.nextToken()
		return obj, nil
	}

	for {
		if p.currentToken.Type == TokenError {
			return nil, p.error(p.currentToken.Value)
		}
		if p.currentToken.Type != TokenString {
			return nil, p.error(fmt.Sprintf("expected string key in object, got %s", p.currentToken.Type))
		}

		rawKey := p.currentToken.Value
		key, err := unescapeString(rawKey)
		if err != nil {
			return nil, p.error(fmt.Sprintf("invalid object key: %s", err.Error()))
		}
		p.nextToken() // consume key

		if p.currentToken.Type != TokenColon {
			return nil, p.error(fmt.Sprintf("expected ':' after object key, got %s", p.currentToken.Type))
		}
		p.nextToken() // consume ':'

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj[key] = val

		if p.currentToken.Type == TokenBraceClose {
			p.nextToken()
			break
		}

		if p.currentToken.Type == TokenError {
			return nil, p.error(p.currentToken.Value)
		}

		if p.currentToken.Type != TokenComma {
			return nil, p.error(fmt.Sprintf("expected ',' or '}' after object value, got %s", p.currentToken.Type))
		}
		p.nextToken() // consume ','

		// Trailing commas are invalid in strict JSON
		if p.currentToken.Type == TokenBraceClose {
			return nil, p.error("trailing comma in object is not allowed")
		}
	}
	return obj, nil
}

// unescapeString decodes escape sequences in raw JSON string token values.
func unescapeString(raw string) (string, error) {
	var sb strings.Builder
	sb.Grow(len(raw))

	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' {
			i++
			if i >= len(raw) {
				return "", fmt.Errorf("unexpected end of string after escape backslash")
			}
			switch raw[i] {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case '/':
				sb.WriteByte('/')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				// decode 4-digit hex code
				if i+4 >= len(raw) {
					return "", fmt.Errorf("invalid unicode escape sequence")
				}
				hexStr := raw[i+1 : i+5]
				i += 4
				val, err := strconv.ParseUint(hexStr, 16, 16)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape sequence: %v", err)
				}

				// Check for UTF-16 surrogate pairs: \uXXXX\uYYYY
				if val >= 0xD800 && val <= 0xDBFF && i+1 < len(raw) && raw[i+1] == '\\' && i+2 < len(raw) && raw[i+2] == 'u' {
					if i+6 < len(raw) {
						trailHexStr := raw[i+3 : i+7]
						trailVal, err := strconv.ParseUint(trailHexStr, 16, 16)
						if err == nil && trailVal >= 0xDC00 && trailVal <= 0xDFFF {
							r := utf16.DecodeRune(rune(val), rune(trailVal))
							sb.WriteRune(r)
							i += 6
							continue
						}
					}
				}

				sb.WriteRune(rune(val))
			default:
				return "", fmt.Errorf("invalid escape character: %c", raw[i])
			}
		} else {
			sb.WriteByte(raw[i])
		}
	}
	return sb.String(), nil
}
