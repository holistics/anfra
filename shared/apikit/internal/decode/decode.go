// Package decode is how apikit reads an op's input and checks its output,
// against the schemas huma generates from the Go types. It is internal to apikit:
// hosts and their ops never see huma's validator or the message rules.
//
// It owns huma's process-wide configuration, set in init. apikit imports this
// package, so the settings are in place before any schema is generated.
package decode

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

func init() {
	// Off by default in huma: `{"Email": …}` would pass validation, and Go's
	// decoder is case-insensitive too, so a miscased field would be silently
	// accepted. The contract is snake_case, exactly.
	huma.ValidateStrictCasing = true
	// On by default in huma: a required list would accept null and document it as
	// a possible output. null appears only where a field says nullable:"true".
	huma.DefaultArrayNullable = false
}

// SchemaOf is t's schema: a reference into reg for a named type, which reg
// registers under the type's name. The empty struct — an op with no input or no
// output — has no name to register under, so its schema is an inline empty object
// rather than a nameless entry: a reference to "#/components/schemas/" with no
// name would resolve to the whole map. Any other unnamed struct is refused at
// registration (apikit.Register), for the same reason.
func SchemaOf(reg huma.Registry, t reflect.Type) *huma.Schema {
	if IsEmpty(t) {
		return &huma.Schema{Type: "object", AdditionalProperties: false}
	}
	return reg.Schema(t, true, "")
}

// IsEmpty reports the empty struct, struct{}.
func IsEmpty(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.Name() == "" && t.NumField() == 0
}

// Input validates raw against In's schema and decodes it. Structural problems are
// invalid_request, with one violation per location; a field path is the caller's
// (`invitations[1].role`), with no transport prefix. An empty body is an empty
// object, so an op whose fields are all optional can be called with none.
func Input[In any](reg huma.Registry, raw []byte) (In, error) {
	return InputTo[In](reg, SchemaOf(reg, reflect.TypeFor[In]()), raw)
}

// InputTo is Input against a schema given rather than reflected from In: an
// engine command's, built from what the engine describes at run time, with In a
// map of its args.
func InputTo[In any](reg huma.Registry, schema *huma.Schema, raw []byte) (In, error) {
	var in In
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	var parsed any
	if err := jsonkit.Unmarshal(raw, &parsed); err != nil {
		return in, apperr.Encapsulate(err, apperr.InvalidRequest, "The body is not valid JSON.")
	}
	res := &huma.ValidateResult{}
	huma.Validate(reg, schema, huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, parsed, res)
	if len(res.Errors) > 0 {
		return in, apperr.NewWith(apperr.InvalidRequest, "", apperr.Violate(violations(res.Errors, exactlyOneGroups(reg, schema), parsed)...))
	}
	if err := jsonkit.Unmarshal(raw, &in); err != nil {
		// Unreachable once the schema accepted it. If reached, the schema and the
		// type disagree, which is ours to fix.
		return in, fmt.Errorf("decode after validation: %w", err)
	}
	return in, nil
}

// CheckOutput reports where out does not match the schema of type t: a nil slice
// encoded as null where the schema says list, an empty required field, an
// out-of-enum value. The op layer calls it in strict mode, so the OpenAPI built
// from the types cannot quietly lie about what an op returns.
func CheckOutput(reg huma.Registry, t reflect.Type, out any) error {
	b, err := jsonkit.Marshal(out)
	if err != nil {
		return err
	}
	var parsed any
	if err := jsonkit.Unmarshal(b, &parsed); err != nil {
		return err
	}
	res := &huma.ValidateResult{}
	huma.Validate(reg, SchemaOf(reg, t), huma.NewPathBuffer(nil, 0),
		huma.ModeReadFromServer, parsed, res)
	if len(res.Errors) == 0 {
		return nil
	}
	msgs := make([]string, len(res.Errors))
	for i, e := range res.Errors {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("output does not match its schema: %s", strings.Join(msgs, "; "))
}
