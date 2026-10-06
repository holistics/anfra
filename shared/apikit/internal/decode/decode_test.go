package decode

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
)

type withList struct {
	Items []string `json:"items"`
}

type withNullable struct {
	Note *string `json:"note" nullable:"true"`
}

func newRegistry() huma.Registry {
	return huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
}

// Finding C: a required list is not nullable, on input or in the schema.
func TestRequiredListRefusesNull(t *testing.T) {
	_, err := Input[withList](newRegistry(), []byte(`{"items":null}`))
	e, ok := errors.AsType[*apperr.Error](err)
	if !ok || e.Code.Public() != apperr.InvalidRequest.Code() {
		t.Fatalf("got %v, want invalid_request", err)
	}
	if v := e.Details.(apperr.Violations).Violations; len(v) != 1 || v[0].Field != "items" {
		t.Errorf("violations = %+v, want one at items", v)
	}
}

func TestSchemasAllowNullOnlyWhereDeclared(t *testing.T) {
	reg := newRegistry()
	list := reg.SchemaFromRef(reg.Schema(reflect.TypeFor[withList](), true, "").Ref)
	if list.Properties["items"].Nullable {
		t.Error("a required list is nullable")
	}
	note := reg.SchemaFromRef(reg.Schema(reflect.TypeFor[withNullable](), true, "").Ref)
	if !note.Properties["note"].Nullable {
		t.Error(`a field marked nullable:"true" is not nullable`)
	}
}

// CheckOutput catches a nil slice, which Go encodes as null whatever the schema says.
func TestCheckOutputCatchesANilSlice(t *testing.T) {
	reg := newRegistry()
	if err := CheckOutput(reg, reflect.TypeFor[withList](), withList{Items: []string{}}); err != nil {
		t.Errorf("an empty list was refused: %v", err)
	}
	err := CheckOutput(reg, reflect.TypeFor[withList](), withList{})
	if err == nil || !strings.Contains(err.Error(), "items") {
		t.Errorf("a nil slice was accepted, or the error does not name the field: %v", err)
	}
}

func TestEmptyBodyIsAnEmptyObject(t *testing.T) {
	type allOptional struct {
		Limit int `json:"limit,omitempty"`
	}
	if _, err := Input[allOptional](newRegistry(), nil); err != nil {
		t.Errorf("an empty body was refused: %v", err)
	}
}

func TestMalformedJSONIsInvalidRequest(t *testing.T) {
	_, err := Input[withList](newRegistry(), []byte(`{"items":`))
	if e, ok := errors.AsType[*apperr.Error](err); !ok || e.Code.Public() != apperr.InvalidRequest.Code() {
		t.Fatalf("got %v, want invalid_request", err)
	}
}
