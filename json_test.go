package json_parser

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type User struct {
	Name    string   `json:"username"`
	Age     int      `json:"age"`
	Admin   bool     `json:"is_admin"`
	Hobbies []string `json:"hobbies"`
}

type Company struct {
	Name    string `json:"company_name"`
	Manager User   `json:"manager"`
}

func TestUnmarshal_Struct(t *testing.T) {
	input := `{
		"username": "alice",
		"age": 28,
		"is_admin": true,
		"hobbies": ["coding", "chess"]
	}`

	var user User
	err := Unmarshal([]byte(input), &user)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := User{
		Name:    "alice",
		Age:     28,
		Admin:   true,
		Hobbies: []string{"coding", "chess"},
	}

	if !reflect.DeepEqual(user, expected) {
		t.Errorf("Expected:\n%+v\nGot:\n%+v", expected, user)
	}
}

func TestUnmarshal_NestedStruct(t *testing.T) {
	input := `{
		"company_name": "Acme Inc.",
		"manager": {
			"username": "bob",
			"age": 45,
			"is_admin": false
		}
	}`

	var company Company
	err := Unmarshal([]byte(input), &company)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if company.Name != "Acme Inc." {
		t.Errorf("Expected Acme Inc., got %q", company.Name)
	}
	if company.Manager.Name != "bob" {
		t.Errorf("Expected manager username bob, got %q", company.Manager.Name)
	}
	if company.Manager.Age != 45 {
		t.Errorf("Expected manager age 45, got %d", company.Manager.Age)
	}
}

func TestUnmarshal_PrimitivesAndMaps(t *testing.T) {
	// Slice of ints
	inputSlice := `[1, 2, 3, 4]`
	var ints []int
	if err := Unmarshal([]byte(inputSlice), &ints); err != nil {
		t.Fatalf("Slice unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(ints, []int{1, 2, 3, 4}) {
		t.Errorf("Slice got: %v", ints)
	}

	// Map of string to int
	inputMap := `{"one": 1, "two": 2}`
	var valMap map[string]int
	if err := Unmarshal([]byte(inputMap), &valMap); err != nil {
		t.Fatalf("Map unmarshal failed: %v", err)
	}
	if valMap["one"] != 1 || valMap["two"] != 2 {
		t.Errorf("Map got: %v", valMap)
	}
}

// CustomMarshaler/Unmarshaler types for testing
type CustomText struct {
	Value string
}

func (c CustomText) MarshalJSON() ([]byte, error) {
	return []byte(`"custom:` + c.Value + `"`), nil
}

func (c *CustomText) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("invalid custom text format")
	}
	val := string(data[1 : len(data)-1])
	if strings.HasPrefix(val, "custom:") {
		c.Value = val[len("custom:"):]
	} else {
		c.Value = val
	}
	return nil
}

func TestCustomMarshalerAndUnmarshaler(t *testing.T) {
	// Test Marshaling
	c := CustomText{Value: "hello"}
	data, err := Marshal(c)
	if err != nil {
		t.Fatalf("Marshal CustomText failed: %v", err)
	}
	if string(data) != `"custom:hello"` {
		t.Errorf("Expected `\"custom:hello\"`, got %q", string(data))
	}

	// Test Unmarshaling
	var c2 CustomText
	if err := Unmarshal([]byte(`"custom:world"`), &c2); err != nil {
		t.Fatalf("Unmarshal CustomText failed: %v", err)
	}
	if c2.Value != "world" {
		t.Errorf("Expected `world`, got %q", c2.Value)
	}
}

// Structs for Embedded struct testing
type InnerEmbedded struct {
	FieldA string `json:"field_a"`
	FieldB int    `json:"field_b"`
}

type OuterStruct struct {
	InnerEmbedded
	FieldC bool `json:"field_c"`
}

func TestEmbeddedStructs(t *testing.T) {
	input := `{
		"field_a": "embedded-value",
		"field_b": 999,
		"field_c": true
	}`

	var out OuterStruct
	if err := Unmarshal([]byte(input), &out); err != nil {
		t.Fatalf("Unmarshal EmbeddedStruct failed: %v", err)
	}

	if out.FieldA != "embedded-value" {
		t.Errorf("Expected out.FieldA to be 'embedded-value', got %q", out.FieldA)
	}
	if out.FieldB != 999 {
		t.Errorf("Expected out.FieldB to be 999, got %d", out.FieldB)
	}
	if !out.FieldC {
		t.Errorf("Expected out.FieldC to be true, got false")
	}

	// Test Marshalling embedded struct
	data, err := Marshal(out)
	if err != nil {
		t.Fatalf("Marshal EmbeddedStruct failed: %v", err)
	}
	// Check that fields from inner embedded struct are flattened in marshaled JSON
	expectedJSON := `{"field_a":"embedded-value","field_b":999,"field_c":true}`
	if string(data) != expectedJSON {
		t.Errorf("Expected JSON %q, got %q", expectedJSON, string(data))
	}
}

func TestRawMessage(t *testing.T) {
	type MessageWithRaw struct {
		ID      int        `json:"id"`
		Payload RawMessage `json:"payload"`
	}

	input := `{"id":123,"payload":{"nested_key":"nested_value"}}`
	var m MessageWithRaw
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal RawMessage failed: %v", err)
	}

	if m.ID != 123 {
		t.Errorf("Expected ID 123, got %d", m.ID)
	}
	expectedPayload := `{"nested_key":"nested_value"}`
	if string(m.Payload) != expectedPayload {
		t.Errorf("Expected Payload %q, got %q", expectedPayload, string(m.Payload))
	}

	// Test Marshalling
	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal RawMessage failed: %v", err)
	}
	if string(data) != input {
		t.Errorf("Expected JSON %q, got %q", input, string(data))
	}
}

func TestNumber(t *testing.T) {
	type StructWithNumber struct {
		Value Number `json:"val"`
	}

	input := `{"val":1.23456789e+10}`
	var s StructWithNumber
	if err := Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("Unmarshal Number failed: %v", err)
	}

	if s.Value.String() != "1.23456789e+10" {
		t.Errorf("Expected String '1.23456789e+10', got %q", s.Value.String())
	}

	f, err := s.Value.Float64()
	if err != nil {
		t.Fatalf("Float64 failed: %v", err)
	}
	if f != 1.23456789e+10 {
		t.Errorf("Expected float 1.23456789e+10, got %f", f)
	}

	// Test Marshalling Number
	data, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal Number failed: %v", err)
	}
	if string(data) != input {
		t.Errorf("Expected JSON %q, got %q", input, string(data))
	}
}

func TestValid(t *testing.T) {
	if !Valid([]byte(`{"a": 1}`)) {
		t.Errorf("Expected `{\"a\": 1}` to be valid JSON")
	}
	if Valid([]byte(`{"a": 1,}`)) {
		t.Errorf("Expected `{\"a\": 1,}` to be invalid JSON")
	}
}

func TestStructTagOptions(t *testing.T) {
	type TaggedStruct struct {
		OmitMe   int    `json:"omit_me,omitempty"`
		KeepMe   int    `json:"keep_me,omitempty"`
		StringVal int   `json:"string_val,string"`
		Ignored  string `json:"-"`
	}

	s := TaggedStruct{
		KeepMe:   456,
		StringVal: 789,
		Ignored:  "hide-me",
	}

	data, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal TaggedStruct failed: %v", err)
	}

	expectedJSON := `{"keep_me":456,"string_val":"789"}`
	if string(data) != expectedJSON {
		t.Errorf("Expected JSON %q, got %q", expectedJSON, string(data))
	}
}

func TestStreamingEncoderDecoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	type Data struct {
		Name string `json:"name"`
		Val  int    `json:"val"`
	}

	d1 := Data{Name: "first", Val: 10}
	d2 := Data{Name: "second", Val: 20}

	if err := enc.Encode(d1); err != nil {
		t.Fatalf("Encode d1 failed: %v", err)
	}
	if err := enc.Encode(d2); err != nil {
		t.Fatalf("Encode d2 failed: %v", err)
	}

	dec := NewDecoder(&buf)
	var out1, out2 Data

	if err := dec.Decode(&out1); err != nil {
		t.Fatalf("Decode out1 failed: %v", err)
	}
	if err := dec.Decode(&out2); err != nil {
		t.Fatalf("Decode out2 failed: %v", err)
	}

	if out1 != d1 || out2 != d2 {
		t.Errorf("Streaming decode got mismatched data: out1=%+v, out2=%+v", out1, out2)
	}
}

func TestMaxDepthLimit(t *testing.T) {
	// 1001 levels of nested arrays
	input := strings.Repeat("[", 1001) + strings.Repeat("]", 1001)
	var v any
	err := Unmarshal([]byte(input), &v)
	if err == nil {
		t.Fatal("Expected error for deeply nested JSON exceeding 1000 limit, but succeeded")
	}
	if !strings.Contains(err.Error(), "exceeded max depth limit") {
		t.Errorf("Expected 'exceeded max depth limit' error message, got: %v", err)
	}

	// Dynamic Parse path check
	_, err = Parse([]byte(input))
	if err == nil {
		t.Fatal("Expected error for Parse with deeply nested JSON exceeding 1000 limit, but succeeded")
	}
	if !strings.Contains(err.Error(), "exceeded max depth limit") {
		t.Errorf("Expected 'exceeded max depth limit' error message, got: %v", err)
	}
}

func TestDisallowUnknownFields(t *testing.T) {
	type Simple struct {
		Name string `json:"name"`
	}

	input := `{"name":"alice","age":30}`
	var s Simple

	// Normal unmarshal ignores unknown fields
	if err := Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if s.Name != "alice" {
		t.Errorf("Expected name alice, got %q", s.Name)
	}

	// Strict unmarshal fails on unknown fields
	err := UnmarshalWithOptions([]byte(input), &s, DecodeOptions{DisallowUnknownFields: true})
	if err == nil {
		t.Fatal("Expected error for unknown field, but succeeded")
	}
	if !strings.Contains(err.Error(), "json: unknown field \"age\"") {
		t.Errorf("Expected 'unknown field' error, got: %v", err)
	}
}

func TestUseNumber(t *testing.T) {
	input := `{"val":123.45}`

	// Normal decode turns number into float64
	var v any
	if err := Unmarshal([]byte(input), &v); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	m := v.(map[string]any)
	if _, ok := m["val"].(float64); !ok {
		t.Errorf("Expected float64, got %T", m["val"])
	}

	// UseNumber turns number into Number
	var v2 any
	dec := NewDecoder(strings.NewReader(input))
	dec.UseNumber()
	if err := dec.Decode(&v2); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	m2 := v2.(map[string]any)
	n, ok := m2["val"].(Number)
	if !ok {
		t.Errorf("Expected types.Number, got %T", m2["val"])
	}
	if n.String() != "123.45" {
		t.Errorf("Expected '123.45', got %q", n.String())
	}
}

func TestCyclicPointer(t *testing.T) {
	type Node struct {
		Name string `json:"name"`
		Next *Node  `json:"next"`
	}

	node1 := Node{Name: "first"}
	node2 := Node{Name: "second"}
	node1.Next = &node2
	node2.Next = &node1 // create cycle

	_, err := Marshal(node1)
	if err == nil {
		t.Fatal("Expected error for cyclic pointer, but succeeded")
	}
	if !strings.Contains(err.Error(), "json: unsupported value: encountered a cycle") {
		t.Errorf("Expected cycle detection error, got: %v", err)
	}
}

// Custom text type implementing encoding.TextMarshaler/TextUnmarshaler
type CustomTextType struct {
	Text string
}

func (c CustomTextType) MarshalText() ([]byte, error) {
	return []byte("text:" + c.Text), nil
}

func (c *CustomTextType) UnmarshalText(text []byte) error {
	s := string(text)
	if strings.HasPrefix(s, "text:") {
		c.Text = s[len("text:"):]
	} else {
		c.Text = s
	}
	return nil
}

func TestTextMarshalerAndUnmarshaler(t *testing.T) {
	type Wrapper struct {
		Val CustomTextType `json:"val"`
	}

	w := Wrapper{Val: CustomTextType{Text: "gopher"}}
	data, err := Marshal(w)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	expectedJSON := `{"val":"text:gopher"}`
	if string(data) != expectedJSON {
		t.Errorf("Expected %q, got %q", expectedJSON, string(data))
	}

	var w2 Wrapper
	if err := Unmarshal(data, &w2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if w2.Val.Text != "gopher" {
		t.Errorf("Expected 'gopher', got %q", w2.Val.Text)
	}
}

func TestHTMLSafeEscaping(t *testing.T) {
	type Msg struct {
		Message string `json:"msg"`
	}
	m := Msg{Message: "<script>&</script>\u2028"}

	// HTML escaping enabled by default
	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	expectedJSON := `{"msg":"\u003cscript\u003e\u0026\u003c/script\u003e\u2028"}`
	if string(data) != expectedJSON {
		t.Errorf("Expected %q, got %q", expectedJSON, string(data))
	}

	// HTML escaping disabled
	data2, err := MarshalWithOptions(m, false)
	if err != nil {
		t.Fatalf("MarshalWithOptions failed: %v", err)
	}
	expectedJSON2 := `{"msg":"<script>&</script>\u2028"}`
	if string(data2) != expectedJSON2 {
		t.Errorf("Expected %q, got %q", expectedJSON2, string(data2))
	}
}

func TestFormattingHelpers(t *testing.T) {
	src := []byte(`{  "a"  :   1  , "b"  :  [  2  ,  3  ]  }`)
	var dst bytes.Buffer

	// Test Compact
	if err := Compact(&dst, src); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	expectedCompact := `{"a":1,"b":[2,3]}`
	if dst.String() != expectedCompact {
		t.Errorf("Expected Compact %q, got %q", expectedCompact, dst.String())
	}

	// Test Indent
	dst.Reset()
	if err := Indent(&dst, []byte(expectedCompact), "", "  "); err != nil {
		t.Fatalf("Indent failed: %v", err)
	}
	expectedIndent := `{
  "a": 1,
  "b": [
    2,
    3
  ]
}`
	if dst.String() != expectedIndent {
		t.Errorf("Expected Indent:\n%s\nGot:\n%s", expectedIndent, dst.String())
	}

	// Test HTMLEscape
	dst.Reset()
	HTMLEscape(&dst, []byte(`{"msg":"<p>&</p>"}`))
	expectedHTML := `{"msg":"\u003cp\u003e\u0026\u003c/p\u003e"}`
	if dst.String() != expectedHTML {
		t.Errorf("Expected HTMLEscape %q, got %q", expectedHTML, dst.String())
	}
}

func TestDecoderInputOffset(t *testing.T) {
	input := `{"a": 1} {"b": 2}`
	dec := NewDecoder(strings.NewReader(input))

	var a map[string]int
	if err := dec.Decode(&a); err != nil {
		t.Fatalf("Decode 1 failed: %v", err)
	}
	// The offset should be right after first JSON object
	if dec.InputOffset() != 8 {
		t.Errorf("Expected offset 8, got %d", dec.InputOffset())
	}

	var b map[string]int
	if err := dec.Decode(&b); err != nil {
		t.Fatalf("Decode 2 failed: %v", err)
	}
}

func TestCaseInsensitiveMatchingFallback(t *testing.T) {
	type StructWithMixedCase struct {
		NameField string `json:"NameField"`
	}

	// JSON key has different casing
	input := `{"namefield":"bob"}`
	var s StructWithMixedCase
	if err := Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if s.NameField != "bob" {
		t.Errorf("Expected NameField to be 'bob', got %q", s.NameField)
	}
}

func TestMarshalIndent(t *testing.T) {
	type Data struct {
		A int `json:"a"`
	}
	d := Data{A: 1}
	b, err := MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	expected := "{\n  \"a\": 1\n}"
	if string(b) != expected {
		t.Errorf("Expected Indent:\n%s\nGot:\n%s", expected, string(b))
	}
}



