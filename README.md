# Strict & High-Performance JSON Parser in Go

A compliant, production-ready JSON parser built from scratch in Go — **1.76x faster than Go's standard library `encoding/json`**. This project implements a full parsing pipeline: a zero-copy lexical scanner, a recursive descent parser, a struct field cache, and a direct reflection decoder.

---

## Performance

Benchmarked on Apple M4 (arm64), Go 1.21+:

| Scenario | Our Parser | `encoding/json` | Speedup |
|---|---|---|---|
| **Small Object** (4 fields) | 391 ns/op · 248 B · 10 allocs | 675 ns/op · 368 B · 11 allocs | **1.73x** |
| **Medium Object** (nested, 9+ fields) | 1,106 ns/op · 544 B · 23 allocs | 2,114 ns/op · 680 B · 22 allocs | **1.91x** |
| **Large Array** (5 nested objects) | 5,197 ns/op · 4,560 B · 90 allocs | 9,287 ns/op · 3,816 B · 71 allocs | **1.79x** |
| **Primitives Only** (floats, ints, bool) | 382 ns/op · 48 B · 1 alloc | 783 ns/op · 264 B · 5 allocs | **2.05x** |
| **String Heavy** (long text fields) | 900 ns/op · 736 B · 7 allocs | 2,661 ns/op · 952 B · 11 allocs | **2.96x** |
| **Dynamic Parse** (`map[string]any`) | 718 ns/op · 984 B · 25 allocs | 847 ns/op · 1,128 B · 27 allocs | **1.18x** |

**Fastest on string-heavy payloads (2.96x)** — the zero-copy tokenizer shines when strings don't need escape processing.

---

## Features

- **Strict RFC 8259 Compliance**: Enforces valid JSON syntax (rejects leading zeros, unclosed containers, trailing commas).
- **Zero-Copy Tokenization**: Tokens store byte offsets into the original input instead of heap-allocated strings, eliminating per-token allocations.
- **Direct Struct Decoding**: `Unmarshal` writes values directly into struct fields — no intermediate `map[string]any` is ever built.
- **Struct Metadata Caching**: JSON-key-to-field mappings are computed once per struct type and cached with `sync.RWMutex`.
- **Direct Integer Parsing**: Integer fields are parsed straight from bytes without going through `float64`.
- **Escape Sequence Decoding**: Full decoding of `\n`, `\t`, `\"`, `\\`, `\uXXXX`, and UTF-16 surrogate pairs (e.g., emojis like `😀`).
- **Detailed Error Reporting**: Returns precise line and column numbers for all syntax errors.
- **Zero Third-Party Dependencies**: Written entirely in pure Go.

---

## Architecture

```
JSON bytes ──┬──[ Lexer ]──> Token stream ──[ Parser ]──> map[string]any   (Parse API)
             │
             └──[ Lexer ]──> Token stream ──[ Fast Decoder ]──> struct     (Unmarshal API)
                                                 │
                                            Field Cache
```

| File | Purpose |
|---|---|
| `lexer.go` | Zero-copy scanner: produces offset-based tokens from raw bytes |
| `parser.go` | Recursive descent parser for the dynamic `Parse()` API |
| `fast_decoder.go` | Direct struct decoder for the optimized `Unmarshal()` API |
| `field_cache.go` | Caches struct field metadata (JSON key → field index) per type |
| `json.go` | Public entry points: `Parse()` and `Unmarshal()` |

---

## Quick Start

### 1. Parsing dynamically

```go
package main

import (
	"fmt"
	"json_parser"
)

func main() {
	input := []byte(`{"project": "json-parser", "version": 1.0}`)

	val, err := json_parser.Parse(input)
	if err != nil {
		fmt.Printf("Parse error: %v\n", err)
		return
	}

	fmt.Printf("Parsed: %#v\n", val)
}
```

### 2. Unmarshaling into a struct (fast path)

```go
package main

import (
	"fmt"
	"json_parser"
)

type Config struct {
	Name    string   `json:"username"`
	Age     int      `json:"age"`
	Hobbies []string `json:"hobbies"`
}

func main() {
	input := []byte(`{
		"username": "bob",
		"age": 30,
		"hobbies": ["reading", "running"]
	}`)

	var cfg Config
	if err := json_parser.Unmarshal(input, &cfg); err != nil {
		fmt.Printf("Unmarshal error: %v\n", err)
		return
	}

	fmt.Printf("User: %+v\n", cfg)
}
```

---

## Testing & Benchmarks

### Run all unit tests:
```bash
go test -v ./...
```

### Run benchmarks against the standard library:
```bash
go test -bench=. -benchmem ./...
```
