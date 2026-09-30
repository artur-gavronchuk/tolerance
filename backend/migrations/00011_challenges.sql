-- +goose Up
CREATE TABLE challenges (
    id text PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    title text NOT NULL,
    summary text NOT NULL DEFAULT '',
    skill_task_slug text NOT NULL REFERENCES skill_tasks (slug),
    min_tier text NOT NULL DEFAULT 'verified' CHECK (min_tier IN ('none', 'verified', 'strong', 'elite')),
    opens_at timestamptz NOT NULL,
    closes_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed', 'published')),
    prizes text NOT NULL DEFAULT '',
    publish_tests boolean NOT NULL DEFAULT true,
    payout_note text NOT NULL DEFAULT '',
    created_by text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (closes_at > opens_at)
);
CREATE INDEX challenges_status_idx ON challenges (status, closes_at DESC);

-- One row per (challenge, agent): the primary key is what makes "one attempt"
-- an invariant of the schema instead of a check someone can forget.
CREATE TABLE challenge_entries (
    challenge_id text NOT NULL REFERENCES challenges (id),
    agent_id text NOT NULL REFERENCES agents (id),
    version_id text NOT NULL REFERENCES agent_versions (id),
    proof_id text NOT NULL REFERENCES proofs (id),
    consent_publish boolean NOT NULL DEFAULT true,
    score numeric(5,4),
    diff_lines int,
    submitted_at timestamptz,
    rank int,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (challenge_id, agent_id)
);
CREATE INDEX challenge_entries_proof_idx ON challenge_entries (proof_id);

-- Expand-only: proofs gains one nullable column and its kind set widens.
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check
    CHECK (kind IN ('proof', 'game_bot', 'qualification', 'challenge'));
ALTER TABLE proofs ADD COLUMN challenge_id text REFERENCES challenges (id);
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
       (kind IN ('proof', 'game_bot') AND task_slug IS NOT NULL)
    OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3)
    OR (kind = 'challenge' AND skill_task_slug IS NOT NULL AND challenge_id IS NOT NULL));

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
-- The worker opens and closes challenges on time and writes entry scores and
-- ranks; it never creates a challenge or an entry.
GRANT SELECT, UPDATE ON challenges TO arena_worker;
GRANT SELECT, UPDATE ON challenge_entries TO arena_worker;

-- +goose Down
REVOKE ALL ON challenges, challenge_entries FROM arena_worker;
DELETE FROM proofs WHERE kind = 'challenge';
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
       (kind IN ('proof', 'game_bot') AND task_slug IS NOT NULL)
    OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3));
ALTER TABLE proofs DROP COLUMN challenge_id;
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check CHECK (kind IN ('proof', 'game_bot', 'qualification'));
DROP TABLE challenge_entries;
DROP TABLE challenges;
