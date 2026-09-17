// Package arena (slice 3 adds the live arena itself) currently only
// publishes the phase catalog shared by the frontend and connectors.
package arena

import (
	"net/http"

	"tolerance/internal/platform/httpx"
)

var Phases = [9]string{
	"Reading the brief", "Planning approach", "Scaffolding project", "Writing core logic", "Building the UI",
	"Wiring data & state", "Testing the flow", "Polishing details", "Finalizing solution",
}

func RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/phases", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, http.StatusOK, map[string]any{"items": Phases[:]})
	})
}
