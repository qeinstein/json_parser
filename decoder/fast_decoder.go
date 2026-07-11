package decoder

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"io"
	"json_parser/lexer"
	"json_parser/types"
	"reflect"
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

// DecodeOptions represents options for decoding JSON data.
type DecodeOptions struct {
	DisallowUnknownFields bool
	UseNumber             bool
}

// Unmarshal parses the JSON-encoded data and stores the result in the value pointed to by v.
func Unmarshal(data []byte, v any) error {
	return UnmarshalWithOptions(data, v, DecodeOptions{})
}

// UnmarshalWithOptions parses the JSON-encoded data with options and stores the result in the value pointed to by v.
func UnmarshalWithOptions(data []byte, v any, opts DecodeOptions) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("unmarshal target must be a non-nil pointer")
	}

	l := lexer.NewLexer(data)
	tok := l.NextToken()

	if tok.Type == lexer.TokenError {
		return &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
	}
	if tok.Type == lexer.TokenEOF {
		return &ParseError{Message: "empty JSON input", Line: tok.Line, Column: tok.Column}
	}

	next, err := directDecode(l, tok, rv.Elem(), 0, opts)
	if err != nil {
		return err
	}

	if next.Type == lexer.TokenError {
		return &ParseError{Message: l.Error(), Line: next.Line, Column: next.Column}
	}
	if next.Type != lexer.TokenEOF {
		return &ParseError{
			Message: fmt.Sprintf("unexpected token at end of input: %s", next.Type),
			Line:    next.Line, Column: next.Column,
		}
	}

	return nil
}

var (
	unmarshalerType     = reflect.TypeOf((*types.Unmarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// directDecode decodes one JSON value starting at tok into dest.
// Returns the next unconsumed token.
func directDecode(l *lexer.Lexer, tok lexer.Token, dest reflect.Value, depth int, opts DecodeOptions) (lexer.Token, error) {
	if depth > 1000 {
		return tok, &ParseError{Message: "exceeded max depth limit", Line: tok.Line, Column: tok.Column}
	}

	t := dest.Type()
	if t.Implements(unmarshalerType) && dest.CanInterface() {
		u := dest.Interface().(types.Unmarshaler)
		raw, next, err := extractRawJSON(l, tok)
		if err != nil {
			return next, err
		}
		return next, u.UnmarshalJSON(raw)
	}
	if t.Implements(textUnmarshalerType) && dest.CanInterface() {
		ut := dest.Interface().(encoding.TextUnmarshaler)
		var txt []byte
		var next lexer.Token
		if tok.Type == lexer.TokenString {
			txt = []byte(unescapeTokenString(l.Input, tok))
			next = l.NextToken()
		} else {
			var raw []byte
			var err error
			raw, next, err = extractRawJSON(l, tok)
			if err != nil {
				return next, err
			}
			txt = raw
		}
		return next, ut.UnmarshalText(txt)
	}

	if dest.CanAddr() {
		ta := dest.Addr().Type()
		if ta.Implements(unmarshalerType) {
			u := dest.Addr().Interface().(types.Unmarshaler)
			raw, next, err := extractRawJSON(l, tok)
			if err != nil {
				return next, err
			}
			return next, u.UnmarshalJSON(raw)
		}
		if ta.Implements(textUnmarshalerType) {
			ut := dest.Addr().Interface().(encoding.TextUnmarshaler)
			var txt []byte
			var next lexer.Token
			if tok.Type == lexer.TokenString {
				txt = []byte(unescapeTokenString(l.Input, tok))
				next = l.NextToken()
			} else {
				var raw []byte
				var err error
				raw, next, err = extractRawJSON(l, tok)
				if err != nil {
					return next, err
				}
				txt = raw
			}
			return next, ut.UnmarshalText(txt)
		}
	}

	// Dereference pointers, allocating if nil
	for dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		dest = dest.Elem()
	}

	// If dest is interface{}, decode to a generic Go value
	if dest.Kind() == reflect.Interface {
		val, next, err := decodeToAny(l, tok, depth+1, opts)
		if err != nil {
			return next, err
		}
		if val != nil {
			dest.Set(reflect.ValueOf(val))
		} else {
			dest.Set(reflect.Zero(dest.Type()))
		}
		return next, nil
	}

	switch tok.Type {
	case lexer.TokenNull:
		dest.Set(reflect.Zero(dest.Type()))
		return l.NextToken(), nil

	case lexer.TokenTrue:
		if dest.Kind() == reflect.Bool {
			dest.SetBool(true)
		}
		return l.NextToken(), nil

	case lexer.TokenFalse:
		if dest.Kind() == reflect.Bool {
			dest.SetBool(false)
		}
		return l.NextToken(), nil

	case lexer.TokenString:
		if dest.Kind() == reflect.String {
			dest.SetString(unescapeTokenString(l.Input, tok))
		}
		return l.NextToken(), nil

	case lexer.TokenNumber:
		return directDecodeNumber(l, tok, dest)

	case lexer.TokenBraceOpen:
		if dest.Kind() == reflect.Struct {
			return directDecodeObject(l, dest, depth+1, opts)
		}
		if dest.Kind() == reflect.Map {
			return directDecodeMap(l, dest, depth+1, opts)
		}
		return skipObject(l, depth+1)

	case lexer.TokenBracketOpen:
		if dest.Kind() == reflect.Slice {
			return directDecodeArray(l, dest, depth+1, opts)
		}
		return skipArray(l, depth+1)

	case lexer.TokenError:
		return tok, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}

	default:
		return tok, &ParseError{
			Message: fmt.Sprintf("unexpected token %s", tok.Type),
			Line:    tok.Line, Column: tok.Column,
		}
	}
}

// directDecodeNumber parses a number token directly into the destination field
// using the most efficient conversion for the target type.
func directDecodeNumber(l *lexer.Lexer, tok lexer.Token, dest reflect.Value) (lexer.Token, error) {
	raw := l.Input[tok.Start:tok.End]
	next := l.NextToken()

	switch dest.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Fast path: parse integer directly from bytes (no string allocation)
		if n, ok := parseIntFromBytes(raw); ok {
			dest.SetInt(n)
			return next, nil
		}
		// Fallback: number has decimal/exponent, go through float
		f, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return next, &ParseError{Message: fmt.Sprintf("invalid number: %v", err), Line: tok.Line, Column: tok.Column}
		}
		dest.SetInt(int64(f))
		return next, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, ok := parseUintFromBytes(raw); ok {
			dest.SetUint(n)
			return next, nil
		}
		f, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return next, &ParseError{Message: fmt.Sprintf("invalid number: %v", err), Line: tok.Line, Column: tok.Column}
		}
		dest.SetUint(uint64(f))
		return next, nil

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return next, &ParseError{Message: fmt.Sprintf("invalid number: %v", err), Line: tok.Line, Column: tok.Column}
		}
		dest.SetFloat(f)
		return next, nil

	case reflect.String:
		dest.SetString(string(raw))
		return next, nil

	default:
		// Unsupported target type for number, skip
		return next, nil
	}
}

// directDecodeObject reads a JSON object and writes key-value pairs directly into struct fields.
func directDecodeObject(l *lexer.Lexer, dest reflect.Value, depth int, opts DecodeOptions) (lexer.Token, error) {
	// '{' was already identified; consume next token
	tok := l.NextToken()

	if tok.Type == lexer.TokenBraceClose {
		return l.NextToken(), nil // empty object
	}

	meta := GetStructMeta(dest.Type())

	for {
		if tok.Type == lexer.TokenError {
			return tok, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}
		if tok.Type != lexer.TokenString {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected string key in object, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		// Match key to struct field using zero-copy byte comparison.
		// Go compiler optimizes map[string] lookup with string([]byte) to avoid allocation.
		keyBytes := l.Input[tok.Start:tok.End]
		metaIdx, found := meta.ByKey[string(keyBytes)]

		// If the key has escape sequences, the raw bytes won't match.
		// Fallback: unescape and try again.
		if !found && bytes.IndexByte(keyBytes, '\\') != -1 {
			unescaped := unescapeTokenString(l.Input, tok)
			metaIdx, found = meta.ByKey[unescaped]
		}

		if !found {
			keyStr := string(keyBytes)
			if bytes.IndexByte(keyBytes, '\\') != -1 {
				keyStr = unescapeTokenString(l.Input, tok)
			}
			metaIdx, found = meta.ByFoldedKey[strings.ToLower(keyStr)]
		}

		tok = l.NextToken() // expect ':'
		if tok.Type != lexer.TokenColon {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ':' after object key, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken() // value token

		if found {
			// Decode value directly into the struct field using index path
			field := FieldByIndex(dest, meta.Fields[metaIdx].Index)
			var err error
			tok, err = directDecode(l, tok, field, depth+1, opts)
			if err != nil {
				return tok, err
			}
		} else {
			if opts.DisallowUnknownFields {
				return tok, fmt.Errorf("json: unknown field %q", string(keyBytes))
			}
			// Unknown field: skip the value entirely (no allocations)
			var err error
			tok, err = skipValue(l, tok, depth+1)
			if err != nil {
				return tok, err
			}
		}

		if tok.Type == lexer.TokenBraceClose {
			return l.NextToken(), nil
		}

		if tok.Type == lexer.TokenError {
			return tok, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}

		if tok.Type != lexer.TokenComma {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or '}' after object value, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken() // next key

		if tok.Type == lexer.TokenBraceClose {
			return tok, &ParseError{
				Message: "trailing comma in object is not allowed",
				Line:    tok.Line, Column: tok.Column,
			}
		}
	}
}

// directDecodeMap reads a JSON object into a map[string]V.
func directDecodeMap(l *lexer.Lexer, dest reflect.Value, depth int, opts DecodeOptions) (lexer.Token, error) {
	tok := l.NextToken()

	if dest.IsNil() {
		dest.Set(reflect.MakeMap(dest.Type()))
	}

	if tok.Type == lexer.TokenBraceClose {
		return l.NextToken(), nil
	}

	elemType := dest.Type().Elem()

	for {
		if tok.Type != lexer.TokenString {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected string key in object, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		key := unescapeTokenString(l.Input, tok)

		tok = l.NextToken() // ':'
		if tok.Type != lexer.TokenColon {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ':' after object key, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken() // value

		newElem := reflect.New(elemType).Elem()
		var err error
		tok, err = directDecode(l, tok, newElem, depth+1, opts)
		if err != nil {
			return tok, err
		}
		dest.SetMapIndex(reflect.ValueOf(key), newElem)

		if tok.Type == lexer.TokenBraceClose {
			return l.NextToken(), nil
		}
		if tok.Type != lexer.TokenComma {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or '}' after object value, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()
	}
}

// directDecodeArray reads a JSON array and writes elements directly into a slice.
func directDecodeArray(l *lexer.Lexer, dest reflect.Value, depth int, opts DecodeOptions) (lexer.Token, error) {
	tok := l.NextToken()

	if tok.Type == lexer.TokenBracketClose {
		dest.Set(reflect.MakeSlice(dest.Type(), 0, 0))
		return l.NextToken(), nil
	}

	elemType := dest.Type().Elem()
	slice := reflect.MakeSlice(dest.Type(), 0, 4) // pre-allocate small capacity

	for {
		newElem := reflect.New(elemType).Elem()
		var err error
		tok, err = directDecode(l, tok, newElem, depth+1, opts)
		if err != nil {
			return tok, err
		}
		slice = reflect.Append(slice, newElem)

		if tok.Type == lexer.TokenBracketClose {
			dest.Set(slice)
			return l.NextToken(), nil
		}

		if tok.Type == lexer.TokenError {
			return tok, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}

		if tok.Type != lexer.TokenComma {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or ']' after array element, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken()

		if tok.Type == lexer.TokenBracketClose {
			return tok, &ParseError{
				Message: "trailing comma in array is not allowed",
				Line:    tok.Line, Column: tok.Column,
			}
		}
	}
}

// Skip functions advance past a JSON value without allocating.

func skipValue(l *lexer.Lexer, tok lexer.Token, depth int) (lexer.Token, error) {
	if depth > 1000 {
		return tok, &ParseError{Message: "exceeded max depth limit", Line: tok.Line, Column: tok.Column}
	}
	switch tok.Type {
	case lexer.TokenString, lexer.TokenNumber, lexer.TokenTrue, lexer.TokenFalse, lexer.TokenNull:
		return l.NextToken(), nil
	case lexer.TokenBraceOpen:
		return skipObject(l, depth+1)
	case lexer.TokenBracketOpen:
		return skipArray(l, depth+1)
	case lexer.TokenError:
		return tok, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
	default:
		return tok, &ParseError{
			Message: fmt.Sprintf("unexpected token %s", tok.Type),
			Line:    tok.Line, Column: tok.Column,
		}
	}
}

func skipObject(l *lexer.Lexer, depth int) (lexer.Token, error) {
	if depth > 1000 {
		return lexer.Token{}, &ParseError{Message: "exceeded max depth limit", Line: l.Line, Column: l.Column}
	}
	tok := l.NextToken()
	if tok.Type == lexer.TokenBraceClose {
		return l.NextToken(), nil
	}
	for {
		// skip key
		if tok.Type != lexer.TokenString {
			return tok, &ParseError{Message: "expected string key", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken() // ':'
		if tok.Type != lexer.TokenColon {
			return tok, &ParseError{Message: "expected ':'", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken() // value
		var err error
		tok, err = skipValue(l, tok, depth+1)
		if err != nil {
			return tok, err
		}
		if tok.Type == lexer.TokenBraceClose {
			return l.NextToken(), nil
		}
		if tok.Type != lexer.TokenComma {
			return tok, &ParseError{Message: "expected ',' or '}'", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken()
	}
}

func skipArray(l *lexer.Lexer, depth int) (lexer.Token, error) {
	if depth > 1000 {
		return lexer.Token{}, &ParseError{Message: "exceeded max depth limit", Line: l.Line, Column: l.Column}
	}
	tok := l.NextToken()
	if tok.Type == lexer.TokenBracketClose {
		return l.NextToken(), nil
	}
	for {
		var err error
		tok, err = skipValue(l, tok, depth+1)
		if err != nil {
			return tok, err
		}
		if tok.Type == lexer.TokenBracketClose {
			return l.NextToken(), nil
		}
		if tok.Type != lexer.TokenComma {
			return tok, &ParseError{Message: "expected ',' or ']'", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken()
	}
}

// Generic decode functions for interface{}/any destinations.

func decodeToAny(l *lexer.Lexer, tok lexer.Token, depth int, opts DecodeOptions) (any, lexer.Token, error) {
	if depth > 1000 {
		return nil, tok, &ParseError{Message: "exceeded max depth limit", Line: tok.Line, Column: tok.Column}
	}
	switch tok.Type {
	case lexer.TokenNull:
		return nil, l.NextToken(), nil
	case lexer.TokenTrue:
		return true, l.NextToken(), nil
	case lexer.TokenFalse:
		return false, l.NextToken(), nil
	case lexer.TokenString:
		s := unescapeTokenString(l.Input, tok)
		return s, l.NextToken(), nil
	case lexer.TokenNumber:
		raw := l.Input[tok.Start:tok.End]
		if opts.UseNumber {
			return types.Number(string(raw)), l.NextToken(), nil
		}
		f, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return nil, tok, &ParseError{Message: fmt.Sprintf("invalid number: %v", err), Line: tok.Line, Column: tok.Column}
		}
		return f, l.NextToken(), nil
	case lexer.TokenBraceOpen:
		return decodeObjectToAny(l, depth+1, opts)
	case lexer.TokenBracketOpen:
		return decodeArrayToAny(l, depth+1, opts)
	case lexer.TokenError:
		return nil, tok, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
	default:
		return nil, tok, &ParseError{
			Message: fmt.Sprintf("unexpected token %s", tok.Type),
			Line:    tok.Line, Column: tok.Column,
		}
	}
}

func decodeObjectToAny(l *lexer.Lexer, depth int, opts DecodeOptions) (any, lexer.Token, error) {
	if depth > 1000 {
		return nil, lexer.Token{}, &ParseError{Message: "exceeded max depth limit", Line: l.Line, Column: l.Column}
	}
	tok := l.NextToken()
	if tok.Type == lexer.TokenBraceClose {
		return map[string]any{}, l.NextToken(), nil
	}
	m := make(map[string]any)
	for {
		if tok.Type != lexer.TokenString {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected string key, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		key := unescapeTokenString(l.Input, tok)

		tok = l.NextToken()
		if tok.Type != lexer.TokenColon {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected ':', got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()

		var val any
		var err error
		val, tok, err = decodeToAny(l, tok, depth+1, opts)
		if err != nil {
			return nil, tok, err
		}
		m[key] = val

		if tok.Type == lexer.TokenBraceClose {
			return m, l.NextToken(), nil
		}
		if tok.Type != lexer.TokenComma {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or '}', got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()
	}
}

func decodeArrayToAny(l *lexer.Lexer, depth int, opts DecodeOptions) (any, lexer.Token, error) {
	if depth > 1000 {
		return nil, lexer.Token{}, &ParseError{Message: "exceeded max depth limit", Line: l.Line, Column: l.Column}
	}
	tok := l.NextToken()
	if tok.Type == lexer.TokenBracketClose {
		return []any{}, l.NextToken(), nil
	}
	arr := make([]any, 0, 4)
	for {
		var val any
		var err error
		val, tok, err = decodeToAny(l, tok, depth+1, opts)
		if err != nil {
			return nil, tok, err
		}
		arr = append(arr, val)

		if tok.Type == lexer.TokenBracketClose {
			return arr, l.NextToken(), nil
		}
		if tok.Type != lexer.TokenComma {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or ']', got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()
	}
}

// String helpers for escape sequence processing.

// unescapeTokenString converts raw token bytes into a Go string,
// processing JSON escape sequences. Optimized for the common case
// where no escape sequences are present (single allocation, no processing).
func unescapeTokenString(input []byte, tok lexer.Token) string {
	raw := input[tok.Start:tok.End]

	// Fast path: no backslash means no escape sequences
	if bytes.IndexByte(raw, '\\') == -1 {
		return string(raw)
	}

	// Slow path: process escape sequences
	var sb strings.Builder
	sb.Grow(len(raw))

	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			sb.WriteByte(raw[i])
			continue
		}
		i++
		if i >= len(raw) {
			break
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
			if i+4 >= len(raw) {
				break
			}
			val, ok := parseHex4(raw[i+1 : i+5])
			if !ok {
				break
			}
			i += 4

			// Check for UTF-16 surrogate pair
			if val >= 0xD800 && val <= 0xDBFF && i+2 < len(raw) && raw[i+1] == '\\' && raw[i+2] == 'u' {
				if i+6 < len(raw) {
					trailVal, ok := parseHex4(raw[i+3 : i+7])
					if ok && trailVal >= 0xDC00 && trailVal <= 0xDFFF {
						r := utf16.DecodeRune(rune(val), rune(trailVal))
						sb.WriteRune(r)
						i += 6
						continue
					}
				}
			}

			sb.WriteRune(rune(val))
		}
	}

	return sb.String()
}

// parseHex4 parses exactly 4 hex digits from a byte slice without allocation.
func parseHex4(b []byte) (uint16, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var val uint16
	for i := 0; i < 4; i++ {
		val <<= 4
		c := b[i]
		switch {
		case c >= '0' && c <= '9':
			val |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			val |= uint16(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			val |= uint16(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return val, true
}

// parseIntFromBytes parses a simple integer (optional '-' followed by digits)
// directly from bytes without converting to string. Returns false if the
// number has decimal points or exponents.
func parseIntFromBytes(b []byte) (int64, bool) {
	if len(b) == 0 || len(b) > 19 {
		return 0, false
	}
	neg := false
	i := 0
	if b[0] == '-' {
		neg = true
		i = 1
	}
	if i >= len(b) {
		return 0, false
	}
	var n int64
	for ; i < len(b); i++ {
		c := b[i]
		if c < '0' || c > '9' {
			return 0, false // has '.', 'e', 'E' etc.
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

// parseUintFromBytes parses a simple unsigned integer directly from bytes.
func parseUintFromBytes(b []byte) (uint64, bool) {
	if len(b) == 0 || len(b) > 20 {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	return n, true
}

// extractRawJSON returns the raw JSON bytes corresponding to the value starting at tok,
// and returns the next unconsumed token.
func extractRawJSON(l *lexer.Lexer, tok lexer.Token) ([]byte, lexer.Token, error) {
	startPos := tok.Start
	if tok.Type == lexer.TokenString {
		startPos-- // include the opening quote '"'
	}

	var endPos int
	var next lexer.Token
	var err error

	switch tok.Type {
	case lexer.TokenString:
		endPos = tok.End + 1
		next = l.NextToken()
	case lexer.TokenNumber, lexer.TokenTrue, lexer.TokenFalse, lexer.TokenNull:
		endPos = tok.End
		next = l.NextToken()
	case lexer.TokenBraceOpen, lexer.TokenBracketOpen:
		var end int
		if tok.Type == lexer.TokenBraceOpen {
			end, err = findMatchingClose(l, lexer.TokenBraceOpen, lexer.TokenBraceClose)
		} else {
			end, err = findMatchingClose(l, lexer.TokenBracketOpen, lexer.TokenBracketClose)
		}
		if err != nil {
			return nil, lexer.Token{}, err
		}
		endPos = end
		next = l.NextToken()
	default:
		return nil, lexer.Token{}, fmt.Errorf("unexpected token for raw JSON extraction: %s", tok.Type)
	}

	raw := l.Input[startPos:endPos]
	return raw, next, nil
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
			return 0, &ParseError{Message: l.Error(), Line: tok.Line, Column: tok.Column}
		}
	}
	return l.Pos, nil
}
