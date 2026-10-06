package decode

import (
	"errors"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
)

// golden uses every validation keyword our struct tags use. Adding a keyword to an
// op means adding it here.
type golden struct {
	Name  string   `json:"name" minLength:"2" maxLength:"5"`
	Role  string   `json:"role" enum:"admin,analyst"`
	Count int      `json:"count,omitempty" minimum:"1" maximum:"10"`
	Ratio float64  `json:"ratio,omitempty" exclusiveMinimum:"0" exclusiveMaximum:"1"`
	Step  int      `json:"step,omitempty" multipleOf:"5"`
	Tags  []string `json:"tags,omitempty" minItems:"1" maxItems:"2" uniqueItems:"true"`
	Email string   `json:"email,omitempty" format:"email"`
	At    string   `json:"at,omitempty" format:"date-time"`
	ID    string   `json:"id,omitempty" format:"uuid"`
	Slug  string   `json:"slug,omitempty" pattern:"^[a-z]+$"`
	Flag  bool     `json:"flag,omitempty"`
	Items []item   `json:"items,omitempty"`
}

type item struct {
	Label string `json:"label"`
}

// TestViolationsGolden pins every message we produce. A huma upgrade that changes
// what it reports fails here, and is then a deliberate change.
func TestViolationsGolden(t *testing.T) {
	reg := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	for _, tc := range []struct {
		name, in string
		want     apperr.Violations
	}{
		{"required, at the root", `{"name":"abc"}`,
			apperr.Violations{v("role", "required", "Required.")}},
		{"required, nested", `{"name":"abc","role":"admin","items":[{}]}`,
			apperr.Violations{v("items[0].label", "required", "Required.")}},
		{"unknown field", `{"name":"abc","role":"admin","colour":"red"}`,
			apperr.Violations{v("colour", "unknown", "Not a field of this input. Field names are snake_case and case-sensitive.")}},
		{"miscased field is unknown, not silently accepted", `{"Name":"abc","name":"abc","role":"admin"}`,
			apperr.Violations{v("Name", "unknown", "Not a field of this input. Field names are snake_case and case-sensitive.")}},
		{"enum echoes the value", `{"name":"abc","role":"owner"}`,
			apperr.Violations{v("role", "invalid", `Must be one of: admin, analyst; got "owner".`)}},
		{"min length does not echo the value", `{"name":"a","role":"admin"}`,
			apperr.Violations{v("name", "too_short", "Must be at least 2 characters long.")}},
		{"max length", `{"name":"abcdef","role":"admin"}`,
			apperr.Violations{v("name", "too_long", "Must be at most 5 characters long.")}},
		{"minimum", `{"name":"abc","role":"admin","count":0}`,
			apperr.Violations{v("count", "invalid", "Must be at least 1.")}},
		{"maximum", `{"name":"abc","role":"admin","count":11}`,
			apperr.Violations{v("count", "invalid", "Must be at most 10.")}},
		{"exclusive minimum", `{"name":"abc","role":"admin","ratio":0}`,
			apperr.Violations{v("ratio", "invalid", "Must be greater than 0.")}},
		{"exclusive maximum", `{"name":"abc","role":"admin","ratio":1}`,
			apperr.Violations{v("ratio", "invalid", "Must be less than 1.")}},
		{"multiple of", `{"name":"abc","role":"admin","step":7}`,
			apperr.Violations{v("step", "invalid", "Must be a multiple of 5.")}},
		{"min items", `{"name":"abc","role":"admin","tags":[]}`,
			apperr.Violations{v("tags", "too_short", "Must have at least 1 item.")}},
		{"max items", `{"name":"abc","role":"admin","tags":["a","b","c"]}`,
			apperr.Violations{v("tags", "too_long", "Must have at most 2 items.")}},
		{"unique items", `{"name":"abc","role":"admin","tags":["a","a"]}`,
			apperr.Violations{v("tags", "invalid", "Must not contain duplicates.")}},
		{"email", `{"name":"abc","role":"admin","email":"nope"}`,
			apperr.Violations{v("email", "invalid", "Must be an email address.")}},
		{"date-time", `{"name":"abc","role":"admin","at":"yesterday"}`,
			apperr.Violations{v("at", "invalid", "Must be an RFC 3339 date-time, e.g. 2026-10-02T09:30:00Z.")}},
		{"uuid", `{"name":"abc","role":"admin","id":"x"}`,
			apperr.Violations{v("id", "invalid", "Must be a UUID.")}},
		{"pattern", `{"name":"abc","role":"admin","slug":"A1"}`,
			apperr.Violations{v("slug", "invalid", "Must match the pattern ^[a-z]+$.")}},
		{"wrong type names the kind, not the value", `{"name":12,"role":"admin"}`,
			apperr.Violations{v("name", "invalid", "Must be a string; got a number.")}},
		{"integer", `{"name":"abc","role":"admin","count":"3"}`,
			apperr.Violations{v("count", "invalid", "Must be an integer; got a string.")}},
		{"boolean", `{"name":"abc","role":"admin","flag":"yes"}`,
			apperr.Violations{v("flag", "invalid", "Must be true or false; got a string.")}},
		{"list", `{"name":"abc","role":"admin","tags":"a"}`,
			apperr.Violations{v("tags", "invalid", "Must be a list; got a string.")}},
		{"the input itself", `[1,2]`,
			apperr.Violations{v("", "invalid", "The input must be an object; got a list.")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Input[golden](reg, []byte(tc.in))
			e, ok := errors.AsType[*apperr.Error](err)
			if !ok || e.Code.Public() != apperr.InvalidRequest.Code() {
				t.Fatalf("got %v, want invalid_request", err)
			}
			if got, _ := e.Details.(apperr.Violations); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("\n got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func v(field, code, msg string) apperr.Violation {
	return apperr.Violation{Field: field, Code: code, Message: msg}
}

// An unmatched huma message degrades to "invalid" with huma's text; it is never
// dropped.
func TestUnmatchedMessageDegrades(t *testing.T) {
	got := violation(&huma.ErrorDetail{Location: "x", Message: "expected something new"})
	want := apperr.Violation{Field: "x", Code: "invalid", Message: "Expected something new."}
	if got != want {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
