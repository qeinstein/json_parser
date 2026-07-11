# Strict & High-Performance JSON Parser in Go

A compliant, production-ready JSON parser built from scratch in Go. This project implements a full parsing pipeline: a lexical scanner, a recursive descent parser, and a reflection-based struct decoder.

---

## Features

- **Strict RFC 8259 Compliance**: Enforces valid JSON syntax (e.g., rejects leading zeros in numbers, unclosed containers, and trailing commas).
- **Escape Sequence Decoding**: Full decoding of escaped characters (`\n`, `\t`, `\"`, `\\`, etc.) and Unicode points including UTF-16 surrogate pairs (e.g., emojis like `😀`).
- **Detailed Error Offsets**: Returns precise error positions (line and column numbers) when parsing invalid JSON inputs.
- **Reflection Decoder**: Maps parsed JSON structures recursively into target Go structs (using `json` tags), maps, slices, and primitive types.
- **Zero Third-Party Dependencies**: Written entirely in pure Go.

---

## Directory Layout

- [lexer.go](file:///Users/toheeb.ogunade/Workspace/json_parser/lexer.go): Scans raw JSON strings into typed tokens.
- [parser.go](file:///Users/toheeb.ogunade/Workspace/json_parser/parser.go): Recursive descent parser that verifies grammar and builds Go values.
- [json.go](file:///Users/toheeb.ogunade/Workspace/json_parser/json.go): Contains the public `Unmarshal` and `Parse` APIs with the reflection decoder.
- [json_benchmark_test.go](file:///Users/toheeb.ogunade/Workspace/json_parser/json_benchmark_test.go): Comparative benchmark against Go's standard library `encoding/json`.

---

## Quick Start

### 1. Parsing dynamically
To parse JSON into generic Go types (`map[string]any`, `[]any`, `float64`, etc.):

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

### 2. Unmarshaling into a Struct
To decode JSON directly into a typed Go struct:

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

### Example Benchmark Results (Apple M4 CPU):
- **Our Parser**: `922.9 ns/op` (33 allocations)
- **Go Standard Library (`encoding/json`)**: `681.4 ns/op` (11 allocations)
