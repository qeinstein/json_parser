package decoder

import (
	"reflect"
	"strings"
	"sync"
)

// FieldMeta holds metadata about a single struct field for JSON encoding/decoding.
type FieldMeta struct {
	Index     []int  // field index path (supports embedded structs)
	Name      string // JSON key name
	OmitEmpty bool   // true if the field has the "omitempty" tag option
	AsString  bool   // true if the field has the "string" tag option
	Type      reflect.Type
}

// StructMeta holds cached metadata about a Go struct's fields for fast JSON key lookup.
type StructMeta struct {
	Fields      []FieldMeta        // all fields in order
	ByKey       map[string]int     // JSON key name -> index into Fields slice
	ByFoldedKey map[string]int     // case-folded JSON key name -> index into Fields slice
}

var (
	fieldCacheMu sync.RWMutex
	fieldCacheMap = make(map[reflect.Type]*StructMeta)
)

// GetStructMeta returns cached struct metadata, building it on first access.
func GetStructMeta(t reflect.Type) *StructMeta {
	fieldCacheMu.RLock()
	meta, ok := fieldCacheMap[t]
	fieldCacheMu.RUnlock()
	if ok {
		return meta
	}

	fieldCacheMu.Lock()
	defer fieldCacheMu.Unlock()

	// Double-check after acquiring write lock
	if meta, ok = fieldCacheMap[t]; ok {
		return meta
	}

	meta = buildStructMeta(t)
	fieldCacheMap[t] = meta
	return meta
}

// buildStructMeta inspects a struct type via reflection and builds the key-to-field mapping.
// It recursively flattens anonymous (embedded) struct fields, matching the behavior of encoding/json.
func buildStructMeta(t reflect.Type) *StructMeta {
	meta := &StructMeta{
		Fields:      make([]FieldMeta, 0, t.NumField()),
		ByKey:       make(map[string]int, t.NumField()),
		ByFoldedKey: make(map[string]int, t.NumField()),
	}

	collectFields(t, nil, meta)
	return meta
}

// collectFields recursively inspects struct fields, flattening embedded structs.
func collectFields(t reflect.Type, parentIndex []int, meta *StructMeta) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		// Build the full index path for this field
		index := make([]int, len(parentIndex)+1)
		copy(index, parentIndex)
		index[len(parentIndex)] = i

		// Handle embedded (anonymous) structs by recursing into them
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			collectFields(f.Type, index, meta)
			continue
		}
		// Also handle pointer-to-struct embeddings
		if f.Anonymous && f.Type.Kind() == reflect.Pointer && f.Type.Elem().Kind() == reflect.Struct {
			collectFields(f.Type.Elem(), index, meta)
			continue
		}

		// Skip unexported non-embedded fields
		if !f.IsExported() {
			continue
		}

		// Parse the json struct tag
		key := f.Name
		omitEmpty := false
		asString := false

		if tag := f.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				key = parts[0]
			}
			for _, opt := range parts[1:] {
				switch opt {
				case "omitempty":
					omitEmpty = true
				case "string":
					asString = true
				}
			}
		}

		// First field with a given key wins (matches encoding/json precedence rules)
		if _, exists := meta.ByKey[key]; exists {
			continue
		}

		idx := len(meta.Fields)
		meta.Fields = append(meta.Fields, FieldMeta{
			Index:     index,
			Name:      key,
			OmitEmpty: omitEmpty,
			AsString:  asString,
			Type:      f.Type,
		})
		meta.ByKey[key] = idx
		folded := strings.ToLower(key)
		if _, exists := meta.ByFoldedKey[folded]; !exists {
			meta.ByFoldedKey[folded] = idx
		}
	}
}

// FieldByIndex traverses a reflect.Value using an index path, allocating nil pointers along the way.
func FieldByIndex(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

// FieldByIndexRead traverses a reflect.Value using an index path for reading (no allocation).
// Returns the zero Value if a nil pointer is encountered.
func FieldByIndexRead(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}
