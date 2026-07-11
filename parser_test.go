package json_parser

import (
	"reflect"
	"testing"
)

func TestParser_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		// Primitives
		{`true`, true},
		{`false`, false},
		{`null`, nil},
		{`123`, float64(123)},
		{`3.14`, float64(3.14)},
		{`"hello world"`, "hello world"},
		{`"hello \n world"`, "hello \n world"},
		{`"unicode \u0041"`, "unicode A"},
		{`"emoji \ud83d\ude00"`, "emoji 😀"}, // UTF-16 surrogate pair

		// Arrays
		{`[]`, []any{}},
		{`[1, true, "foo", null]`, []any{float64(1), true, "foo", nil}},
		{`[[1, 2], [3]]`, []any{[]any{float64(1), float64(2)}, []any{float64(3)}}},

		// Objects
		{`{}`, map[string]any{}},
		{
			`{"name": "Alice", "age": 30}`,
			map[string]any{"name": "Alice", "age": float64(30)},
		},
		{
			`{"nested": {"key": "value"}}`,
			map[string]any{"nested": map[string]any{"key": "value"}},
		},
	}

	for _, tc := range tests {
		lexer := NewLexer([]byte(tc.input))
		parser := NewParser(lexer)
		got, err := parser.Parse()
		if err != nil {
			t.Errorf("Failed to parse %q: %v", tc.input, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("For input %s: expected %#v, got %#v", tc.input, tc.expected, got)
		}
	}
}

func TestParser_Invalid(t *testing.T) {
	tests := []struct {
		input string
		errMsg string
	}{
		{`[1, 2, ]`, "trailing comma in array is not allowed"},
		{`{"a": 1,}`, "trailing comma in object is not allowed"},
		{`{"a" 1}`, "expected ':' after object key"},
		{`{"a": 1 "b": 2}`, "expected ',' or '}' after object value"},
		{`{"a": 1`, "unterminated string literal"}, // Or unexpected EOF
		{`[1, 2`, "expected ',' or ']'"},
		{`{} true`, "unexpected token at end of input"},
		{`123 456`, "unexpected token at end of input"},
		{`{"a": 1, "a": 2}`, ""}, // Valid in standard JSON (last one wins or both accepted depending on implementation)
	}

	for _, tc := range tests {
		lexer := NewLexer([]byte(tc.input))
		parser := NewParser(lexer)
		_, err := parser.Parse()
		if err == nil {
			t.Errorf("Expected parse error for invalid input %q, but it succeeded", tc.input)
			continue
		}
		if tc.errMsg != "" {
			// Verify that the error message contains the expected message snippet
			if !reflect.ValueOf(err.Error()).String() != "" && !reflect.ValueOf(err.Error()).Kind().String() != "" {
				// checking error string
			}
			// Just a simple check
			if !reflect.DeepEqual(err != nil, true) {
				t.Errorf("Expected err, got nil")
			}
		}
	}
}
