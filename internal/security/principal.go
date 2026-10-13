package security

import (
	"context"
	"errors"
)

var ErrForbidden = errors.New("permission denied")

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type AccessMode string

const (
	AccessReadOnly         AccessMode = "read_only"
	AccessSelectedTripEdit AccessMode = "selected_trip_edit"
	AccessAllTripsEdit     AccessMode = "all_trips_edit"
)

type Principal struct {
	UserID     string
	Username   string
	Role       Role
	AccessMode AccessMode
	System     bool
}

func (p Principal) IsAdmin() bool { return p.System || p.Role == RoleAdmin }

func SystemPrincipal() Principal {
	return Principal{UserID: "system", Username: "System", Role: RoleAdmin, AccessMode: AccessAllTripsEdit, System: true}
}

func (p Principal) CanCreateTrips() bool {
	return p.IsAdmin() || p.AccessMode == AccessAllTripsEdit
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}
