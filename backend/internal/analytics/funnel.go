package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

const days = 14

// FunnelDay is one UTC day of distinct people at each step. Returned counts the day's visitors who did
// anything on the next day (D1 retention); nil for today, which has no next day yet.
type FunnelDay struct {
	Day      string `json:"day"`
	Visit    int    `json:"visit"`
	Signin   int    `json:"signin"`
	Download int    `json:"download"`
	Upload   int    `json:"upload"`
	Passed   int    `json:"passed"`
	Returned *int   `json:"returned"`
}

type ModeActivity struct {
	Mode   string  `json:"mode"` // daily | tanks
	People int     `json:"people"`
	Events int     `json:"events"`
	Series []Point `json:"series"` // distinct people per day
}

type Point struct {
	Day   string `json:"day"`
	Value int    `json:"value"`
}

type Page struct {
	Path     string `json:"path"`
	Views    int    `json:"views"`
	Visitors int    `json:"visitors"`
}

type Source struct {
	Source string `json:"source"` // utm_source, else referrer host, else "(direct)"
	Visits int    `json:"visits"`
}

type FunnelReport struct {
	Days    []FunnelDay    `json:"days"`
	Modes   []ModeActivity `json:"modes"`
	Pages   []Page         `json:"pages"`
	Sources []Source       `json:"sources"`
}

var modeOf = map[string]string{"daily": "daily", "bot": "tanks"}
var modeOrder = []string{"daily", "tanks"}

// Funnel reads the last 14 UTC days. A person is the user id, or for a signed-out browser that later signed
// in, the user its anon id was seen with, else the anon id itself.
func (s *Service) Funnel(ctx context.Context) (FunnelReport, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, -(days - 1))
	rep := FunnelReport{Days: make([]FunnelDay, days), Modes: []ModeActivity{}, Pages: []Page{}, Sources: []Source{}}
	idx := map[string]int{}
	for i := range rep.Days {
		d := from.AddDate(0, 0, i).Format("2006-01-02")
		rep.Days[i] = FunnelDay{Day: d}
		idx[d] = i
	}
	modes := map[string]*ModeActivity{}
	for _, m := range modeOrder {
		ma := &ModeActivity{Mode: m, Series: make([]Point, days)}
		for i := range rep.Days {
			ma.Series[i] = Point{Day: rep.Days[i].Day}
		}
		modes[m] = ma
	}

	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE ev ON COMMIT DROP AS
			WITH link AS (SELECT anon_id, min(user_id) AS user_id FROM events
				WHERE at >= $1 AND anon_id IS NOT NULL AND user_id IS NOT NULL GROUP BY anon_id)
			SELECT (e.at AT TIME ZONE 'UTC')::date AS day, e.name, COALESCE(e.user_id, l.user_id, 'a:' || e.anon_id) AS pid,
				e.path, e.ref, e.utm, e.entry
			FROM events e LEFT JOIN link l ON l.anon_id = e.anon_id
			WHERE e.at >= $1 AND (e.user_id IS NOT NULL OR e.anon_id IS NOT NULL)`, from); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT day::text,
				count(DISTINCT pid) FILTER (WHERE name = 'page_view'),
				count(DISTINCT pid) FILTER (WHERE name IN ('signin', 'signup')),
				count(DISTINCT pid) FILTER (WHERE name = 'daily.download'),
				count(DISTINCT pid) FILTER (WHERE name = 'daily.upload'),
				count(DISTINCT pid) FILTER (WHERE name = 'daily.passed')
			FROM ev GROUP BY day`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d string
			var v, si, dl, up, ps int
			if err := rows.Scan(&d, &v, &si, &dl, &up, &ps); err != nil {
				rows.Close()
				return err
			}
			if i, ok := idx[d]; ok {
				rep.Days[i].Visit, rep.Days[i].Signin, rep.Days[i].Download, rep.Days[i].Upload, rep.Days[i].Passed = v, si, dl, up, ps
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if rows, err = tx.Query(ctx, `SELECT a.day::text, count(DISTINCT a.pid) FROM ev a
			JOIN ev b ON b.pid = a.pid AND b.day = a.day + 1 WHERE a.name = 'page_view' GROUP BY a.day`); err != nil {
			return err
		}
		for rows.Next() {
			var d string
			var n int
			if err := rows.Scan(&d, &n); err != nil {
				rows.Close()
				return err
			}
			if i, ok := idx[d]; ok {
				rep.Days[i].Returned = &n
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range rep.Days[:days-1] {
			if rep.Days[i].Returned == nil {
				zero := 0
				rep.Days[i].Returned = &zero
			}
		}

		if rows, err = tx.Query(ctx, `SELECT day::text, split_part(name, '.', 1), count(DISTINCT pid), count(*) FROM ev
			WHERE name LIKE 'daily.%' OR name LIKE 'bot.%' GROUP BY 1, 2`); err != nil {
			return err
		}
		for rows.Next() {
			var d, prefix string
			var people, n int
			if err := rows.Scan(&d, &prefix, &people, &n); err != nil {
				rows.Close()
				return err
			}
			if i, ok := idx[d]; ok && modeOf[prefix] != "" {
				ma := modes[modeOf[prefix]]
				ma.Series[i].Value = people
				ma.Events += n
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		// Distinct people over the whole window per mode.
		if rows, err = tx.Query(ctx, `SELECT split_part(name, '.', 1), count(DISTINCT pid) FROM ev
			WHERE name LIKE 'daily.%' OR name LIKE 'bot.%' GROUP BY 1`); err != nil {
			return err
		}
		for rows.Next() {
			var prefix string
			var people int
			if err := rows.Scan(&prefix, &people); err != nil {
				rows.Close()
				return err
			}
			if modeOf[prefix] != "" {
				modes[modeOf[prefix]].People = people
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if rows, err = tx.Query(ctx, `SELECT path, count(*), count(DISTINCT pid) FROM ev
			WHERE name = 'page_view' AND path IS NOT NULL GROUP BY path ORDER BY 2 DESC, path LIMIT 12`); err != nil {
			return err
		}
		for rows.Next() {
			var p Page
			if err := rows.Scan(&p.Path, &p.Views, &p.Visitors); err != nil {
				rows.Close()
				return err
			}
			rep.Pages = append(rep.Pages, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if rows, err = tx.Query(ctx, `SELECT COALESCE(NULLIF(utm, ''), NULLIF(ref, ''), '(direct)') AS src, count(*) FROM ev
			WHERE name = 'page_view' AND entry GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 12`); err != nil {
			return err
		}
		for rows.Next() {
			var src Source
			if err := rows.Scan(&src.Source, &src.Visits); err != nil {
				rows.Close()
				return err
			}
			rep.Sources = append(rep.Sources, src)
		}
		rows.Close()
		return rows.Err()
	})
	for _, m := range modeOrder {
		rep.Modes = append(rep.Modes, *modes[m])
	}
	return rep, err
}
