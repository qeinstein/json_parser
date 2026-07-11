package json_parser

import (
	"bytes"
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

// ParseError and DecodeOptions are exported from decoder package using type aliases
type (
	ParseError    = decoder.ParseError
	DecodeOptions = decoder.DecodeOptions
)

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

// UnmarshalWithOptions parses the JSON-encoded data with options and stores the result in the value pointed to by v.
func UnmarshalWithOptions(data []byte, v any, opts DecodeOptions) error {
	return decoder.UnmarshalWithOptions(data, v, opts)
}

// Marshal returns the JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	return encoder.Marshal(v)
}

// MarshalWithOptions returns the JSON encoding of v with configuration options.
func MarshalWithOptions(v any, escapeHTML bool) ([]byte, error) {
	return encoder.MarshalWithOptions(v, escapeHTML)
}

// Valid reports whether data is a valid JSON encoding.
func Valid(data []byte) bool {
	l := lexer.NewLexer(data)
	p := parser.NewParser(l)
	_, err := p.Parse()
	return err == nil
}

// MarshalIndent is like Marshal but applies Indent to format the output.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	b, err := encoder.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = Indent(&buf, b, prefix, indent)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
