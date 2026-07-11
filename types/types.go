package types

import (
	"errors"
	"strconv"
)

// Marshaler is the interface implemented by types that can marshal themselves into valid JSON.
type Marshaler interface {
	MarshalJSON() ([]byte, error)
}

// Unmarshaler is the interface implemented by types that can unmarshal a JSON description of themselves.
type Unmarshaler interface {
	UnmarshalJSON([]byte) error
}

// RawMessage is a raw encoded JSON value. It can be used to delay JSON decoding
// or to precompute a JSON encoding.
type RawMessage []byte

// MarshalJSON returns m as the JSON encoding of m.
func (m RawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalJSON sets *m to a copy of data.
func (m *RawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[:0], data...)
	return nil
}

// Number represents a JSON number literal. It allows callers to decode a JSON number
// without losing precision by avoiding float64 conversion.
type Number string

// String returns the literal text of the number.
func (n Number) String() string {
	return string(n)
}

// Float64 returns the number as a float64.
func (n Number) Float64() (float64, error) {
	return strconv.ParseFloat(string(n), 64)
}

// Int64 returns the number as an int64.
func (n Number) Int64() (int64, error) {
	return strconv.ParseInt(string(n), 10, 64)
}

// MarshalJSON returns the JSON encoding of the number.
func (n Number) MarshalJSON() ([]byte, error) {
	if n == "" {
		return []byte("0"), nil
	}
	return []byte(n), nil
}
