// Package apperr gives errors their meaning: a code a caller can handle, which
// side is responsible (scope), a default message for quick display, typed
// details for programmatic handling, and the steps the error passed through.
//
// A formal error is made with New or Encapsulate, or by returning a code
// itself. Steps are added by apptracing when a step ends. Everything else —
// a plain error, a fmt.Errorf wrap, another module's error, a join — is
// unclassified, and is rendered as internal_server_error.
//
// Codes are public or internal. Public codes are the contract with clients:
// this package defines the few generic to any host, and a host defines the rest
// with DefinePublicCode. Any package may define internal codes for its own callers to
// handle; each maps to a defined code, which is what a client sees.
//
// Every code belongs to a namespace, one per module, so modules name their codes
// independently: a name is unique within its namespace. A client sees the bare
// name (not_found), the log the qualified one (apperr.not_found).
//
// A library's codes are its own contract, not its host's. A host serves only
// its own codes and the generic ones: where it calls the library, it translates
// the library's errors to its own codes (Translate), which re-decides their
// scope, and refuses to serve any it did not.
//
// A code knows nothing of any transport: an HTTP status is for the HTTP
// transport to assign, per public code.
package apperr

import (
	"reflect"
	"sort"

	"github.com/holistics/anfra/shared/appstep"
)

// Scope annotates which side is responsible for an error: the person, the
// calling program, or us.
type Scope string

const (
	// User: the request was well-formed, and the person's input or the state
	// they are acting on refuses it.
	User Scope = "user"
	// Client: the calling program is wrong. The person did nothing to cause it.
	Client Scope = "client"
	// Server: we are wrong, or something we depend on is down.
	Server Scope = "server"
)

// Namespace is a module's namespace, shared with its steps: a code's name is
// unique within its namespace, and its qualified name, namespace.name,
// everywhere. Declare one per module with DefineNamespace.
type Namespace = appstep.Namespace

// DefineNamespace declares a module's namespace, for its codes and its steps.
// Call it once per module, from a package-level var.
func DefineNamespace(name string) Namespace { return appstep.DefineNamespace(name) }

type code struct {
	ns      Namespace
	name    string
	scope   Scope
	message string
	public  *code        // the code an internal code maps to; nil for a defined (non-internal) code
	details reflect.Type // a defined code's details type; nil when it has none
}

// Code identifies a kind of failure. It is comparable, and it is itself an
// error: returning a code returns a formal error with the code's default
// message, and errors.Is(err, code) reports whether err was caused by a
// failure with that code.
//
// Codes are made only by the Define functions, so a code cannot be invented at
// a call site or paired with the wrong scope.
type Code struct{ c *code }

// TypedCode is a code whose details have a type, so they are checked where the
// error is made.
//
// It embeds Code through the unexported alias baseCode, and that is deliberate.
// The embedded field is then unexported, so no TypedCode literal compiles outside
// this package — keyed or not — and only DefinePublicCodeWith and DefineInternalCodeWith
// can pair a code with a details type; yet Code's methods are still promoted.
// Embedding Code directly would let any package write TypedCode[X]{Code: c} and
// give a code a details type its documentation, and the OpenAPI built from it,
// does not have. Do not "simplify" it back.
type TypedCode[D any] struct{ baseCode }

type baseCode = Code

// AnyCode is a Code or a TypedCode.
type AnyCode interface {
	error
	Code() Code
}

// Code returns c itself, so that a Code satisfies AnyCode. The method exists for
// TypedCode, whose Code is otherwise unreachable from outside this package.
func (c Code) Code() Code { return c }

// Code returns the code underneath the typing: what an *Error's Code field holds,
// and what to compare one with.
func (t TypedCode[D]) Code() Code { return t.baseCode }

// Codes are errors by value, never by pointer: comparable, usable as map keys,
// and returned bare (`return apperr.Unauthenticated`), like syscall.Errno. A *Code
// would still satisfy error, but == and type switches on Code would miss it.
var (
	_ error = Code{}
	_ error = TypedCode[Violations]{}
)

// String is the code's name, as a client sees it.
func (c Code) String() string {
	if c.c == nil {
		return ""
	}
	return c.c.name
}

// Qualified is the code's name qualified by its namespace, as the log sees it:
// apperr.not_found.
func (c Code) Qualified() string {
	if c.c == nil {
		return ""
	}
	return c.c.ns.String() + "." + c.c.name
}

// Namespace is the namespace the code was defined in.
func (c Code) Namespace() Namespace {
	if c.c == nil {
		return Namespace{}
	}
	return c.c.ns
}

// Public is the public code a client sees: the code itself if it is public,
// the code it maps to if it is internal.
func (c Code) Public() Code {
	if c.c != nil && c.c.public != nil {
		return Code{c.c.public}
	}
	return c
}

// IsPublic reports whether the code is part of its module's contract with
// clients: a defined code, not an internal one.
func (c Code) IsPublic() bool { return c.c != nil && c.c.public == nil }

// Scope is which side the code says is responsible — its public code's.
func (c Code) Scope() Scope { return c.Public().c.scope }

// Message is the code's default message.
func (c Code) Message() string { return c.c.message }

// Error is the log form of a bare code: its qualified name and default message.
func (c Code) Error() string { return c.Qualified() + ": " + c.c.message }

// Is lets errors.Is match a code by the code itself or by the public code it
// maps to, so a check for validation_failed also matches an internal code that
// renders as one.
func (c Code) Is(target error) bool { return matches(c, target) }

// As lets errors.As find a bare code as a formal error, with the code's
// default message.
func (c Code) As(target any) bool {
	if p, ok := target.(**Error); ok {
		*p = &Error{Code: c, Message: c.c.message}
		return true
	}
	return false
}

func matches(c Code, target error) bool {
	t, ok := target.(AnyCode)
	if !ok {
		return false
	}
	tc := t.Code()
	return tc == c || tc == c.Public()
}

var (
	// registered is every code, in definition order.
	registered []Code
	// qualified holds every code's qualified name; genericNames the generic
	// codes' bare names, which no other namespace may define.
	qualified    = map[string]bool{}
	genericNames = map[string]bool{}
)

// DefinePublicCode defines a public code of ns: one a client can handle, with the scope
// it states and a default message for quick display. Call it from a
// package-level var, once per name; a name already in ns or one of the generic
// codes' names, no scope, or no message panics at startup.
//
// Mint a public code only when a client should act differently from how it
// acts on every existing one.
func DefinePublicCode(ns Namespace, name string, scope Scope, message string) Code {
	switch scope {
	case User, Client, Server:
	default:
		panic("apperr: code " + name + " has no scope")
	}
	if ns != Generic && genericNames[name] {
		panic("apperr: code " + ns.String() + "." + name + " takes the name of a generic code")
	}
	c := define(ns, name, scope, message, nil)
	if ns == Generic {
		genericNames[name] = true
	}
	return c
}

// DefinePublicCodeWith is DefinePublicCode for a code whose details have type D, recording D so
// the contract published from the catalog (OpenAPI, the generated client)
// types them.
func DefinePublicCodeWith[D any](ns Namespace, name string, scope Scope, message string) TypedCode[D] {
	c := DefinePublicCode(ns, name, scope, message)
	c.c.details = reflect.TypeFor[D]()
	return TypedCode[D]{c}
}

// DetailsType is the type of the details a client receives with c: its public
// code's, since an internal code's details are its public code's. Nil when the
// code carries none.
func (c Code) DetailsType() reflect.Type { return c.Public().c.details }

func define(ns Namespace, name string, scope Scope, message string, public *code) Code {
	if ns.String() == "" {
		panic("apperr: code " + name + " has no namespace")
	}
	if name == "" || message == "" {
		panic("apperr: a code needs a name and a default message")
	}
	q := ns.String() + "." + name
	if qualified[q] {
		panic("apperr: duplicate code " + q)
	}
	qualified[q] = true
	c := Code{&code{ns: ns, name: name, scope: scope, message: message, public: public}}
	registered = append(registered, c)
	return c
}

// DefineInternalCode defines a code of ns for callers inside its module to
// handle. A client sees public instead, with this code's message carried over,
// so the message must be safe for callers.
func DefineInternalCode(ns Namespace, name string, public Code, message string) Code {
	if public.c == nil || public.c.public != nil {
		panic("apperr: internal code " + name + " must map to a defined code, not an internal one")
	}
	return define(ns, name, "", message, public.c)
}

// DefineInternalCodeWith is DefineInternalCode for a public code with typed
// details: the internal code's details have the same type, so they carry over.
func DefineInternalCodeWith[D any](ns Namespace, name string, public TypedCode[D], message string) TypedCode[D] {
	return TypedCode[D]{DefineInternalCode(ns, name, public.baseCode, message)}
}

// Codes returns ns's public codes, sorted by name: its module's contract with
// clients. A host's contract is its own namespace's and Generic's.
func Codes(ns Namespace) []Code {
	var out []Code
	for _, c := range registered {
		if c.IsPublic() && c.c.ns == ns {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// Generic is the namespace of the codes generic to any host, whose names no
// other namespace may define.
var Generic = DefineNamespace("apperr")

// The codes generic to any host. A host defines the rest of its contract with
// DefinePublicCode.
var (
	InternalServerError = DefinePublicCode(Generic, "internal_server_error", Server, "Something went wrong on our side.")
	InvalidRequest      = DefinePublicCodeWith[Violations](Generic, "invalid_request", Client, "The request is not valid.")
	NotFound            = DefinePublicCode(Generic, "not_found", User, "Not found.")
	// ValidationFailed: the request was well-formed, and its input's meaning is
	// refused — a value out of range, a name already taken, an arg that does not
	// apply. Its violations name each field.
	ValidationFailed = DefinePublicCodeWith[Violations](Generic, "validation_failed", User, "Some of the input is not valid.")
)

// Violations are field-level details: one entry per field the caller got
// wrong. invalid_request carries them for structural errors, validation_failed
// for the input's meaning, and a host's own codes may carry them too.
type Violations []Violation

// Violation is one field-level problem. Field is the request field or engine
// arg name as the caller sent it; Code is from a small generic set (required,
// invalid, unknown, unsupported, too_short, too_long, taken).
type Violation struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
