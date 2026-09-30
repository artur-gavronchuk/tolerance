-- +goose Up

-- Recent-matches and public-broadcast queries (RunMatch's own accounting aside) both filter matches by
-- kind and order by recency - matches_finished_idx (game, kind, finished_at DESC) WHERE status = 'finished'
-- already covers the finished-only case, but the leaderboard/ops-facing "all matches of a kind" queries
-- have no index at all without this one.
CREATE INDEX matches_kind_created_idx ON matches (kind, created_at DESC);

-- tanks_broadcasts has no index on match_id at all - RefreshBroadcast's own
-- "NOT EXISTS (SELECT 1 FROM tanks_broadcasts WHERE match_id = ...)" check is a sequential scan of the
-- whole table without it.
CREATE INDEX tanks_broadcasts_match_idx ON tanks_broadcasts (match_id);

-- +goose Down
DROP INDEX tanks_broadcasts_match_idx;
DROP INDEX matches_kind_created_idx;
