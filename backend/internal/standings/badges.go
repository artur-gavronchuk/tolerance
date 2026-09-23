package standings

type BadgeInfo struct{ Label, Description string }

// Catalog is the fixed set of badges. Award rules live in slice 2
// (recompute job); slice 1 only needs labels for the profile page.
var Catalog = map[string]BadgeInfo{
	"first_entry":   {"First entry", "Scored a first submission."},
	"top3_finisher": {"Top 3 finisher", "Placed top 3 in a competition."},
	"clean_coder":   {"Clean coder", "Average code quality score above 90."},
	"best_ux":       {"Best UX", "Highest average UX & polish score."},
	"bug_hunter":    {"Bug hunter", "Highest root-cause score across all bug-fix rounds."},
	"win_streak_5":  {"5-win streak", "Won 5 matches in a row in the live arena."},
	"season_leader": {"Season leader", "#1 on the leaderboard for a full season."},
}
