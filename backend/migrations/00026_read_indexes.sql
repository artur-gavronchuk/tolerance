-- +goose Up
-- Indexes for the read paths that scan every submission (overall leaderboard, streaks, profiles, stacks, admin
-- pulse); found with a seeded 5000-person database (cmd/seed). Partial, so they only hold graded daily rows.

-- Streaks and solved days: DISTINCT (user, day) of passed daily submissions, as an index-only scan.
CREATE INDEX submissions_solved_idx ON submissions (user_id, day) WHERE day IS NOT NULL AND status = 'passed';

-- Overall points: each person's best graded attempt per day.
CREATE INDEX submissions_graded_idx ON submissions (user_id, day) INCLUDE (task_slug, passed_tests, total_tests, score)
    WHERE day IS NOT NULL AND status IN ('passed', 'failed') AND passed_tests > 0 AND total_tests > 0;

-- The day's best score per task (optimize days only; bugfix rows have no score).
CREATE INDEX submissions_scored_idx ON submissions (day) INCLUDE (score, passed_tests, total_tests) WHERE day IS NOT NULL AND score IS NOT NULL;

-- Admin pulse and "last 7 days" counters.
CREATE INDEX submissions_created_idx ON submissions (created_at);
CREATE INDEX users_created_idx ON users (created_at);
CREATE INDEX product_entries_created_idx ON product_entries (created_at);

-- Profile lookups are case-insensitive.
CREATE INDEX users_handle_lower_idx ON users (lower(handle));

-- +goose Down
DROP INDEX users_handle_lower_idx;
DROP INDEX product_entries_created_idx;
DROP INDEX users_created_idx;
DROP INDEX submissions_created_idx;
DROP INDEX submissions_scored_idx;
DROP INDEX submissions_graded_idx;
DROP INDEX submissions_solved_idx;
