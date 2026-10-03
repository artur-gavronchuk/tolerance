package httpx

import (
	"net/http"
	"strconv"
)

// PageParams reads ?offset&limit: offset defaults to 0, limit to def and is clamped to max.
func PageParams(r *http.Request, def, max int) (offset, limit int, err error) {
	limit = def
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 {
			return 0, 0, WithField(http.StatusUnprocessableEntity, "validation_failed", "limit must be a positive integer", "limit", "invalid")
		}
		limit = min(n, max)
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 0 {
			return 0, 0, WithField(http.StatusUnprocessableEntity, "validation_failed", "offset must be a non-negative integer", "offset", "invalid")
		}
		offset = n
	}
	return offset, limit, nil
}
