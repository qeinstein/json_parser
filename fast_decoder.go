package json_parser

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf16"
)

// directUnmarshal is the fast path: it scans tokens and writes directly into the
// target struct/slice/map without building an intermediate map[string]any.
func directUnmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("unmarshal target must be a non-nil pointer")
	}

	l := &Lexer{input: data, line: 1, column: 1}
	tok := l.NextToken()

	if tok.Type == TokenError {
		return &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}
	}
	if tok.Type == TokenEOF {
		return &ParseError{Message: "empty JSON input", Line: tok.Line, Column: tok.Column}
	}

	next, err := directDecode(l, tok, rv.Elem())
	if err != nil {
		return err
	}

	if next.Type == TokenError {
		return &ParseError{Message: l.err, Line: next.Line, Column: next.Column}
	}
	if next.Type != TokenEOF {
		return &ParseError{
			Message: fmt.Sprintf("unexpected token at end of input: %s", next.Type),
			Line:    next.Line, Column: next.Column,
		}
	}

	return nil
}

// directDecode decodes one JSON value starting at tok into dest.
// Returns the next unconsumed token.
func directDecode(l *Lexer, tok Token, dest reflect.Value) (Token, error) {
	// Dereference pointers, allocating if nil
	for dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		dest = dest.Elem()
	}

	// If dest is interface{}, decode to a generic Go value
	if dest.Kind() == reflect.Interface {
		val, next, err := decodeToAny(l, tok)
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
	case TokenNull:
		dest.Set(reflect.Zero(dest.Type()))
		return l.NextToken(), nil

	case TokenTrue:
		if dest.Kind() == reflect.Bool {
			dest.SetBool(true)
		}
		return l.NextToken(), nil

	case TokenFalse:
		if dest.Kind() == reflect.Bool {
			dest.SetBool(false)
		}
		return l.NextToken(), nil

	case TokenString:
		if dest.Kind() == reflect.String {
			dest.SetString(unescapeTokenString(l.input, tok))
		}
		return l.NextToken(), nil

	case TokenNumber:
		return directDecodeNumber(l, tok, dest)

	case TokenBraceOpen:
		if dest.Kind() == reflect.Struct {
			return directDecodeObject(l, dest)
		}
		if dest.Kind() == reflect.Map {
			return directDecodeMap(l, dest)
		}
		return skipObject(l)

	case TokenBracketOpen:
		if dest.Kind() == reflect.Slice {
			return directDecodeArray(l, dest)
		}
		return skipArray(l)

	case TokenError:
		return tok, &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}

	default:
		return tok, &ParseError{
			Message: fmt.Sprintf("unexpected token %s", tok.Type),
			Line:    tok.Line, Column: tok.Column,
		}
	}
}

// directDecodeNumber parses a number token directly into the destination field
// using the most efficient conversion for the target type.
func directDecodeNumber(l *Lexer, tok Token, dest reflect.Value) (Token, error) {
	raw := l.input[tok.Start:tok.End]
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

	default:
		// Unsupported target type for number, skip
		return next, nil
	}
}

// directDecodeObject reads a JSON object and writes key-value pairs directly into struct fields.
func directDecodeObject(l *Lexer, dest reflect.Value) (Token, error) {
	// '{' was already identified; consume next token
	tok := l.NextToken()

	if tok.Type == TokenBraceClose {
		return l.NextToken(), nil // empty object
	}

	meta := getStructMeta(dest.Type())

	for {
		if tok.Type == TokenError {
			return tok, &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}
		}
		if tok.Type != TokenString {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected string key in object, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		// Match key to struct field using zero-copy byte comparison.
		// Go compiler optimizes map[string] lookup with string([]byte) to avoid allocation.
		keyBytes := l.input[tok.Start:tok.End]
		fieldIdx, found := meta.byKey[string(keyBytes)]

		// If the key has escape sequences, the raw bytes won't match.
		// Fallback: unescape and try again.
		if !found && bytes.IndexByte(keyBytes, '\\') != -1 {
			unescaped := unescapeTokenString(l.input, tok)
			fieldIdx, found = meta.byKey[unescaped]
		}

		tok = l.NextToken() // expect ':'
		if tok.Type != TokenColon {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ':' after object key, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken() // value token

		if found {
			// Decode value directly into the struct field
			var err error
			tok, err = directDecode(l, tok, dest.Field(fieldIdx))
			if err != nil {
				return tok, err
			}
		} else {
			// Unknown field: skip the value entirely (no allocations)
			var err error
			tok, err = skipValue(l, tok)
			if err != nil {
				return tok, err
			}
		}

		if tok.Type == TokenBraceClose {
			return l.NextToken(), nil
		}

		if tok.Type == TokenError {
			return tok, &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}
		}

		if tok.Type != TokenComma {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or '}' after object value, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken() // next key

		if tok.Type == TokenBraceClose {
			return tok, &ParseError{
				Message: "trailing comma in object is not allowed",
				Line:    tok.Line, Column: tok.Column,
			}
		}
	}
}

// directDecodeMap reads a JSON object into a map[string]V.
func directDecodeMap(l *Lexer, dest reflect.Value) (Token, error) {
	tok := l.NextToken()

	if dest.IsNil() {
		dest.Set(reflect.MakeMap(dest.Type()))
	}

	if tok.Type == TokenBraceClose {
		return l.NextToken(), nil
	}

	elemType := dest.Type().Elem()

	for {
		if tok.Type != TokenString {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected string key in object, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		key := unescapeTokenString(l.input, tok)

		tok = l.NextToken() // ':'
		if tok.Type != TokenColon {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ':' after object key, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken() // value

		newElem := reflect.New(elemType).Elem()
		var err error
		tok, err = directDecode(l, tok, newElem)
		if err != nil {
			return tok, err
		}
		dest.SetMapIndex(reflect.ValueOf(key), newElem)

		if tok.Type == TokenBraceClose {
			return l.NextToken(), nil
		}
		if tok.Type != TokenComma {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or '}' after object value, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()
	}
}

// directDecodeArray reads a JSON array and writes elements directly into a slice.
func directDecodeArray(l *Lexer, dest reflect.Value) (Token, error) {
	tok := l.NextToken()

	if tok.Type == TokenBracketClose {
		dest.Set(reflect.MakeSlice(dest.Type(), 0, 0))
		return l.NextToken(), nil
	}

	elemType := dest.Type().Elem()
	slice := reflect.MakeSlice(dest.Type(), 0, 4) // pre-allocate small capacity

	for {
		newElem := reflect.New(elemType).Elem()
		var err error
		tok, err = directDecode(l, tok, newElem)
		if err != nil {
			return tok, err
		}
		slice = reflect.Append(slice, newElem)

		if tok.Type == TokenBracketClose {
			dest.Set(slice)
			return l.NextToken(), nil
		}

		if tok.Type == TokenError {
			return tok, &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}
		}

		if tok.Type != TokenComma {
			return tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or ']' after array element, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}

		tok = l.NextToken()

		if tok.Type == TokenBracketClose {
			return tok, &ParseError{
				Message: "trailing comma in array is not allowed",
				Line:    tok.Line, Column: tok.Column,
			}
		}
	}
}

// --- Skip functions: advance past a JSON value without allocating ---

func skipValue(l *Lexer, tok Token) (Token, error) {
	switch tok.Type {
	case TokenString, TokenNumber, TokenTrue, TokenFalse, TokenNull:
		return l.NextToken(), nil
	case TokenBraceOpen:
		return skipObject(l)
	case TokenBracketOpen:
		return skipArray(l)
	case TokenError:
		return tok, &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}
	default:
		return tok, &ParseError{
			Message: fmt.Sprintf("unexpected token %s", tok.Type),
			Line:    tok.Line, Column: tok.Column,
		}
	}
}

func skipObject(l *Lexer) (Token, error) {
	tok := l.NextToken()
	if tok.Type == TokenBraceClose {
		return l.NextToken(), nil
	}
	for {
		// skip key
		if tok.Type != TokenString {
			return tok, &ParseError{Message: "expected string key", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken() // ':'
		if tok.Type != TokenColon {
			return tok, &ParseError{Message: "expected ':'", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken() // value
		var err error
		tok, err = skipValue(l, tok)
		if err != nil {
			return tok, err
		}
		if tok.Type == TokenBraceClose {
			return l.NextToken(), nil
		}
		if tok.Type != TokenComma {
			return tok, &ParseError{Message: "expected ',' or '}'", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken()
	}
}

func skipArray(l *Lexer) (Token, error) {
	tok := l.NextToken()
	if tok.Type == TokenBracketClose {
		return l.NextToken(), nil
	}
	for {
		var err error
		tok, err = skipValue(l, tok)
		if err != nil {
			return tok, err
		}
		if tok.Type == TokenBracketClose {
			return l.NextToken(), nil
		}
		if tok.Type != TokenComma {
			return tok, &ParseError{Message: "expected ',' or ']'", Line: tok.Line, Column: tok.Column}
		}
		tok = l.NextToken()
	}
}

// --- Generic decode (for interface{} destinations) ---

func decodeToAny(l *Lexer, tok Token) (any, Token, error) {
	switch tok.Type {
	case TokenNull:
		return nil, l.NextToken(), nil
	case TokenTrue:
		return true, l.NextToken(), nil
	case TokenFalse:
		return false, l.NextToken(), nil
	case TokenString:
		s := unescapeTokenString(l.input, tok)
		return s, l.NextToken(), nil
	case TokenNumber:
		raw := l.input[tok.Start:tok.End]
		f, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return nil, tok, &ParseError{Message: fmt.Sprintf("invalid number: %v", err), Line: tok.Line, Column: tok.Column}
		}
		return f, l.NextToken(), nil
	case TokenBraceOpen:
		return decodeObjectToAny(l)
	case TokenBracketOpen:
		return decodeArrayToAny(l)
	case TokenError:
		return nil, tok, &ParseError{Message: l.err, Line: tok.Line, Column: tok.Column}
	default:
		return nil, tok, &ParseError{
			Message: fmt.Sprintf("unexpected token %s", tok.Type),
			Line:    tok.Line, Column: tok.Column,
		}
	}
}

func decodeObjectToAny(l *Lexer) (any, Token, error) {
	tok := l.NextToken()
	if tok.Type == TokenBraceClose {
		return map[string]any{}, l.NextToken(), nil
	}
	m := make(map[string]any)
	for {
		if tok.Type != TokenString {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected string key, got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		key := unescapeTokenString(l.input, tok)

		tok = l.NextToken()
		if tok.Type != TokenColon {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected ':', got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()

		var val any
		var err error
		val, tok, err = decodeToAny(l, tok)
		if err != nil {
			return nil, tok, err
		}
		m[key] = val

		if tok.Type == TokenBraceClose {
			return m, l.NextToken(), nil
		}
		if tok.Type != TokenComma {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or '}', got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()
	}
}

func decodeArrayToAny(l *Lexer) (any, Token, error) {
	tok := l.NextToken()
	if tok.Type == TokenBracketClose {
		return []any{}, l.NextToken(), nil
	}
	arr := make([]any, 0, 4)
	for {
		var val any
		var err error
		val, tok, err = decodeToAny(l, tok)
		if err != nil {
			return nil, tok, err
		}
		arr = append(arr, val)

		if tok.Type == TokenBracketClose {
			return arr, l.NextToken(), nil
		}
		if tok.Type != TokenComma {
			return nil, tok, &ParseError{
				Message: fmt.Sprintf("expected ',' or ']', got %s", tok.Type),
				Line:    tok.Line, Column: tok.Column,
			}
		}
		tok = l.NextToken()
	}
}

// --- String helpers ---

// unescapeTokenString converts raw token bytes into a Go string,
// processing JSON escape sequences. Optimized for the common case
// where no escape sequences are present (single allocation, no processing).
func unescapeTokenString(input []byte, tok Token) string {
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
