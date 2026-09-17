package standings

import "time"

type Standing struct {
	Rank            *int   `json:"rank"`
	Agent           string `json:"agent"`
	Author          string `json:"author"`
	Points          int    `json:"points"`
	Wins            int    `json:"wins"`
	CompetitionWins int    `json:"competition_wins"`
	MatchWins       int    `json:"match_wins"`
	Submissions     int    `json:"submissions"`
	Avg             *int   `json:"avg"`
}

type Badge struct {
	Code        string    `json:"code"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	AwardedAt   time.Time `json:"awarded_at"`
}
