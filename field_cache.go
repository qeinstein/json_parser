package json_parser

import (
	"reflect"
	"strings"
	"sync"
)

// structMeta holds cached metadata about a Go struct's fields for fast JSON key lookup.
type structMeta struct {
	// byKey maps JSON key names to struct field indices (reflect.Value.Field index).
	byKey map[string]int
}

var (
	fieldCacheMu sync.RWMutex
	fieldCacheMap = make(map[reflect.Type]*structMeta)
)

// getStructMeta returns cached struct metadata, building it on first access.
func getStructMeta(t reflect.Type) *structMeta {
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
func buildStructMeta(t reflect.Type) *structMeta {
	n := t.NumField()
	meta := &structMeta{
		byKey: make(map[string]int, n),
	}

	for i := 0; i < n; i++ {
		f := t.Field(i)

		// Skip unexported fields
		if !f.IsExported() {
			continue
		}

		key := f.Name
		if tag := f.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				key = parts[0]
			}
		}

		meta.byKey[key] = i
	}

	return meta
}
