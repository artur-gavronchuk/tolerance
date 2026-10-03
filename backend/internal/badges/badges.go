// Package badges serves small flat SVG badges (shields.io look) people paste into a README: a person's place on
// the overall daily board with their streak, and a tanks bot's season rank with its rating. Unknown, banned,
// deleted and house subjects get a grey "not found" badge with status 404, so moderation hides them here too.
package badges

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"tolerance/internal/daily"
	"tolerance/internal/games"
	"tolerance/internal/platform/httpx"
)

const (
	colorLabel = "#24292f"
	colorGreen = "#2da44e"
	colorBlue  = "#0969da"
	colorGrey  = "#6e7781"
)

// Deps are the services the badges read through; they apply the visibility rules (banned, hidden, house).
type Deps struct {
	Daily *daily.Service
	Games *games.Service
}

func RegisterPublicRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /api/v1/badges/u/{file}", func(w http.ResponseWriter, r *http.Request) {
		handle, ok := strings.CutSuffix(r.PathValue("file"), ".svg")
		if !ok {
			write(w, http.StatusNotFound, notFound())
			return
		}
		p, err := d.Daily.ProfileOf(r.Context(), handle)
		if bad(w, err) {
			return
		}
		place := "unranked"
		if p.Place != nil {
			place = fmt.Sprintf("#%d", *p.Place)
		}
		msg := place
		if p.Streak.Current > 0 {
			msg += fmt.Sprintf(" · %d-day streak", p.Streak.Current)
		}
		write(w, http.StatusOK, render("tolerance", msg, colorGreen))
	})
	mux.HandleFunc("GET /api/v1/badges/bot/{file}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutSuffix(r.PathValue("file"), ".svg")
		if !ok {
			write(w, http.StatusNotFound, notFound())
			return
		}
		b, err := botProfile(r.Context(), d.Games, id)
		if bad(w, err) {
			return
		}
		if b.House {
			write(w, http.StatusNotFound, notFound())
			return
		}
		msg := fmt.Sprintf("%d", b.Rating)
		if b.Rank > 0 {
			msg = fmt.Sprintf("#%d · %d", b.Rank, b.Rating)
		}
		write(w, http.StatusOK, render("tolerance tanks", msg, colorBlue))
	})
}

func botProfile(ctx context.Context, g *games.Service, id string) (games.BotProfile, error) {
	return g.Bot(ctx, id)
}

// bad answers a lookup error: a not-found badge for 404s, a plain error otherwise. It reports whether it wrote.
func bad(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var p *httpx.Problem
	if errors.As(err, &p) && p.Status == http.StatusNotFound {
		write(w, http.StatusNotFound, notFound())
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "badge unavailable", http.StatusInternalServerError)
	return true
}

func notFound() []byte { return render("tolerance", "not found", colorGrey) }

func write(w http.ResponseWriter, status int, svg []byte) {
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	if status == http.StatusOK {
		h.Set("Cache-Control", "public, max-age=600")
	} else {
		h.Set("Cache-Control", "public, max-age=60")
	}
	w.WriteHeader(status)
	_, _ = w.Write(svg)
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// textWidth approximates Verdana 11px: wider for capitals and digits, narrower for thin letters.
func textWidth(s string) int {
	w := 0.0
	for _, r := range s {
		switch {
		case strings.ContainsRune("iljI.,:;'| !", r):
			w += 3.6
		case strings.ContainsRune("frt-()", r):
			w += 5
		case r >= 'A' && r <= 'Z', strings.ContainsRune("mwMW", r), r > 0x2000:
			w += 8
		default:
			w += 6.6
		}
	}
	return int(w+0.5) + 10
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// render draws a two-part flat badge: dark label, coloured message. Text is XML-escaped; the content is
// clipped so a long user-supplied handle cannot blow the badge up.
func render(label, msg, color string) []byte {
	label, msg = clip(label, 24), clip(msg, 40)
	lw, mw := textWidth(label), textWidth(msg)
	total := lw + mw
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">`, total, esc(label), esc(msg))
	fmt.Fprintf(&b, `<title>%s: %s</title>`, esc(label), esc(msg))
	fmt.Fprintf(&b, `<linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>`)
	fmt.Fprintf(&b, `<clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>`, total)
	fmt.Fprintf(&b, `<g clip-path="url(#r)"><rect width="%d" height="20" fill="%s"/><rect x="%d" width="%d" height="20" fill="%s"/><rect width="%d" height="20" fill="url(#s)"/></g>`,
		lw, colorLabel, lw, mw, color, total)
	fmt.Fprintf(&b, `<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">`)
	fmt.Fprintf(&b, `<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text><text x="%d" y="14">%s</text>`, lw/2, esc(label), lw/2, esc(label))
	fmt.Fprintf(&b, `<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text><text x="%d" y="14">%s</text>`, lw+mw/2, esc(msg), lw+mw/2, esc(msg))
	b.WriteString(`</g></svg>`)
	return b.Bytes()
}
