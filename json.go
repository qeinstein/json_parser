package json_parser

import (
	"json_parser/decoder"
	"json_parser/encoder"
	"json_parser/lexer"
	"json_parser/parser"
	"json_parser/types"
)

// Exported Types from types package using type aliases
type (
	Marshaler   = types.Marshaler
	Unmarshaler = types.Unmarshaler
	RawMessage  = types.RawMessage
	Number      = types.Number
)

// ParseError is exported from decoder package using a type alias
type ParseError = decoder.ParseError

// Parse takes a JSON byte slice and returns the parsed Go data structures.
func Parse(data []byte) (any, error) {
	l := lexer.NewLexer(data)
	p := parser.NewParser(l)
	return p.Parse()
}

// Unmarshal parses the JSON-encoded data and stores the result in the value pointed to by v.
func Unmarshal(data []byte, v any) error {
	return decoder.Unmarshal(data, v)
}

// Marshal returns the JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	return encoder.Marshal(v)
}

// Valid reports whether data is a valid JSON encoding.
func Valid(data []byte) bool {
	l := lexer.NewLexer(data)
	p := parser.NewParser(l)
	_, err := p.Parse()
	return err == nil
}
