package validator

import (
	"fmt"
	"reflect"
	"testing"
)

type TestStruct struct {
	Name     string   `validation:"required,min:3,max:20"`
	Age      int      `validation:"min:0,max:120"`
	Category string   `validation:"in:foo,bar,baz"`
	Tags     []string `validation:"arrayItemSizeMin:2,arrayItemSizeMax:10"`
}

var passwordRules = PasswordRules{
	min:                  8,
	max:                  32,
	numberOfSpecialChars: 1,
	numberOfUpperChars:   1,
	numberOfLowerChars:   1,
	numberOfDigits:       1,
}

func TestValidateRequired(t *testing.T) {
	t.Run("missing required field", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Name: ""},
		}
		v.Validate()
		if !v.HasErrors() {
			t.Fatal("expected errors for missing required field")
		}
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Name" {
				if _, ok := e.InvalidConditions["required"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected required error on Name field")
		}
	})

	t.Run("required field present", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Name: "Alice"},
		}
		v.Validate()
		for _, e := range v.FieldErrors {
			if e.Field == "Name" {
				if _, ok := e.InvalidConditions["required"]; ok {
					t.Fatal("did not expect required error when field is present")
				}
			}
		}
	})
}

func TestValidateMinMax(t *testing.T) {
	t.Run("string too short", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Name: "Ab"},
		}
		v.Validate()
		if !v.HasErrors() {
			t.Fatal("expected errors")
		}
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Name" {
				if _, ok := e.InvalidConditions["min"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected min error")
		}
	})

	t.Run("string too long", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Name: "thisnameiswaytoolongtobevalid"},
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Name" {
				if _, ok := e.InvalidConditions["max"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected max error")
		}
	})

	t.Run("int out of range", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Age: 200},
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Age" {
				if _, ok := e.InvalidConditions["max"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected max error for Age")
		}
	})
}

func TestValidateIn(t *testing.T) {
	t.Run("value in set", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Category: "bar"},
		}
		v.Validate()
		for _, e := range v.FieldErrors {
			if e.Field == "Category" {
				t.Fatalf("unexpected error for valid category: %v", e.InvalidConditions)
			}
		}
	})

	t.Run("value not in set", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Category: "qux"},
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Category" {
				if _, ok := e.InvalidConditions["in"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected in error for invalid category")
		}
	})
}

func TestValidateArrayItemSize(t *testing.T) {
	t.Run("array item too short", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Tags: []string{"a"}},
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Tags" {
				if _, ok := e.InvalidConditions["arrayItemSizeMin"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected arrayItemSizeMin error")
		}
	})

	t.Run("array item too long", func(t *testing.T) {
		v := &Validator[TestStruct]{
			ToValidate: &TestStruct{Tags: []string{"supercalifragilisticexpialidocious"}},
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Tags" {
				if _, ok := e.InvalidConditions["arrayItemSizeMax"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected arrayItemSizeMax error")
		}
	})
}

func TestCheckSizeMissingParam(t *testing.T) {
	v := &Validator[TestStruct]{}
	field := reflect.TypeOf(TestStruct{}).Field(0) // Name
	v.CheckSize(field, reflect.ValueOf(""), true, "", "min")
	if !v.HasErrors() {
		t.Fatal("expected error for missing param")
	}
	found := false
	for _, e := range v.FieldErrors {
		if e.Field == "Name" {
			if _, ok := e.InvalidConditions["param"]; ok {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected param error")
	}
}

func TestCheckPasswordIsValid(t *testing.T) {
	type PasswordStruct struct {
		Password string `validation:"password"`
	}

	t.Run("valid password", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "Hello1!x"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		if v.HasErrors() {
			t.Fatalf("expected no errors for valid password, got: %v", v.FieldErrors)
		}
	})

	t.Run("missing lowercase", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "HELLO1!X"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Password" {
				if _, ok := e.InvalidConditions["lowerChar"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected lowerChar error for missing lowercase")
		}
	})

	t.Run("missing uppercase", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "hello1!x"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Password" {
				if _, ok := e.InvalidConditions["upperChar"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected upperChar error for missing uppercase")
		}
	})

	t.Run("missing number", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "Hello!!x"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Password" {
				if _, ok := e.InvalidConditions["digit"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected digit error for missing number")
		}
	})

	t.Run("missing special char", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "Hello11x"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Password" {
				if _, ok := e.InvalidConditions["specialChar"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected specialChar error for missing special char")
		}
	})

	t.Run("expecting valid chars", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "Hello1!€"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Password" {
				if _, ok := e.InvalidConditions["invalidChar"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected invalidChar error for disallowed character")
		}
	})

	t.Run("too short", func(t *testing.T) {
		v := &Validator[PasswordStruct]{
			ToValidate:    &PasswordStruct{Password: "Hi1!"},
			PasswordRules: passwordRules,
		}
		v.Validate()
		found := false
		for _, e := range v.FieldErrors {
			if e.Field == "Password" {
				if _, ok := e.InvalidConditions["size"]; ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("expected size error for password under 8 chars")
		}
	})
}

func TestValidateRegex(t *testing.T) {
	t.Run("match regex", func(t *testing.T) {
		type RegexStruct struct {
			Email string `validation:"regex:^[a-z]+@[a-z]+\\.[a-z]+$"`
		}
		v := &Validator[RegexStruct]{
			ToValidate: &RegexStruct{Email: "test@example.com"},
		}
		v.Validate()
		if v.HasErrors() {
			t.Fatalf("expected no errors for valid regex, got: %v", v.FieldErrors)
		}
	})

	t.Run("no match regex", func(t *testing.T) {
		type RegexStruct struct {
			Email string `validation:"regex:^[a-z]+@[a-z]+\\.[a-z]+$"`
		}
		v := &Validator[RegexStruct]{
			ToValidate: &RegexStruct{Email: "invalid-email"},
		}
		v.Validate()
		if !v.HasErrors() {
			t.Fatal("expected regex error for non-matching value")
		}
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		type RegexStruct struct {
			Email string `validation:"regex:[invalid"`
		}
		v := &Validator[RegexStruct]{
			ToValidate: &RegexStruct{Email: "test@example.com"},
		}
		v.Validate()
		if !v.HasErrors() {
			t.Fatal("expected param error for invalid regex pattern")
		}
	})

	t.Run("regex on non-string field", func(t *testing.T) {
		type RegexStruct struct {
			Age int `validation:"regex:^\\d+$"`
		}
		v := &Validator[RegexStruct]{
			ToValidate: &RegexStruct{Age: 25},
		}
		v.Validate()
		if !v.HasErrors() {
			t.Fatal("expected regex error for non-string field")
		}
	})

	t.Run("missing regex param", func(t *testing.T) {
		type RegexStruct struct {
			Email string `validation:"regex:"`
		}
		v := &Validator[RegexStruct]{
			ToValidate: &RegexStruct{Email: "test"},
		}
		v.Validate()
		if !v.HasErrors() {
			t.Fatal("expected param error for empty regex pattern")
		}
	})
}

func TestStringMustContain(t *testing.T) {
	if !StringMustContain("hello", "aeiou", 1) {
		t.Fatal("expected StringMustContain to find a vowel")
	}
	if StringMustContain("xyz", "aeiou", 1) {
		t.Fatal("expected StringMustContain to fail without vowels")
	}
}

func TestStringSizeBetween(t *testing.T) {
	if !StringSizeBetween("hello", 3, 10) {
		t.Fatal("expected StringSizeBetween to pass")
	}
	if StringSizeBetween("hi", 3, 10) {
		t.Fatal("expected StringSizeBetween to fail (too short)")
	}
	if StringSizeBetween("this is way too long", 3, 10) {
		t.Fatal("expected StringSizeBetween to fail (too long)")
	}
}

func TestValidateEmail(t *testing.T) {
	emails := []string{"@s.cc", "s.cc", "s.cc@", "s.cc@sd"}

	for i, email := range emails {
		t.Run(fmt.Sprintf("invalid email %d", i), func(t *testing.T) {
			type EmailStruct struct {
				Email string `validation:"required,email"`
			}
			v := &Validator[EmailStruct]{
				ToValidate: &EmailStruct{Email: email},
			}
			v.Validate()
			if !v.HasErrors() {
				t.Fatalf("expected email %s to fail", email)
			}
		})
	}
}
