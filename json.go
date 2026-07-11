package json_parser

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Parse takes a JSON byte slice and returns the parsed Go data structures (map[string]any, []any, float64, etc.)
func Parse(data []byte) (any, error) {
	lexer := NewLexer(data)
	parser := NewParser(lexer)
	return parser.Parse()
}

// Unmarshal parses the JSON-encoded data and stores the result in the value pointed to by v.
// If v is nil or not a pointer, Unmarshal returns an error.
func Unmarshal(data []byte, v any) error {
	parsedVal, err := Parse(data)
	if err != nil {
		return err
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("unmarshal target must be a non-nil pointer")
	}

	return decodeValue(parsedVal, rv.Elem())
}

func decodeValue(src any, dest reflect.Value) error {
	// If the destination can't be set, we can't write to it.
	if !dest.CanSet() {
		return errors.New("cannot set destination value")
	}

	// Dereference pointers, allocating new objects if they are nil
	for dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		dest = dest.Elem()
	}

	// Handle JSON null
	if src == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}

	// If dest is of type interface{} / any, we can assign the parsed structure directly
	if dest.Kind() == reflect.Interface {
		dest.Set(reflect.ValueOf(src))
		return nil
	}

	srcVal := reflect.ValueOf(src)

	// Direct type assignment
	if srcVal.Type().AssignableTo(dest.Type()) {
		dest.Set(srcVal)
		return nil
	}

	// Handle conversions from parsed float64 values to other numeric types
	if srcNum, ok := src.(float64); ok {
		switch dest.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dest.SetInt(int64(srcNum))
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			dest.SetUint(uint64(srcNum))
			return nil
		case reflect.Float32, reflect.Float64:
			dest.SetFloat(srcNum)
			return nil
		}
	}

	// Handle Array and Slice unmarshaling
	if srcArr, ok := src.([]any); ok {
		if dest.Kind() != reflect.Slice && dest.Kind() != reflect.Array {
			return fmt.Errorf("cannot unmarshal array into destination of type %s", dest.Type())
		}

		length := len(srcArr)
		if dest.Kind() == reflect.Slice {
			slice := reflect.MakeSlice(dest.Type(), length, length)
			for i := 0; i < length; i++ {
				if err := decodeValue(srcArr[i], slice.Index(i)); err != nil {
					return err
				}
			}
			dest.Set(slice)
			return nil
		} else {
			// Array
			if dest.Len() < length {
				length = dest.Len()
			}
			for i := 0; i < length; i++ {
				if err := decodeValue(srcArr[i], dest.Index(i)); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Handle Map and Struct unmarshaling
	if srcMap, ok := src.(map[string]any); ok {
		if dest.Kind() == reflect.Struct {
			return decodeStruct(srcMap, dest)
		}

		if dest.Kind() == reflect.Map {
			if dest.Type().Key().Kind() != reflect.String {
				return fmt.Errorf("cannot unmarshal map keys of type %s (must be string)", dest.Type().Key())
			}
			if dest.IsNil() {
				dest.Set(reflect.MakeMap(dest.Type()))
			}
			elemType := dest.Type().Elem()
			for k, v := range srcMap {
				newElem := reflect.New(elemType).Elem()
				if err := decodeValue(v, newElem); err != nil {
					return err
				}
				dest.SetMapIndex(reflect.ValueOf(k), newElem)
			}
			return nil
		}
	}

	return fmt.Errorf("cannot unmarshal %T value into destination of type %s", src, dest.Type())
}

func decodeStruct(src map[string]any, dest reflect.Value) error {
	destType := dest.Type()
	for i := 0; i < dest.NumField(); i++ {
		field := dest.Field(i)
		fieldType := destType.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Read the JSON struct tag
		jsonKey := fieldType.Name
		if tag := fieldType.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				jsonKey = parts[0]
			}
		}

		// Attempt to lookup key case-insensitively if direct lookup fails (optional but friendly)
		// For standard JSON, we try direct match first:
		val, exists := src[jsonKey]
		if !exists {
			// Fallback: case-insensitive check
			for k, v := range src {
				if strings.EqualFold(k, jsonKey) {
					val = v
					exists = true
					break
				}
			}
		}

		if exists {
			if err := decodeValue(val, field); err != nil {
				return fmt.Errorf("field %s: %w", fieldType.Name, err)
			}
		}
	}
	return nil
}
