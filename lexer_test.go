package json_parser

import (
	"testing"
)

func TestLexer_BasicTokens(t *testing.T) {
	input := `{} [] :,`
	expected := []struct {
		typ TokenType
		val string
	}{
		{TokenBraceOpen, "{"},
		{TokenBraceClose, "}"},
		{TokenBracketOpen, "["},
		{TokenBracketClose, "]"},
		{TokenColon, ":"},
		{TokenComma, ","},
		{TokenEOF, ""},
	}

	lexer := NewLexer([]byte(input))
	for i, exp := range expected {
		tok := lexer.NextToken()
		if tok.Type != exp.typ {
			t.Fatalf("[%d] expected token type %s, got %s (val=%q)", i, exp.typ, tok.Type, lexer.TokenValue(tok))
		}
		if exp.val != "" {
			got := lexer.TokenValue(tok)
			if got != exp.val {
				t.Fatalf("[%d] expected token value %q, got %q", i, exp.val, got)
			}
		}
	}
}

func TestLexer_Keywords(t *testing.T) {
	input := `true false null`
	expected := []TokenType{TokenTrue, TokenFalse, TokenNull, TokenEOF}

	lexer := NewLexer([]byte(input))
	for i, typ := range expected {
		tok := lexer.NextToken()
		if tok.Type != typ {
			t.Fatalf("[%d] expected %s, got %s", i, typ, tok.Type)
		}
	}
}

func TestLexer_Strings(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenType
		val      string
	}{
		{`"hello"`, TokenString, `hello`},
		{`"hello \n world"`, TokenString, `hello \n world`},
		{`"escaped \" quote"`, TokenString, `escaped \" quote`},
		{`"unicode \u0041 escape"`, TokenString, `unicode \u0041 escape`},
		// Error cases
		{`"unterminated`, TokenError, `unterminated string literal`},
		{`"invalid escape \x"`, TokenError, `invalid escape character: 'x'`},
		{`"invalid unicode \u12"`, TokenError, `invalid unicode escape sequence`},
	}

	for _, tc := range tests {
		lexer := NewLexer([]byte(tc.input))
		tok := lexer.NextToken()
		if tok.Type != tc.expected {
			t.Errorf("For input %s: expected %s, got %s (val=%q)", tc.input, tc.expected, tok.Type, lexer.TokenValue(tok))
		}
		if tc.expected == TokenString {
			got := lexer.TokenValue(tok)
			if got != tc.val {
				t.Errorf("For input %s: expected value %q, got %q", tc.input, tc.val, got)
			}
		}
	}
}

func TestLexer_Numbers(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenType
		val      string
	}{
		{`123`, TokenNumber, `123`},
		{`-123`, TokenNumber, `-123`},
		{`0`, TokenNumber, `0`},
		{`3.14159`, TokenNumber, `3.14159`},
		{`-0.123`, TokenNumber, `-0.123`},
		{`1e10`, TokenNumber, `1e10`},
		{`3.14e+2`, TokenNumber, `3.14e+2`},
		{`1.5e-3`, TokenNumber, `1.5e-3`},
		// Error cases
		{`0123`, TokenError, `invalid number: leading zero not allowed`},
		{`1.`, TokenError, `invalid number: expected decimal digit, got ""`},
		{`-`, TokenError, `incomplete number`},
		{`1e`, TokenError, `invalid number: expected exponent value, got ""`},
	}

	for _, tc := range tests {
		lexer := NewLexer([]byte(tc.input))
		tok := lexer.NextToken()
		if tok.Type != tc.expected {
			t.Errorf("For input %s: expected %s, got %s (val=%q)", tc.input, tc.expected, tok.Type, lexer.TokenValue(tok))
		}
		if tc.expected == TokenNumber {
			got := lexer.TokenValue(tok)
			if got != tc.val {
				t.Errorf("For input %s: expected value %q, got %q", tc.input, tc.val, got)
			}
		}
	}
}

func TestLexer_Complex(t *testing.T) {
	input := `{
		"name": "John Doe",
		"age": 42,
		"alive": true,
		"tags": ["admin", "user"],
		"details": null
	}`

	expected := []TokenType{
		TokenBraceOpen,
		TokenString, TokenColon, TokenString, TokenComma,
		TokenString, TokenColon, TokenNumber, TokenComma,
		TokenString, TokenColon, TokenTrue, TokenComma,
		TokenString, TokenColon, TokenBracketOpen, TokenString, TokenComma, TokenString, TokenBracketClose, TokenComma,
		TokenString, TokenColon, TokenNull,
		TokenBraceClose,
		TokenEOF,
	}

	lexer := NewLexer([]byte(input))
	for i, typ := range expected {
		tok := lexer.NextToken()
		if tok.Type != typ {
			t.Fatalf("[%d] expected %s, got %s (val=%q, line=%d, col=%d)", i, typ, tok.Type, lexer.TokenValue(tok), tok.Line, tok.Column)
		}
	}
}
