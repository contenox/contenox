package vfs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Permission is an int so comparisons are total-order: higher grants more.
// The string form is the wire format; [PermissionFromString] parses it.
type Permission int

const (
	PermissionNone   Permission = 0
	PermissionView   Permission = 1
	PermissionEdit   Permission = 2
	PermissionManage Permission = 3
)

// Wildcard matches any resource or resource type. A grant on "*"/"*" is a
// tenant-wide grant.
const Wildcard = "*"

// ErrAccessDenied is returned when an actor holds no grant covering an
// operation. A caller that must not leak a resource's existence maps it to
// "not found"; the authorization itself does not choose the status.
var ErrAccessDenied = errors.New("vfs: access denied")

var permissionNames = map[Permission]string{
	PermissionNone:   "none",
	PermissionView:   "view",
	PermissionEdit:   "edit",
	PermissionManage: "manage",
}

var permissionValues = map[string]Permission{
	"none":   PermissionNone,
	"view":   PermissionView,
	"edit":   PermissionEdit,
	"manage": PermissionManage,
}

func (p Permission) String() string {
	if name, ok := permissionNames[p]; ok {
		return name
	}
	return "unknown"
}

// PermissionStrings returns the valid permission names, weakest first. It is
// what an API surfaces so a client never spells one itself.
func PermissionStrings() []string {
	return []string{"none", "view", "edit", "manage"}
}

// PermissionFromString parses the wire form, refusing anything unknown so a
// caller can reject it at the edge rather than storing a permission nobody
// declared.
func PermissionFromString(s string) (Permission, error) {
	if v, ok := permissionValues[strings.ToLower(strings.TrimSpace(s))]; ok {
		return v, nil
	}
	return PermissionNone, fmt.Errorf("vfs: invalid permission %q", s)
}

// AccessEntry is one grant: a permission to an identity on a resource, scoped
// to a tenant. Resource and ResourceType may be [Wildcard].
type AccessEntry struct {
	ID           string
	TenantID     string
	Identity     string
	Resource     string
	ResourceType string
	Permission   Permission
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Actor is who a call acts as. TenantID is the tenant whose files are in play,
// UserID the identity grants are written against, and Admin bypasses the list
// entirely — the platform operator and the relay's own server-side calls.
type Actor struct {
	TenantID string
	UserID   string
	Admin    bool
}

type actorContextKey struct{}

// WithActor stamps the acting identity onto ctx. It is how a session's actor
// reaches the [ACL] callbacks, which receive no actor argument.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ActorFromContext returns the actor [WithActor] stamped, or false when none
// was: an unauthenticated call has no grants and therefore no access.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(Actor)
	return actor, ok
}

// AccessList is a set of grants in memory, with the one decision they exist to
// answer. It is separate from the store so a caller holding a tenant's grants
// can authorize without a query per resource.
type AccessList []*AccessEntry

// Allows reports whether any entry grants required on (resourceType, resource).
// Comparison is total-order: an entry's permission satisfies any requirement at
// or below it, and Wildcard matches any type or resource.
func (al AccessList) Allows(resourceType, resource string, required Permission) bool {
	for _, e := range al {
		if e.Permission < required {
			continue
		}
		typeMatch := e.ResourceType == resourceType || e.ResourceType == Wildcard
		resourceMatch := e.Resource == resource || e.Resource == Wildcard
		if typeMatch && resourceMatch {
			return true
		}
	}
	return false
}
