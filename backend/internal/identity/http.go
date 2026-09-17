package identity

import (
	"net/http"
	"strings"

	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/httpx"
)

// Middleware authenticates the bearer token, resolves it to a platform
// user, and — if X-Organization-Id is present — resolves the caller's role
// in that organization. A request with no organization header still
// reaches the handler (GET /me and POST /organizations need that), just
// with an Actor that has no OrganizationID; RequireOrganization enforces
// the header where a route actually needs it.
func Middleware(verifier *auth.Verifier, service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				httpx.WriteError(w, r, httpx.Unauthenticated("Нужен Bearer-токен."))
				return
			}
			claims, err := verifier.Verify(r.Context(), token)
			if err != nil {
				httpx.WriteError(w, r, httpx.Unauthenticated("Токен недействителен или просрочен."))
				return
			}
			user, err := service.ResolveUser(r.Context(), claims)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			actor := Actor{UserID: user.ID}
			if orgID := r.Header.Get("X-Organization-Id"); orgID != "" {
				role, ok, err := service.ActiveMembership(r.Context(), orgID, user.ID)
				if err != nil {
					httpx.WriteError(w, r, err)
					return
				}
				if !ok {
					httpx.WriteError(w, r, httpx.Forbidden("У вас нет доступа к этой организации."))
					return
				}
				actor.OrganizationID, actor.Role = orgID, role
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), actor)))
		})
	}
}

// RequireOrganization rejects a request that reached an organization-scoped
// route without a resolved X-Organization-Id, rather than letting it fall
// through with a permanently empty OrganizationID.
func RequireOrganization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if MustFromContext(r.Context()).OrganizationID == "" {
			httpx.WriteError(w, r, httpx.New(http.StatusBadRequest, "organization_required", "Укажите заголовок X-Organization-Id."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}

type meResponse struct {
	User        User             `json:"user"`
	Memberships []MembershipView `json:"memberships"`
}

// HandleMe serves GET /api/v1/me: the caller's identity and every
// organization they belong to, so the client can render an org switcher.
func HandleMe(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		memberships, err := service.Memberships(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if memberships == nil {
			memberships = []MembershipView{}
		}
		httpx.Respond(w, http.StatusOK, meResponse{
			User:        User{ID: actor.UserID},
			Memberships: memberships,
		})
	}
}

// RegisterRoutes mounts the routes that need no organization context: they
// must work before one is chosen (GET /me) or before one exists at all
// (POST /organizations). Every other module's routes are organization-
// scoped and sit behind RequireOrganization instead.
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	mux.HandleFunc("GET /api/v1/me", HandleMe(service))
	mux.HandleFunc("POST /api/v1/organizations", HandleCreateOrganization(service))
}

type createOrganizationInput struct {
	Name string `json:"name"`
}

// HandleCreateOrganization serves POST /api/v1/organizations. Any
// authenticated user may create an organization and becomes its owner.
//
// This does not use the idempotency package: idempotency_records is scoped
// by organization_id (so Row Level Security can keep one organization's
// stored responses from another's), which a replay lookup would need to
// know in advance — impossible here, since the whole point of this command
// is that the organization doesn't exist yet. A double-submit creates two
// organizations with the same name, which is visible and correctable by
// the person who did it, not a silent data-integrity problem.
func HandleCreateOrganization(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var input createOrganizationInput
		if err := httpx.Decode(raw, &input); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if !httpx.ValidText(input.Name, 1, 200) {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "Укажите название организации.", "name", "required"))
			return
		}
		org, err := service.CreateOrganizationWithOwner(r.Context(), actor.UserID, strings.TrimSpace(input.Name))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, org)
	}
}
