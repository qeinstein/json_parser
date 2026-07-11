package json_parser

import (
	"errors"
	"reflect"
)

// Parse takes a JSON byte slice and returns the parsed Go data structures (map[string]any, []any, float64, etc.)
func Parse(data []byte) (any, error) {
	lexer := NewLexer(data)
	parser := NewParser(lexer)
	return parser.Parse()
}

// Unmarshal parses the JSON-encoded data and stores the result in the value pointed to by v.
// If v is nil or not a pointer, Unmarshal returns an error.
//
// When v points to a struct, slice, map, or primitive, Unmarshal uses an optimized
// direct decoder that skips intermediate allocations. For interface{}/any targets,
// it falls back to the dynamic Parse() path.
func Unmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("unmarshal target must be a non-nil pointer")
	}

	// Use the fast direct decoder for concrete types
	return directUnmarshal(data, v)
}
