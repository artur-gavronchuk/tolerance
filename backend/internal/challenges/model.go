package challenges

import "time"

// Statuses a challenge moves through. draft is invisible to the public; open
// accepts entries; closed has places; published also shows the task, the hidden
// test names and the diffs of entrants who agreed to that.
const (
	StatusDraft     = "draft"
	StatusOpen      = "open"
	StatusClosed    = "closed"
	StatusPublished = "published"
)

// Challenge is the operator's view: everything, including the task it hides.
type Challenge struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	SkillTaskSlug string    `json:"skill_task_slug"`
	MinTier       string    `json:"min_tier"`
	OpensAt       time.Time `json:"opens_at"`
	ClosesAt      time.Time `json:"closes_at"`
	Status        string    `json:"status"`
	Prizes        string    `json:"prizes"`
	PublishTests  bool      `json:"publish_tests"`
	PayoutNote    string    `json:"payout_note"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

// NewInput is what an administrator supplies to create a challenge. PublishTests
// is a pointer so leaving it out means the default (true) rather than false.
type NewInput struct {
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	SkillTaskSlug string    `json:"skill_task_slug"`
	MinTier       string    `json:"min_tier"`
	OpensAt       time.Time `json:"opens_at"`
	ClosesAt      time.Time `json:"closes_at"`
	Prizes        string    `json:"prizes"`
	PublishTests  *bool     `json:"publish_tests"`
}

// Entry is one agent's single attempt. Score, DiffLines, SubmittedAt and Rank
// fill in as the proof finishes and the challenge closes.
type Entry struct {
	ChallengeSlug  string     `json:"challenge_slug"`
	Title          string     `json:"title"`
	AgentID        string     `json:"agent_id"`
	VersionID      string     `json:"version_id"`
	ProofID        string     `json:"proof_id"`
	ConsentPublish bool       `json:"consent_publish"`
	Score          *float64   `json:"score"`
	DiffLines      *int       `json:"diff_lines"`
	SubmittedAt    *time.Time `json:"submitted_at"`
	Rank           *int       `json:"rank"`
	CreatedAt      time.Time  `json:"created_at"`
}

// tierRank orders the access tiers so a minimum can be compared. It mirrors
// skillrating.Tier's vocabulary; "none" means anyone rated at all may enter.
var tierRank = map[string]int{"none": 0, "verified": 1, "strong": 2, "elite": 3}

// inProcessVerdictLanguages are the languages where a diff can still reach the
// test harness inside its own process, so a verdict is not yet worth money. A
// prize challenge is refused on these until the harness runs out of process; a
// language leaves this list together with that change, not before.
var inProcessVerdictLanguages = map[string]bool{"python": true}

// Summary is a challenge as a list shows it: no task, no standings.
type Summary struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Status    string    `json:"status"`
	SkillSlug string    `json:"skill_slug"`
	MinTier   string    `json:"min_tier"`
	OpensAt   time.Time `json:"opens_at"`
	ClosesAt  time.Time `json:"closes_at"`
	Prizes    string    `json:"prizes"`
	Entrants  int       `json:"entrants"`
}

// Standing is one place on a closed challenge's table. Diff is filled in only
// after publication, and only for an entrant who agreed to that.
type Standing struct {
	Result
	Diff string `json:"diff"`
}

// PublicView is what anyone can see, and it depends on the status: an open
// challenge shows only its terms and how many agents are in; a closed one shows
// places; a published one also shows the task, the hidden test names and the
// diffs of consenting entrants.
type PublicView struct {
	Summary
	TaskMD      string     `json:"task_md"`
	HiddenTests []string   `json:"hidden_tests"`
	Standings   []Standing `json:"standings"`
}

// Lists groups challenges for the public index. A draft appears nowhere.
type Lists struct {
	Open     []Summary `json:"open"`
	Upcoming []Summary `json:"upcoming"`
	Past     []Summary `json:"past"`
}
