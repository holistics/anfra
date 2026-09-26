// Package authz answers one question: may this principal perform these actions
// on these resources, within their tenant?
//
// Out of scope, by design:
//
//   - Tenant isolation, which is enforced structurally by scoping store handles
//     to a tenant, never by a rule here.
//   - Authentication. This package consumes a Principal; producing one is authn's.
//   - Row/column security on query results, which is applied as a query rewrite at
//     AML/AQL compile time.
//
// Policy is the decision point; app.Dispatch is the single enforcement point.
package authz

import (
	"context"
	"fmt"
)

// TenantID identifies a tenant. It rides on Principal for audit and for policies
// that need it; it is not the mechanism keeping tenants apart.
type TenantID string

// Role is a named set of capabilities within a tenant.
type Role string

// Principal is who is asking. The zero value is nobody and is never authorized
// by a real policy.
type Principal struct {
	TenantID TenantID
	UserID   string
	Roles    []Role
}

// Action is what is being attempted.
type Action string

const (
	ActionRead  Action = "read"
	ActionWrite Action = "write"
	ActionQuery Action = "query"
	ActionAdmin Action = "admin"
)

// ResourceKind is the type of thing being acted on.
type ResourceKind string

const (
	KindRepo       ResourceKind = "repo"
	KindDataset    ResourceKind = "dataset"
	KindDashboard  ResourceKind = "dashboard"
	KindDataSource ResourceKind = "datasource"
)

// Resource is the thing being acted on. ID is empty when the action covers the
// kind as a whole rather than one instance.
type Resource struct {
	Kind ResourceKind
	ID   string
}

// Permission is one (action, resource) pair.
type Permission struct {
	Action   Action
	Resource Resource
}

func (p Permission) String() string {
	if p.Resource.ID == "" {
		return fmt.Sprintf("%s:%s", p.Action, p.Resource.Kind)
	}
	return fmt.Sprintf("%s:%s/%s", p.Action, p.Resource.Kind, p.Resource.ID)
}

// Policy decides. Implementations must be safe for concurrent use. A refusal is
// a *DeniedError naming the permission that failed.
type Policy interface {
	Authorize(ctx context.Context, p Principal, perms []Permission) error
}

// DeniedError reports the specific permission that was refused.
type DeniedError struct {
	Principal  Principal
	Permission Permission
}

func (e *DeniedError) Error() string {
	who := e.Principal.UserID
	if who == "" {
		who = "principal"
	}
	return fmt.Sprintf("%s is not permitted to %s", who, e.Permission)
}

// AllowAll permits everything. Used by the CLI, whose user owns the repo they
// are pointed at.
type AllowAll struct{}

func (AllowAll) Authorize(context.Context, Principal, []Permission) error { return nil }

// DenyAll refuses everything. For tests.
type DenyAll struct{}

func (DenyAll) Authorize(_ context.Context, p Principal, perms []Permission) error {
	if len(perms) == 0 {
		return nil
	}
	return &DeniedError{Principal: p, Permission: perms[0]}
}

// Public is the permission set of a command that needs no authorization, shaped
// as a Requires function so a command declares it as `Requires: authz.Public`.
// A nil Requires is not equivalent: Dispatch refuses it.
func Public(map[string]any) []Permission { return nil }

// LocalOwner is the principal the CLI runs as.
func LocalOwner() Principal {
	return Principal{UserID: "local"}
}
