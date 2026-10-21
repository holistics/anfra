package cloudapi_test

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/cloudapi"
)

// The types here hold to the server's contract, the vendored openapi.yaml: what
// the client sends, the server takes (every member is one it knows, of its type,
// and every member it requires is sent), and what the client reads, the server
// answers (every member read is one it has, of its type). The client may read
// less than the server answers. A failure here is a change one side made that
// the CLI would not survive: fix the type, or, if anfra-cloud broke the
// contract, raise it there.
func TestConformsToTheServersContract(t *testing.T) {
	spec := loadSpec(t)
	type empty struct{}
	for _, c := range []struct {
		op      string
		in, out any
		codes   []string // the error codes the client tells apart
	}{
		{cloudapi.OpStatus, empty{}, cloudapi.StatusOut{}, nil},
		{cloudapi.OpListMissingBlobs, cloudapi.ListMissingBlobsIn{}, cloudapi.ListMissingBlobsOut{}, nil},
		{cloudapi.OpCreateBlobs, cloudapi.CreateBlobsIn{}, cloudapi.CreateBlobsOut{}, nil},
		{cloudapi.OpPublish, cloudapi.PublishIn{}, cloudapi.PublishOut{}, []string{cloudapi.CodeBlobsMissing, cloudapi.CodeRepoInvalid}},
		{cloudapi.OpListDataSources, empty{}, cloudapi.ListDataSourcesOut{}, []string{cloudapi.CodeForbidden}},
		{cloudapi.OpCreateDataSource, cloudapi.CreateDataSourceIn{}, cloudapi.DataSource{}, []string{cloudapi.CodeForbidden}},
		{cloudapi.OpTestDataSource, cloudapi.TestDataSourceIn{}, cloudapi.TestDataSourceOut{}, []string{cloudapi.CodeForbidden}},
		{cloudapi.OpListAppOrigins, empty{}, cloudapi.AppOriginsOut{}, nil},
	} {
		t.Run(c.op, func(t *testing.T) {
			op, ok := spec.ops[c.op]
			if !ok {
				t.Fatalf("the server has no %s", c.op)
			}
			// Every operation this bearer-only CLI calls must admit OAuth on the
			// server contract, including an authentication challenge on failure.
			security, _ := op["security"].([]any)
			bearer := false
			for _, alternative := range security {
				requirements, _ := alternative.(map[string]any)
				if _, ok := requirements["oauthBearer"]; ok && len(requirements) == 1 {
					bearer = true
				}
			}
			if !bearer {
				t.Error("operation does not admit standalone OAuth bearer credentials")
			}
			if dig(op, "responses", "401", "headers", "WWW-Authenticate") == nil {
				t.Error("operation lacks bearer authentication challenge")
			}
			req := dig(op, "requestBody", "content", "application/json", "schema")
			if req == nil {
				t.Fatalf("%s takes no JSON body", c.op)
			}
			for _, p := range spec.sent(req, reflect.TypeOf(c.in), "in") {
				t.Error(p)
			}
			res := dig(op, "responses", "200", "content", "application/json", "schema")
			if res == nil {
				t.Fatalf("%s answers no JSON body", c.op)
			}
			for _, p := range spec.read(res, reflect.TypeOf(c.out), "out") {
				t.Error(p)
			}
			errs := spec.errorBodies(op)
			for _, code := range c.codes {
				if _, ok := errs[code]; !ok {
					t.Errorf("%s never answers %s", c.op, code)
				}
			}
			if body, ok := errs[cloudapi.CodeBlobsMissing]; ok {
				for _, p := range spec.read(dig(body, "properties", "details"), reflect.TypeOf(cloudapi.MissingBlobs{}), "blobs_missing.details") {
					t.Error(p)
				}
			}
		})
	}
}

type contract struct {
	doc map[string]any
	ops map[string]map[string]any // by operationId
}

func loadSpec(t *testing.T) contract {
	t.Helper()
	raw, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	c := contract{doc: doc, ops: map[string]map[string]any{}}
	paths, _ := doc["paths"].(map[string]any)
	for _, item := range paths {
		methods, _ := item.(map[string]any)
		for _, o := range methods {
			op, _ := o.(map[string]any)
			if id, ok := op["operationId"].(string); ok {
				c.ops[id] = op
			}
		}
	}
	return c
}

// resolve follows a schema's $ref, if it is one.
func (c contract) resolve(s map[string]any) map[string]any {
	for s != nil {
		ref, ok := s["$ref"].(string)
		if !ok {
			return s
		}
		name, ok := strings.CutPrefix(ref, "#/components/schemas/")
		if !ok {
			return nil
		}
		s = dig(c.doc, "components", "schemas", name)
	}
	return nil
}

// sent are the problems with sending a value of type t where s is taken.
func (c contract) sent(s map[string]any, t reflect.Type, at string) []string {
	return c.match(s, t, at, true)
}

// read are the problems with reading s's values into type t.
func (c contract) read(s map[string]any, t reflect.Type, at string) []string {
	return c.match(s, t, at, false)
}

var timeType = reflect.TypeFor[time.Time]()

func (c contract) match(s map[string]any, t reflect.Type, at string, sending bool) []string {
	s = c.resolve(s)
	if s == nil {
		return []string{at + ": no schema"}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	want := func(typ string) []string {
		if !hasType(s, typ) {
			return []string{fmt.Sprintf("%s: the server's is %v, this is %s", at, s["type"], typ)}
		}
		return nil
	}
	switch {
	case t == timeType:
		return want("string")
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8:
		if p := want("string"); p != nil {
			return p
		}
		if s["contentEncoding"] != "base64" {
			return []string{at + ": bytes, which the server does not take as base64"}
		}
		return nil
	case t.Kind() == reflect.String:
		return want("string")
	case t.Kind() == reflect.Bool:
		return want("boolean")
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Uint64:
		return want("integer")
	case t.Kind() == reflect.Slice:
		if p := want("array"); p != nil {
			return p
		}
		items, _ := s["items"].(map[string]any)
		return c.match(items, t.Elem(), at+"[]", sending)
	case t.Kind() == reflect.Map:
		if p := want("object"); p != nil {
			return p
		}
		if s["additionalProperties"] == false {
			return []string{at + ": a map, where the server takes fixed members"}
		}
		return nil
	case t.Kind() == reflect.Struct:
		if p := want("object"); p != nil {
			return p
		}
		return c.members(s, t, at, sending)
	}
	return []string{fmt.Sprintf("%s: %s is not a type this test knows", at, t)}
}

func (c contract) members(s map[string]any, t reflect.Type, at string, sending bool) []string {
	props, _ := s["properties"].(map[string]any)
	var problems []string
	sent := map[string]bool{}
	for f := range t.Fields() {
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		sent[name] = !slices.Contains(strings.Split(opts, ","), "omitempty")
		p, ok := props[name].(map[string]any)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s.%s: the server has no such member", at, name))
			continue
		}
		problems = append(problems, c.match(p, f.Type, at+"."+name, sending)...)
	}
	if sending {
		required, _ := s["required"].([]any)
		for _, r := range required {
			name, _ := r.(string)
			always, ok := sent[name]
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s.%s: the server requires it, and this does not send it", at, name))
			case !always:
				problems = append(problems, fmt.Sprintf("%s.%s: the server requires it, and this omits it when empty", at, name))
			}
		}
	}
	return problems
}

// errorBodies are the error bodies op answers with, by code.
func (c contract) errorBodies(op map[string]any) map[string]map[string]any {
	bodies := map[string]map[string]any{}
	var walk func(s map[string]any)
	walk = func(s map[string]any) {
		s = c.resolve(s)
		if s == nil {
			return
		}
		if code, ok := dig(s, "properties", "code")["const"].(string); ok {
			bodies[code] = s
			return
		}
		if e := dig(s, "properties", "error"); e != nil {
			walk(e)
		}
		for _, key := range []string{"oneOf", "anyOf"} {
			alts, _ := s[key].([]any)
			for _, a := range alts {
				m, _ := a.(map[string]any)
				walk(m)
			}
		}
	}
	responses, _ := op["responses"].(map[string]any)
	for status, r := range responses {
		if status == "200" {
			continue
		}
		m, _ := r.(map[string]any)
		walk(dig(m, "content", "application/json", "schema"))
	}
	return bodies
}

func hasType(s map[string]any, typ string) bool {
	switch v := s["type"].(type) {
	case string:
		return v == typ
	case []any:
		return slices.Contains(v, any(typ))
	}
	return false
}

func dig(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		next, ok := m[k].(map[string]any)
		if !ok {
			return nil
		}
		m = next
	}
	return m
}
