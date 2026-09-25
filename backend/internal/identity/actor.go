package identity

import "context"

const (
	KindUser   = "user"
	KindAgent  = "agent"
	KindSystem = "system"
)

// Actor is who is making the request. ID is the audit/idempotency actor id
// (user_… or agent_…). For an agent actor UserID is the owner.
type Actor struct {
	Kind    string
	ID      string
	UserID  string
	AgentID string
	Role    string // "user" | "admin" for users; "" for agents
}

// System is the actor for scheduler and worker writes.
var System = Actor{Kind: KindSystem, ID: "system"}

type actorKey struct{}

func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// FromContext returns the actor attached by a Require* middleware.
func FromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// MustFromContext panics if no actor is attached. Every HTTP handler behind
// a Require* middleware can rely on this never panicking in production; a
// panic here means a route was wired outside the middleware chain, which is
// a startup wiring bug, not a request-time condition to recover from.
func MustFromContext(ctx context.Context) Actor {
	a, ok := FromContext(ctx)
	if !ok {
		panic("identity: handler reached without an authenticating middleware")
	}
	return a
}

// ActorLog is a small mutable holder an outer HTTP middleware (the request
// logger) places into the context before calling into the auth middleware
// chain. RequireSession/RequireAgent fill in the id they resolve as the
// request passes through. This exists because r.WithContext returns a new
// *http.Request; the outer middleware's own r value never sees a context
// value attached deeper in the chain, so FromContext there would always
// miss. A pointer stored in the context is the one thing both sides share:
// mutating *ActorLog through it is visible to whoever holds the pointer,
// context copies notwithstanding.
type ActorLog struct {
	UserID  string
	AgentID string
}

type actorLogKey struct{}

// WithActorLog attaches h for a downstream Require* middleware to fill in.
func WithActorLog(ctx context.Context, h *ActorLog) context.Context {
	return context.WithValue(ctx, actorLogKey{}, h)
}

// actorLogFromContext returns the holder placed by WithActorLog, or nil on
// a route with no request logger wired in front (e.g. a test that calls a
// handler directly).
func actorLogFromContext(ctx context.Context) *ActorLog {
	h, _ := ctx.Value(actorLogKey{}).(*ActorLog)
	return h
}
