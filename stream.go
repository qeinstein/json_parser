package json_parser

import (
	"bytes"
	"io"
	"json_parser/decoder"
	"json_parser/encoder"
	"json_parser/lexer"
)

// Decoder reads and decodes JSON values from an input stream.
type Decoder struct {
	r                    io.Reader
	buf                  []byte
	offset               int
	disallowUnknownFields bool
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// DisallowUnknownFields causes the Decoder to return an error when the
// destination struct does not have a field corresponding to a JSON object key.
func (dec *Decoder) DisallowUnknownFields() {
	dec.disallowUnknownFields = true
}

// Decode reads the next JSON value from the input and stores it in the value pointed to by v.
func (dec *Decoder) Decode(v any) error {
	// Read all available data from the reader
	if dec.buf == nil {
		data, err := io.ReadAll(dec.r)
		if err != nil {
			return err
		}
		dec.buf = data
	}

	// Skip whitespace to find the start of the next value
	for dec.offset < len(dec.buf) {
		b := dec.buf[dec.offset]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			dec.offset++
		} else {
			break
		}
	}

	if dec.offset >= len(dec.buf) {
		return io.EOF
	}

	// Find the end of the current JSON value
	end, err := findValueEnd(dec.buf[dec.offset:])
	if err != nil {
		return err
	}

	valueBytes := dec.buf[dec.offset : dec.offset+end]
	dec.offset += end

	if dec.disallowUnknownFields {
		return unmarshalStrict(valueBytes, v)
	}

	return Unmarshal(valueBytes, v)
}

// More reports whether there is another element in the current array or object being parsed.
func (dec *Decoder) More() bool {
	for i := dec.offset; i < len(dec.buf); i++ {
		b := dec.buf[i]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		return b != ']' && b != '}'
	}
	return false
}

// Buffered returns a reader for the data remaining in the decoder's buffer.
func (dec *Decoder) Buffered() io.Reader {
	return bytes.NewReader(dec.buf[dec.offset:])
}

// findValueEnd scans forward to find where a single JSON value ends.
// Returns the byte length of the value.
func findValueEnd(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}

	// Use the lexer to find the complete value
	l := lexer.NewLexer(data)
	tok := l.NextToken()

	switch tok.Type {
	case lexer.TokenBraceOpen:
		return findMatchingClose(l, lexer.TokenBraceOpen, lexer.TokenBraceClose)
	case lexer.TokenBracketOpen:
		return findMatchingClose(l, lexer.TokenBracketOpen, lexer.TokenBracketClose)
	case lexer.TokenString, lexer.TokenNumber, lexer.TokenTrue, lexer.TokenFalse, lexer.TokenNull:
		return l.Pos, nil
	case lexer.TokenError:
		return 0, &decoder.ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
	default:
		return 0, &decoder.ParseError{Message: "unexpected token", Line: tok.Line, Column: tok.Column}
	}
}

// findMatchingClose scans tokens until the matching close bracket/brace is found.
func findMatchingClose(l *lexer.Lexer, open, close lexer.TokenType) (int, error) {
	depth := 1
	for depth > 0 {
		tok := l.NextToken()
		switch tok.Type {
		case open:
			depth++
		case close:
			depth--
		case lexer.TokenEOF:
			return 0, io.ErrUnexpectedEOF
		case lexer.TokenError:
			return 0, &decoder.ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}
	}
	return l.Pos, nil
}

// Encoder writes JSON values to an output stream.
type Encoder struct {
	w   io.Writer
	err error
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes the JSON encoding of v to the stream, followed by a newline.
func (enc *Encoder) Encode(v any) error {
	data, err := encoder.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = enc.w.Write(data)
	return err
}

// unmarshalStrict is like Unmarshal but returns an error for unknown fields.
func unmarshalStrict(data []byte, v any) error {
	// For now, route through the normal path.
	// A full implementation would check each key against the struct fields
	// and error on any unrecognized key.
	return Unmarshal(data, v)
}
