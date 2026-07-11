package json_parser

import (
	"reflect"
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
