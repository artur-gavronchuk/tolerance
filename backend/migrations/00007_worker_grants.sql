-- +goose Up

-- Least-privilege grants for arena_worker (see 00006_worker_role.go): exactly the tables/columns the
-- sandbox worker's own code paths touch (internal/proofs.Worker, internal/games.Worker and their shared
-- internal/platform/jobs queue), enumerated by reading that code, not copied from arena_app's grants.
-- Deliberately NOT granted: users, sessions, user_identities, api_keys, agent_presence - none of the
-- worker's code paths touch them, and a sandbox escape on a worker host must not reach them either. Future
-- tables do not leak to arena_worker: unlike arena_app (see 00002_schema.sql/00003_games.sql), there is no
-- ALTER DEFAULT PRIVILEGES for it, so a new table needs an explicit grant here, by design.
GRANT USAGE ON SCHEMA public TO arena_worker;

-- Job queue: Claim/Complete/Fail/Reclaim (internal/platform/jobs) need SELECT (their WHERE/RETURNING
-- clauses) and UPDATE; Enqueue (a ladder match's run_match job, games.createLadderMatch) needs INSERT. The
-- worker never deletes a job.
GRANT SELECT, INSERT, UPDATE ON jobs TO arena_worker;

-- Proof lifecycle: RunProof/finish/MarkInfraError/ExpireStale (internal/proofs.Worker) read and update a
-- proof's own columns (status, diff, kind, sandbox_result, ...); proof_tasks (image, run_cmd, timeouts,
-- the repo/hidden tarballs) is read-only from here - only cmd/migrate's catalog sync writes it. The worker
-- never creates or deletes a proof.
GRANT SELECT, UPDATE ON proofs TO arena_worker;
GRANT SELECT ON proof_tasks TO arena_worker;

-- Tanks: bots, versions, matches and their players are read/written across RunMatch, Qualify, ScheduleTick,
-- JudgeProof's createVersion and the periodic sweeps (SweepStuck, sweepFailedChecks, markMatchPlatformFailure,
-- markCheckPlatformFailure). match_replays additionally needs DELETE for PruneReplays' hourly cleanup.
-- tanks_broadcasts is only ever read and inserted (RefreshBroadcast), never updated or deleted.
GRANT SELECT, INSERT, UPDATE ON game_bots, bot_versions, matches, match_players TO arena_worker;
GRANT SELECT, INSERT, UPDATE, DELETE ON match_replays TO arena_worker;
GRANT SELECT, INSERT ON tanks_broadcasts TO arena_worker;

-- Audit trail: a bot auto-created for an agent's first tanks run (games.ensureBot, reached from
-- proofs.Worker's GameBotJudge path) records an audit_events row. The worker only ever appends; it never
-- reads the audit log back.
GRANT INSERT ON audit_events TO arena_worker;

-- Just enough of agents to resolve an owner's user id from their agent id (JudgeProof) and their agent's
-- display name from their user id (games.agentNameFor, used to name a bot the first time an agent runs) -
-- nothing else about an agent or its owner (description, created_at, version) is exposed, and no other
-- user-identifying table (users, sessions, user_identities, api_keys) is reachable at all.
GRANT SELECT (id, owner_user_id, name) ON agents TO arena_worker;

-- +goose Down
REVOKE ALL ON agents FROM arena_worker;
REVOKE ALL ON audit_events FROM arena_worker;
REVOKE ALL ON tanks_broadcasts, match_replays, match_players, matches, bot_versions, game_bots FROM arena_worker;
REVOKE ALL ON proof_tasks, proofs FROM arena_worker;
REVOKE ALL ON jobs FROM arena_worker;
REVOKE USAGE ON SCHEMA public FROM arena_worker;
