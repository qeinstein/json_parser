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
	r                     io.Reader
	buf                   []byte
	offset                int
	disallowUnknownFields bool
	useNumber             bool
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

// UseNumber causes the Decoder to decode numbers into a Number instead of a float64
// when decoding into interface{}/any.
func (dec *Decoder) UseNumber() {
	dec.useNumber = true
}

// InputOffset returns the input stream byte offset of the current decoder position.
func (dec *Decoder) InputOffset() int64 {
	return int64(dec.offset)
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

	opts := decoder.DecodeOptions{
		DisallowUnknownFields: dec.disallowUnknownFields,
		UseNumber:             dec.useNumber,
	}

	return decoder.UnmarshalWithOptions(valueBytes, v, opts)
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
	w          io.Writer
	escapeHTML bool
	err        error
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w, escapeHTML: true}
}

// SetEscapeHTML specifies whether problematic HTML characters should be escaped
// inside JSON strings.
func (enc *Encoder) SetEscapeHTML(on bool) {
	enc.escapeHTML = on
}

// Encode writes the JSON encoding of v to the stream, followed by a newline.
func (enc *Encoder) Encode(v any) error {
	data, err := encoder.MarshalWithOptions(v, enc.escapeHTML)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = enc.w.Write(data)
	return err
}

// HTMLEscape appends to dst the JSON-encoded src with <, >, &, \u2028, and \u2029
// characters inside string literals escaped as \u003c, \u003e, \u0026, \u2028, and \u2029.
func HTMLEscape(dst *bytes.Buffer, src []byte) {
	for i := 0; i < len(src); i++ {
		b := src[i]
		switch b {
		case '<':
			dst.WriteString(`\u003c`)
		case '>':
			dst.WriteString(`\u003e`)
		case '&':
			dst.WriteString(`\u0026`)
		case 0xe2:
			if i+2 < len(src) && src[i+1] == 0x80 && (src[i+2] == 0xa8 || src[i+2] == 0xa9) {
				if src[i+2] == 0xa8 {
					dst.WriteString(`\u2028`)
				} else {
					dst.WriteString(`\u2029`)
				}
				i += 2
			} else {
				dst.WriteByte(b)
			}
		default:
			dst.WriteByte(b)
		}
	}
}

// Compact appends to dst the JSON-encoded src with insignificant space characters elided.
func Compact(dst *bytes.Buffer, src []byte) error {
	l := lexer.NewLexer(src)
	for {
		tok := l.NextToken()
		if tok.Type == lexer.TokenError {
			return &decoder.ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}
		if tok.Type == lexer.TokenEOF {
			break
		}
		if tok.Type == lexer.TokenString {
			dst.WriteByte('"')
			dst.Write(l.TokenBytes(tok))
			dst.WriteByte('"')
		} else {
			dst.Write(l.TokenBytes(tok))
		}
	}
	return nil
}

// Indent appends to dst the JSON-encoded src with indentation and formatting.
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	l := lexer.NewLexer(src)
	depth := 0
	needNewline := false

	writeIndent := func() {
		dst.WriteString(prefix)
		for i := 0; i < depth; i++ {
			dst.WriteString(indent)
		}
	}

	nextChar := func(lx *lexer.Lexer) byte {
		pos := lx.Pos
		for pos < len(lx.Input) {
			b := lx.Input[pos]
			if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
				pos++
				continue
			}
			return b
		}
		return 0
	}

	for {
		tok := l.NextToken()
		if tok.Type == lexer.TokenError {
			return &decoder.ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}
		if tok.Type == lexer.TokenEOF {
			break
		}

		switch tok.Type {
		case lexer.TokenBraceOpen, lexer.TokenBracketOpen:
			if needNewline {
				dst.WriteByte('\n')
				writeIndent()
			}
			// Check if next is closing brace/bracket to format empty container on same line
			b := nextChar(l)
			if (tok.Type == lexer.TokenBraceOpen && b == '}') || (tok.Type == lexer.TokenBracketOpen && b == ']') {
				dst.Write(l.TokenBytes(tok))
				closeTok := l.NextToken()
				dst.Write(l.TokenBytes(closeTok))
				needNewline = false
			} else {
				dst.Write(l.TokenBytes(tok))
				depth++
				needNewline = true
			}

		case lexer.TokenBraceClose, lexer.TokenBracketClose:
			depth--
			dst.WriteByte('\n')
			writeIndent()
			dst.Write(l.TokenBytes(tok))
			needNewline = false

		case lexer.TokenComma:
			dst.WriteByte(',')
			dst.WriteByte('\n')
			writeIndent()
			needNewline = false

		case lexer.TokenColon:
			dst.WriteByte(':')
			dst.WriteByte(' ')
			needNewline = false

		default:
			if needNewline {
				dst.WriteByte('\n')
				writeIndent()
				needNewline = false
			}
			if tok.Type == lexer.TokenString {
				dst.WriteByte('"')
				dst.Write(l.TokenBytes(tok))
				dst.WriteByte('"')
			} else {
				dst.Write(l.TokenBytes(tok))
			}
		}
	}
	return nil
}
