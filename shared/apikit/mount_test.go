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

func library() *apikit.Registry[team, team] {
	reg := apikit.NewRegistry(apikit.RegistryConfig[team, team]{Namespace: libNS})
	reg.Group("core", "The library's commands.")
	adm := apikit.Admission[team, team]{Admit: func(_ context.Context, t team) (team, error) { return t, nil }}
	apikit.Register(reg, adm, &apikit.Def[team, greetIn, greetOut]{
		Name: "core.greet", Summary: "Greet someone.", Doc: "The library's doc.", HTTP: true, Timeout: time.Minute,
		Errors: []apperr.AnyCode{notAllow},
		// What the host authorizes on is the name as the library reads it.
		Prepare: func(in greetIn) (greetIn, error) {
			in.Name = strings.TrimSpace(in.Name)
			return in, nil
		},
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

// greetRef is the library's ref to core.greet, as a library exports it.
var greetRef = apikit.OpRef[greetIn]{Name: "core.greet"}

func mounting(bound *[]string) apikit.Mounting[request, string, team, greetIn] {
	return apikit.Mounting[request, string, team, greetIn]{
		Admit: admission.Admit,
		Authorize: func(_ context.Context, _ request, user string, in greetIn) (string, error) {
			if user == "bob" && in.Name == "Zed" {
				return "", notAllow
			}
			return user + " (greeting " + in.Name + ")", nil
		},
		Bind: func(_ context.Context, caller string) (team, error) {
			*bound = append(*bound, caller)
			return team("team of " + caller), nil
		},
	}
}

func untyped(bound *[]string) apikit.Mounting[request, string, team, any] {
	return apikit.Mounting[request, string, team, any]{
		Admit: admission.Admit,
		Bind: func(_ context.Context, caller string) (team, error) {
			*bound = append(*bound, caller)
			return team("team of " + caller), nil
		},
	}
}

func TestMount(t *testing.T) {
	lib := library()
	var bound []string
	reg := newRegistry()
	apikit.Mount(reg, lib, greetRef, mounting(&bound), apikit.AppendDoc("Here, only for teams."))
	o, ok := reg.Lookup("core.greet")
	if !ok {
		t.Fatal("the mounted op is not in the host's registry")
	}
	inner, _ := lib.Lookup("core.greet")
	rt := runtime()

	if m := o.Meta(); m.Doc != "The library's doc.\n\nHere, only for teams." || m.Errors[0] != notAllow || !m.HTTP {
		t.Errorf("meta = %+v: want the library's, with the host's doc after it", m)
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
	// Validated before it is authorized: the library's schema refuses first.
	if _, err := o.Invoke(ctx, rt, request{user: "bob"}, []byte(`{"name":""}`)); code(err) != "invalid_request" {
		t.Errorf("invalid input: got %v", err)
	}
	// Authorized on the prepared input: the name trimmed, as the library reads it.
	if _, err := o.Invoke(ctx, rt, request{user: "bob"}, []byte(`{"name":" Zed "}`)); code(err) != "not_allowed" {
		t.Errorf("not authorized on the prepared input: got %v", err)
	}
	if len(bound) != 0 {
		t.Errorf("bound for a refused caller: %v", bound)
	}
	out, err := o.Invoke(ctx, rt, request{user: "ann"}, []byte(`{"name":" Zed "}`))
	if err != nil || out.(greetOut).Text != "team of ann (greeting Zed) greets Zed" {
		t.Errorf("got %v, %v: want the library's handler, on the prepared input, run with what Bind built from Authorize's caller", out, err)
	}
	if _, err := o.Invoke(ctx, rt, request{user: "ann"}, []byte(`{"name":`)); code(err) != "invalid_request" {
		t.Errorf("malformed JSON: got %v", err)
	}
	_, err = o.Invoke(ctx, rt, request{user: "bob"}, []byte(`{"name":"Zed"}`))
	if !strings.HasPrefix(err.Error(), "test_host.op.core.greet(user=bob): ") {
		t.Errorf("the host's step is missing from the log form: %q", err.Error())
	}
}

// The host's shorter timeout bounds the mounted op, as the host names it.
func TestMountTimeout(t *testing.T) {
	var bound []string
	reg := newRegistry()
	apikit.Mount(reg, library(), apikit.Untyped("core.wait"), untyped(&bound), apikit.WithTimeout(10*time.Millisecond))
	o, _ := reg.Lookup("core.wait")
	if _, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, nil); code(err) != "too_slow" {
		t.Errorf("got %v, want the host's timeout code", err)
	}
}

func TestMountRefusesMistakes(t *testing.T) {
	var bound []string
	lib := library()
	mustPanic(t, "not in the registry", func() {
		apikit.Mount(newRegistry(), lib, apikit.OpRef[greetIn]{Name: "core.nope"}, mounting(&bound))
	})
	mustPanic(t, "takes struct {}, not apikit_test.greetIn", func() {
		apikit.Mount(newRegistry(), lib, apikit.OpRef[greetIn]{Name: "core.wait"}, mounting(&bound))
	})
	noBind := mounting(&bound)
	noBind.Bind = nil
	mustPanic(t, "Admit and Bind", func() { apikit.Mount(newRegistry(), lib, greetRef, noBind) })
}

// An untyped ref mounts any op; a library checks the refs it exports.
func TestRefs(t *testing.T) {
	lib := library()
	if err := apikit.CheckRef(lib, greetRef); err != nil {
		t.Errorf("greetRef: %v", err)
	}
	if err := apikit.CheckRef(lib, apikit.Untyped("core.greet")); err != nil {
		t.Errorf("untyped: %v", err)
	}
	if err := apikit.CheckRef(lib, apikit.OpRef[greetIn]{Name: "core.wait"}); err == nil {
		t.Error("a ref to an op of another input passed")
	}
	var bound []string
	reg := newRegistry()
	apikit.Mount(reg, lib, apikit.Untyped("core.greet"), untyped(&bound))
	o, _ := reg.Lookup("core.greet")
	out, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`))
	if err != nil || out.(greetOut).Text != "team of ann greets Zed" {
		t.Errorf("untyped: got %v, %v", out, err)
	}
}
