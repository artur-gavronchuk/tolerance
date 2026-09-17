// Package identity resolves who is calling: it turns a verified OIDC
// subject into a platform user, and a user plus an X-Organization-Id header
// into an Actor with a role, which is what every other module authorizes
// against.
package identity

import "context"

// Actor is who is making the request, and in which organization. Role is
// empty when no organization was resolved (e.g. GET /me before the caller
// has picked one).
type Actor struct {
	UserID         string
	OrganizationID string
	Role           string
}

type actorKey struct{}

func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// FromContext returns the actor attached by Middleware. Handlers that are
// reached past Middleware can call MustFromContext instead; this form is
// for code that might run outside an authenticated request (tests, jobs).
func FromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// MustFromContext panics if no actor is attached. Every HTTP handler behind
// Middleware can rely on this never panicking in production; a panic here
// means a route was wired outside the middleware chain, which is a startup
// wiring bug, not a request-time condition to recover from.
func MustFromContext(ctx context.Context) Actor {
	a, ok := FromContext(ctx)
	if !ok {
		panic("identity: handler reached without going through Middleware")
	}
	return a
}
