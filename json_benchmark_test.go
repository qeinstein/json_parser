package json_parser

import (
	encoding_json "encoding/json"
	"testing"
)

// Benchmark types

type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

type Employee struct {
	ID        int      `json:"id"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Email     string   `json:"email"`
	Age       int      `json:"age"`
	Active    bool     `json:"active"`
	Salary    float64  `json:"salary"`
	Tags      []string `json:"tags"`
	Address   Address  `json:"address"`
}

type APIResponse struct {
	Status  string     `json:"status"`
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    []Employee `json:"data"`
}

// Small object: simple struct, 4 fields

func BenchmarkSmallObject(b *testing.B) {
	input := []byte(`{
		"username": "alice",
		"age": 28,
		"is_admin": true,
		"hobbies": ["coding", "chess"]
	}`)

	b.Run("OurParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = Unmarshal(input, &user)
		}
	})

	b.Run("Stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = encoding_json.Unmarshal(input, &user)
		}
	})
}

// Medium object: nested struct, 9+ fields

func BenchmarkMediumObject(b *testing.B) {
	input := []byte(`{
		"id": 42,
		"first_name": "Jane",
		"last_name": "Doe",
		"email": "jane.doe@example.com",
		"age": 34,
		"active": true,
		"salary": 95000.50,
		"tags": ["engineering", "backend", "senior", "go"],
		"address": {
			"street": "123 Main St",
			"city": "San Francisco",
			"state": "CA",
			"zip_code": "94105",
			"country": "US"
		}
	}`)

	b.Run("OurParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var emp Employee
			_ = Unmarshal(input, &emp)
		}
	})

	b.Run("Stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var emp Employee
			_ = encoding_json.Unmarshal(input, &emp)
		}
	})
}

// Large array of objects

func BenchmarkLargeArray(b *testing.B) {
	input := []byte(`{
		"status": "ok",
		"code": 200,
		"message": "success",
		"data": [
			{"id": 1, "first_name": "Alice", "last_name": "Smith", "email": "alice@example.com", "age": 30, "active": true, "salary": 80000, "tags": ["admin"], "address": {"street": "1 A St", "city": "NYC", "state": "NY", "zip_code": "10001", "country": "US"}},
			{"id": 2, "first_name": "Bob", "last_name": "Jones", "email": "bob@example.com", "age": 25, "active": false, "salary": 70000, "tags": ["user", "beta"], "address": {"street": "2 B St", "city": "LA", "state": "CA", "zip_code": "90001", "country": "US"}},
			{"id": 3, "first_name": "Charlie", "last_name": "Brown", "email": "charlie@example.com", "age": 40, "active": true, "salary": 120000, "tags": ["admin", "senior"], "address": {"street": "3 C St", "city": "Chicago", "state": "IL", "zip_code": "60601", "country": "US"}},
			{"id": 4, "first_name": "Diana", "last_name": "Prince", "email": "diana@example.com", "age": 35, "active": true, "salary": 95000, "tags": ["manager"], "address": {"street": "4 D St", "city": "Seattle", "state": "WA", "zip_code": "98101", "country": "US"}},
			{"id": 5, "first_name": "Eve", "last_name": "Adams", "email": "eve@example.com", "age": 28, "active": true, "salary": 85000, "tags": ["user", "tester"], "address": {"street": "5 E St", "city": "Denver", "state": "CO", "zip_code": "80201", "country": "US"}}
		]
	}`)

	b.Run("OurParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var resp APIResponse
			_ = Unmarshal(input, &resp)
		}
	})

	b.Run("Stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var resp APIResponse
			_ = encoding_json.Unmarshal(input, &resp)
		}
	})
}

// Primitives only: numbers, bools, null

func BenchmarkPrimitivesOnly(b *testing.B) {
	type Metrics struct {
		CPU     float64 `json:"cpu"`
		Memory  float64 `json:"memory"`
		Disk    float64 `json:"disk"`
		Network float64 `json:"network"`
		Uptime  int     `json:"uptime"`
		Healthy bool    `json:"healthy"`
	}

	input := []byte(`{
		"cpu": 73.45,
		"memory": 82.1,
		"disk": 45.67,
		"network": 12.89,
		"uptime": 864000,
		"healthy": true
	}`)

	b.Run("OurParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var m Metrics
			_ = Unmarshal(input, &m)
		}
	})

	b.Run("Stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var m Metrics
			_ = encoding_json.Unmarshal(input, &m)
		}
	})
}

// String heavy: long strings, many string fields

func BenchmarkStringHeavy(b *testing.B) {
	type Article struct {
		Title       string `json:"title"`
		Subtitle    string `json:"subtitle"`
		Author      string `json:"author"`
		Body        string `json:"body"`
		Category    string `json:"category"`
		PublishedAt string `json:"published_at"`
	}

	input := []byte(`{
		"title": "Understanding Zero-Copy Parsing Techniques in Modern Systems Programming",
		"subtitle": "A deep dive into how high-performance parsers avoid unnecessary memory allocations",
		"author": "Dr. Performance McOptimize",
		"body": "In the world of systems programming, every nanosecond counts. When we talk about parsing structured data formats like JSON, XML, or Protocol Buffers, the overhead of memory allocations can quickly dominate the total processing time. Zero-copy parsing is a technique where the parser avoids copying data from the input buffer into separate heap allocations wherever possible.",
		"category": "systems-programming",
		"published_at": "2026-07-11T12:00:00Z"
	}`)

	b.Run("OurParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var a Article
			_ = Unmarshal(input, &a)
		}
	})

	b.Run("Stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var a Article
			_ = encoding_json.Unmarshal(input, &a)
		}
	})
}

// Dynamic parse into map[string]any

func BenchmarkDynamicParse(b *testing.B) {
	input := []byte(`{
		"name": "test",
		"value": 42,
		"active": true,
		"tags": ["a", "b", "c"],
		"nested": {"key": "val"}
	}`)

	b.Run("OurParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = Parse(input)
		}
	})

	b.Run("Stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var v any
			_ = encoding_json.Unmarshal(input, &v)
		}
	})
}
