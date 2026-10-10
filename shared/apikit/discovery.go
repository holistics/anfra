package apikit

import (
	"context"
	"fmt"
	"slices"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"

	"github.com/holistics/anfra/shared/httpkit"
)

// Discovery renders a registry's ops in three levels, for an agent to drill
// through instead of reading a whole contract: the index (each group, in one
// line), a group (its ops, each in one line), and an op's usage (its doc,
// schemas and codes). Every level shows only what the caller may call, as the
// host's Visible says. HTTP and MCP serve the same levels.
type Discovery[R any] struct {
	// Visible reports which of ops the caller of r may call: the host's
	// admission and fixed permissions, short of the input. Nil: every op.
	Visible func(ctx context.Context, r R, ops []Meta) ([]bool, error)
	// Errors are the codes the host adds to an op (HTTP.Errors), and Codes how
	// it serves them: an op's usage lists both with its own.
	Errors func(m Meta) []apperr.Code
	Codes  httpkit.Codes
}

// Index is the top level: every group with at least one op the caller may call.
type Index struct {
	Groups []GroupEntry `json:"groups"`
}

// GroupEntry is a group in the index.
type GroupEntry struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// GroupListing is a group's ops the caller may call.
type GroupListing struct {
	Name    string    `json:"name"`
	Summary string    `json:"summary"`
	Ops     []OpEntry `json:"ops"`
}

// OpEntry is an op in its group's listing.
type OpEntry struct {
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	ReadOnly    bool   `json:"read_only"`
	Idempotent  bool   `json:"idempotent"`
	Destructive bool   `json:"destructive"`
}

// Usage is all an agent needs to call an op: what it is for, what it takes,
// what it answers, and how it can fail.
type Usage struct {
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Doc         string         `json:"doc,omitempty"`
	ReadOnly    bool           `json:"read_only"`
	Idempotent  bool           `json:"idempotent"`
	Destructive bool           `json:"destructive"`
	Input       map[string]any `json:"input"`
	Output      map[string]any `json:"output"`
	Errors      []ErrorEntry   `json:"errors"`
}

// ErrorEntry is a code an op can fail with.
type ErrorEntry struct {
	Code    string `json:"code"`
	Scope   string `json:"scope"`
	Message string `json:"message"`
}

// Index is the groups whose ops, among those include admits, the caller may
// call, in the order their first op was registered.
func (d Discovery[R]) Index(ctx context.Context, reg *Registry[R], r R, include func(Meta) bool) (Index, error) {
	ops, err := d.visible(ctx, reg, r, include)
	if err != nil {
		return Index{}, err
	}
	idx := Index{Groups: []GroupEntry{}}
	for _, o := range ops {
		g := GroupOf(o.Meta().Name)
		if !slices.ContainsFunc(idx.Groups, func(e GroupEntry) bool { return e.Name == g }) {
			idx.Groups = append(idx.Groups, GroupEntry{Name: g, Summary: reg.groups[g]})
		}
	}
	return idx, nil
}

// Group is group's ops the caller may call; not_found for a group with none.
func (d Discovery[R]) Group(ctx context.Context, reg *Registry[R], r R, include func(Meta) bool, group string) (GroupListing, error) {
	ops, err := d.visible(ctx, reg, r, include)
	if err != nil {
		return GroupListing{}, err
	}
	g := GroupListing{Name: group, Summary: reg.groups[group], Ops: []OpEntry{}}
	for _, o := range ops {
		if m := o.Meta(); GroupOf(m.Name) == group {
			g.Ops = append(g.Ops, OpEntry{Name: m.Name, Summary: m.Summary, ReadOnly: m.ReadOnly, Idempotent: m.Idempotent, Destructive: m.Destructive})
		}
	}
	if len(g.Ops) == 0 {
		return GroupListing{}, apperr.New(apperr.NotFound, fmt.Sprintf("No group %q.", group))
	}
	return g, nil
}

// Usage is the op named name, for a caller who may call it; not_found
// otherwise, alike, so discovery does not reveal an op the caller cannot see.
func (d Discovery[R]) Usage(ctx context.Context, rt *Runtime, reg *Registry[R], r R, include func(Meta) bool, name string) (Usage, error) {
	ops, err := d.visible(ctx, reg, r, include)
	if err != nil {
		return Usage{}, err
	}
	i := slices.IndexFunc(ops, func(o Op[R]) bool { return o.Meta().Name == name })
	if i < 0 {
		return Usage{}, apperr.New(apperr.NotFound, fmt.Sprintf("No operation %q.", name))
	}
	o := ops[i]
	m := o.Meta()
	in, err := rt.Inline(o.InSchema(rt))
	if err != nil {
		return Usage{}, err
	}
	out, err := rt.Inline(rt.Schema(o.OutType()))
	if err != nil {
		return Usage{}, err
	}
	u := Usage{Name: m.Name, Summary: m.Summary, Doc: m.Doc, ReadOnly: m.ReadOnly, Idempotent: m.Idempotent, Destructive: m.Destructive,
		Input: in, Output: out, Errors: []ErrorEntry{}}
	for _, c := range d.codes(m) {
		u.Errors = append(u.Errors, ErrorEntry{Code: c.String(), Scope: string(c.Scope()), Message: c.Message()})
	}
	return u, nil
}

// codes are the public codes an op can fail with: apikit's, the host's, its
// own, as served, each once.
func (d Discovery[R]) codes(m Meta) []apperr.Code {
	codes := []apperr.Code{apperr.InvalidRequest.Code(), apperr.InternalServerError}
	if d.Errors != nil {
		codes = append(codes, d.Errors(m)...)
	}
	for _, c := range m.Errors {
		codes = append(codes, d.Codes.Served(c))
	}
	var out []apperr.Code
	for _, c := range codes {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// visible are reg's ops that include admits and the caller may call.
func (d Discovery[R]) visible(ctx context.Context, reg *Registry[R], r R, include func(Meta) bool) ([]Op[R], error) {
	var ops []Op[R]
	var metas []Meta
	for _, o := range reg.Ops() {
		if m := o.Meta(); include == nil || include(m) {
			ops = append(ops, o)
			metas = append(metas, m)
		}
	}
	if d.Visible == nil {
		return ops, nil
	}
	seen, err := d.Visible(ctx, r, metas)
	if err != nil {
		return nil, err
	}
	var out []Op[R]
	for i, o := range ops {
		if seen[i] {
			out = append(out, o)
		}
	}
	return out, nil
}

// Inline is s as a self-contained JSON Schema: every reference into Schemas
// replaced by what it names. For a document that must stand alone — an MCP
// tool's input, an op's usage. A schema that refers to itself cannot be
// inlined, and is an error.
func (rt *Runtime) Inline(s *huma.Schema) (map[string]any, error) {
	raw, err := jsonkit.Marshal(s)
	if err != nil {
		return nil, err
	}
	var root any
	if err := jsonkit.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	out, err := rt.inline(root, map[string]bool{})
	if err != nil {
		return nil, err
	}
	m, _ := out.(map[string]any)
	return m, nil
}

func (rt *Runtime) inline(v any, seen map[string]bool) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok {
			if seen[ref] {
				return nil, fmt.Errorf("schema %s refers to itself, and cannot be inlined", ref)
			}
			target := rt.Schemas.SchemaFromRef(ref)
			if target == nil {
				return nil, fmt.Errorf("schema %s is not in the registry", ref)
			}
			raw, err := jsonkit.Marshal(target)
			if err != nil {
				return nil, err
			}
			var resolved any
			if err := jsonkit.Unmarshal(raw, &resolved); err != nil {
				return nil, err
			}
			seen[ref] = true
			defer delete(seen, ref)
			return rt.inline(resolved, seen)
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			ie, err := rt.inline(e, seen)
			if err != nil {
				return nil, err
			}
			out[k] = ie
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			ie, err := rt.inline(e, seen)
			if err != nil {
				return nil, err
			}
			out[i] = ie
		}
		return out, nil
	}
	return v, nil
}
