package json_parser

import (
	encoding_json "encoding/json"
	"testing"
)

func BenchmarkParser_Unmarshal(b *testing.B) {
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
