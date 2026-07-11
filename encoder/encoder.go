package encoder

import (
	"fmt"
	"json_parser/decoder"
	"json_parser/types"
	"math"
	"reflect"
	"strconv"
	"unicode/utf8"
)

// Marshal returns the JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	var e encoder
	e.buf = make([]byte, 0, 256)
	if err := e.encodeValue(reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	return e.buf, nil
}

// encoder writes JSON into a byte buffer.
type encoder struct {
	buf []byte
}

func (e *encoder) writeByte(b byte) {
	e.buf = append(e.buf, b)
}

func (e *encoder) writeString(s string) {
	e.buf = append(e.buf, s...)
}

// encodeValue writes the JSON representation of a reflect.Value.
func (e *encoder) encodeValue(v reflect.Value) error {
	// Handle interfaces and pointers
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			e.writeString("null")
			return nil
		}
		v = v.Elem()
	}

	// Check for Marshaler interface
	if v.CanInterface() {
		if m, ok := v.Interface().(types.Marshaler); ok {
			data, err := m.MarshalJSON()
			if err != nil {
				return err
			}
			e.buf = append(e.buf, data...)
			return nil
		}
	}

	// Check addressable value for pointer-receiver Marshaler
	if v.CanAddr() {
		if m, ok := v.Addr().Interface().(types.Marshaler); ok {
			data, err := m.MarshalJSON()
			if err != nil {
				return err
			}
			e.buf = append(e.buf, data...)
			return nil
		}
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			e.writeString("true")
		} else {
			e.writeString("false")
		}
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.writeString(strconv.FormatInt(v.Int(), 10))
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		e.writeString(strconv.FormatUint(v.Uint(), 10))
		return nil

	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return fmt.Errorf("json: unsupported value: %v", f)
		}
		bits := 64
		if v.Kind() == reflect.Float32 {
			bits = 32
		}
		e.writeString(strconv.FormatFloat(f, 'f', -1, bits))
		return nil

	case reflect.String:
		e.encodeString(v.String())
		return nil

	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return e.encodeByteSlice(v)
		}
		if v.IsNil() {
			e.writeString("null")
			return nil
		}
		return e.encodeArray(v)

	case reflect.Array:
		return e.encodeArray(v)

	case reflect.Map:
		if v.IsNil() {
			e.writeString("null")
			return nil
		}
		return e.encodeMap(v)

	case reflect.Struct:
		return e.encodeStruct(v)

	default:
		return fmt.Errorf("json: unsupported type: %s", v.Type())
	}
}

// encodeString writes a JSON-escaped string including surrounding quotes.
func (e *encoder) encodeString(s string) {
	e.writeByte('"')
	for i := 0; i < len(s); {
		b := s[i]
		switch {
		case b == '"':
			e.writeString(`\"`)
			i++
		case b == '\\':
			e.writeString(`\\`)
			i++
		case b < 0x20:
			switch b {
			case '\n':
				e.writeString(`\n`)
			case '\r':
				e.writeString(`\r`)
			case '\t':
				e.writeString(`\t`)
			case '\b':
				e.writeString(`\b`)
			case '\f':
				e.writeString(`\f`)
			default:
				e.writeString(`\u00`)
				e.writeByte(hexDigits[b>>4])
				e.writeByte(hexDigits[b&0xf])
			}
			i++
		case b < utf8.RuneSelf:
			e.writeByte(b)
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				e.writeString(`\ufffd`)
			} else {
				e.writeString(s[i : i+size])
			}
			i += size
		}
	}
	e.writeByte('"')
}

var hexDigits = [16]byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'}

// encodeArray writes a JSON array from a slice or array value.
func (e *encoder) encodeArray(v reflect.Value) error {
	e.writeByte('[')
	for i := 0; i < v.Len(); i++ {
		if i > 0 {
			e.writeByte(',')
		}
		if err := e.encodeValue(v.Index(i)); err != nil {
			return err
		}
	}
	e.writeByte(']')
	return nil
}

// encodeMap writes a JSON object from a map value. Keys must be strings.
func (e *encoder) encodeMap(v reflect.Value) error {
	if v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("json: unsupported map key type: %s", v.Type().Key())
	}

	e.writeByte('{')
	iter := v.MapRange()
	first := true
	for iter.Next() {
		if !first {
			e.writeByte(',')
		}
		first = false
		e.encodeString(iter.Key().String())
		e.writeByte(':')
		if err := e.encodeValue(iter.Value()); err != nil {
			return err
		}
	}
	e.writeByte('}')
	return nil
}

// encodeStruct writes a JSON object from a struct value, respecting json tags.
func (e *encoder) encodeStruct(v reflect.Value) error {
	meta := decoder.GetStructMeta(v.Type())

	e.writeByte('{')
	first := true
	for _, fm := range meta.Fields {
		field := decoder.FieldByIndexRead(v, fm.Index)
		if !field.IsValid() {
			continue
		}

		// Handle omitempty: skip zero-value fields
		if fm.OmitEmpty && isZeroValue(field) {
			continue
		}

		if !first {
			e.writeByte(',')
		}
		first = false

		e.encodeString(fm.Name)
		e.writeByte(':')

		// Handle the "string" tag option: wrap the value in a JSON string
		if fm.AsString {
			if err := e.encodeAsString(field); err != nil {
				return err
			}
		} else {
			if err := e.encodeValue(field); err != nil {
				return err
			}
		}
	}
	e.writeByte('}')
	return nil
}

// encodeAsString encodes a value as a JSON string (for the "string" tag option).
func (e *encoder) encodeAsString(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			e.encodeString("true")
		} else {
			e.encodeString("false")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.encodeString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		e.encodeString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		e.encodeString(strconv.FormatFloat(v.Float(), 'f', -1, 64))
	case reflect.String:
		e.encodeString(v.String())
	default:
		return e.encodeValue(v)
	}
	return nil
}

// encodeByteSlice encodes a []byte as a base64-encoded JSON string,
// matching encoding/json behavior.
func (e *encoder) encodeByteSlice(v reflect.Value) error {
	if v.IsNil() {
		e.writeString("null")
		return nil
	}
	b := v.Bytes()
	e.writeByte('"')
	e.writeString(base64Encode(b))
	e.writeByte('"')
	return nil
}

// base64Encode encodes bytes using standard base64 encoding.
func base64Encode(src []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	if len(src) == 0 {
		return ""
	}

	dst := make([]byte, ((len(src)+2)/3)*4)
	di := 0
	for si := 0; si < len(src); si += 3 {
		var val uint32
		remaining := len(src) - si
		switch {
		case remaining >= 3:
			val = uint32(src[si])<<16 | uint32(src[si+1])<<8 | uint32(src[si+2])
			dst[di] = table[val>>18&0x3f]
			dst[di+1] = table[val>>12&0x3f]
			dst[di+2] = table[val>>6&0x3f]
			dst[di+3] = table[val&0x3f]
		case remaining == 2:
			val = uint32(src[si])<<16 | uint32(src[si+1])<<8
			dst[di] = table[val>>18&0x3f]
			dst[di+1] = table[val>>12&0x3f]
			dst[di+2] = table[val>>6&0x3f]
			dst[di+3] = '='
		case remaining == 1:
			val = uint32(src[si]) << 16
			dst[di] = table[val>>18&0x3f]
			dst[di+1] = table[val>>12&0x3f]
			dst[di+2] = '='
			dst[di+3] = '='
		}
		di += 4
	}
	return string(dst)
}

// isZeroValue checks if a reflect.Value is the zero value for its type.
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.String() == ""
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.Array:
		return v.Len() == 0
	case reflect.Struct:
		return false
	default:
		return false
	}
}
