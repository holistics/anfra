package apikit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/shared/apperr"

	"github.com/holistics/anfra/shared/apikit"
)

// A library's registry, admitted from its own context (a team name), mounted by
// a host whose requests carry a user (request) and whose caller is that user.

type team string

func library() *apikit.Registry[team] {
	reg := apikit.NewRegistry(apikit.RegistryConfig[team]{Namespace: libNS})
	adm := apikit.Admission[team, team]{Admit: func(_ context.Context, t team) (team, error) { return t, nil }}
	apikit.Register(reg, adm, &apikit.Def[team, greetIn, greetOut]{
		Name: "core.greet", Summary: "Greet someone.", Doc: "The library's doc.", HTTP: true, Timeout: time.Minute,
		Errors: []apperr.AnyCode{notAllow},
		Handle: func(_ context.Context, t team, in greetIn) (greetOut, error) {
			return greetOut{Text: string(t) + " greets " + in.Name, Tags: []string{}}, nil
		},
	})
	apikit.Register(reg, adm, &apikit.Def[team, struct{}, struct{}]{
		Name: "core.wait", Summary: "Wait.", Timeout: time.Minute, Errors: []apperr.AnyCode{tooSlow},
		Handle: func(ctx context.Context, _ team, _ struct{}) (struct{}, error) {
			<-ctx.Done()
			return struct{}{}, ctx.Err()
		},
	})
	return reg
}

var libNS = apperr.DefineNamespace("test_lib_mounted")

func mounting(bound *[]string) apikit.Mounting[request, string, team] {
	return apikit.Mounting[request, string, team]{
		Admit: admission.Admit,
		Authorize: func(_ context.Context, _ request, user string, input apikit.Args) error {
			if user == "bob" && input["name"] == "Zed" {
				return notAllow
			}
			return nil
		},
		Bind: func(_ context.Context, user string) (team, error) {
			*bound = append(*bound, user)
			return team("team of " + user), nil
		},
		Overlay: func(m apikit.Meta) apikit.Meta {
			m.Doc += " Here, only for teams."
			return m
		},
	}
}

func TestMount(t *testing.T) {
	lib := library()
	var bound []string
	reg := newRegistry()
	apikit.Mount(reg, lib, "core.greet", mounting(&bound))
	o, ok := reg.Lookup("core.greet")
	if !ok {
		t.Fatal("the mounted op is not in the host's registry")
	}
	inner, _ := lib.Lookup("core.greet")
	rt := runtime()

	if m := o.Meta(); m.Doc != "The library's doc. Here, only for teams." || m.Errors[0] != notAllow || !m.HTTP {
		t.Errorf("meta = %+v: want the library's, with the overlay's doc", m)
	}
	if o.InSchema(rt).Ref != inner.InSchema(rt).Ref || o.OutType() != inner.OutType() {
		t.Error("the mounted op's schemas are not the library's")
	}

	ctx := context.Background()
	if _, err := o.Invoke(ctx, rt, request{}, []byte(`{"name":"Ann"}`)); code(err) != "no_entry" {
		t.Errorf("not admitted: got %v", err)
	}
	if _, err := o.Invoke(ctx, rt, request{user: "bob"}, []byte(`{"name":"Zed"}`)); code(err) != "not_allowed" {
		t.Errorf("not authorized: got %v", err)
	}
	if len(bound) != 0 {
		t.Errorf("bound for a refused caller: %v", bound)
	}
	// Authorized first, validated after: the library's schema still refuses.
	if _, err := o.Invoke(ctx, rt, request{user: "ann"}, []byte(`{"name":""}`)); code(err) != "invalid_request" {
		t.Errorf("invalid input: got %v", err)
	}
	out, err := o.Invoke(ctx, rt, request{user: "ann"}, []byte(`{"name":"Zed"}`))
	if err != nil || out.(greetOut).Text != "team of ann greets Zed" {
		t.Errorf("got %v, %v: want the library's handler, run with what Bind built", out, err)
	}
	if _, err := o.Invoke(ctx, rt, request{user: "ann"}, []byte(`{"name":`)); code(err) != "invalid_request" {
		t.Errorf("malformed JSON: got %v", err)
	}
	_, err = o.Invoke(ctx, rt, request{user: "bob"}, []byte(`{"name":"Zed"}`))
	if !strings.HasPrefix(err.Error(), "test_host.op.core.greet(user=bob): ") {
		t.Errorf("the host's step is missing from the log form: %q", err.Error())
	}
}

// An overlay's shorter timeout bounds the mounted op, as the host names it.
func TestMountTimeout(t *testing.T) {
	var bound []string
	reg := newRegistry()
	m := mounting(&bound)
	m.Overlay = func(meta apikit.Meta) apikit.Meta { meta.Timeout = 10 * time.Millisecond; return meta }
	apikit.Mount(reg, library(), "core.wait", m)
	o, _ := reg.Lookup("core.wait")
	if _, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, nil); code(err) != "too_slow" {
		t.Errorf("got %v, want the host's timeout code", err)
	}
}

func TestMountRefusesMistakes(t *testing.T) {
	var bound []string
	lib := library()
	mustPanic(t, "not in the registry", func() { apikit.Mount(newRegistry(), lib, "core.nope", mounting(&bound)) })
	renamed := mounting(&bound)
	renamed.Overlay = func(m apikit.Meta) apikit.Meta { m.Name = "core.other"; return m }
	mustPanic(t, "may not rename", func() { apikit.Mount(newRegistry(), lib, "core.greet", renamed) })
	noBind := mounting(&bound)
	noBind.Bind = nil
	mustPanic(t, "Admit and Bind", func() { apikit.Mount(newRegistry(), lib, "core.greet", noBind) })
}
