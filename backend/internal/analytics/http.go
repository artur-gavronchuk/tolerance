package analytics

import (
	"net/http"
	"time"

	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
)

const maxBatch = 20

// RegisterPublicRoutes mounts POST /events, the browser beacon. viewer returns the signed-in user id or "".
// Signed-out visitors who send DNT or Sec-GPC are not recorded at all.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, viewer func(*http.Request) string, limiter *ratelimit.Limiter, trustProxy bool) {
	mux.HandleFunc("POST /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow("events:"+clientip.FromRequest(r, trustProxy), 120, time.Minute) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many events"))
			return
		}
		raw, err := httpx.ReadBodyLimit(w, r, 16<<10)
		var in struct {
			AnonID string `json:"anon_id"`
			Events []struct {
				Name  string `json:"name"`
				Path  string `json:"path"`
				Ref   string `json:"ref"`
				UTM   string `json:"utm"`
				Entry bool   `json:"entry"`
			} `json:"events"`
		}
		if err == nil {
			err = httpx.Decode(raw, &in)
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		uid := viewer(r)
		dnt := r.Header.Get("DNT") == "1" || r.Header.Get("Sec-GPC") == "1"
		anon := in.AnonID
		if dnt || !anonRe.MatchString(anon) {
			anon = ""
		}
		if uid == "" && anon == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var evs []Event
		for _, e := range in.Events {
			if len(evs) == maxBatch {
				break
			}
			if c, ok := Clean(Event{Name: e.Name, UserID: uid, AnonID: anon, Path: e.Path, Ref: e.Ref, UTM: e.UTM, Entry: e.Entry}); ok {
				evs = append(evs, c)
			}
		}
		if err := s.Record(r.Context(), evs); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// RegisterAdminRoutes mounts GET /admin/funnel (the caller adds the session and admin guards).
func RegisterAdminRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/admin/funnel", func(w http.ResponseWriter, r *http.Request) {
		f, err := s.Funnel(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, f)
	})
}
