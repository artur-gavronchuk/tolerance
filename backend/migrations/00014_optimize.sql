-- +goose Up
ALTER TABLE tasks
    ADD COLUMN kind text NOT NULL DEFAULT 'bugfix' CHECK (kind IN ('bugfix', 'optimize')),
    ADD COLUMN direction text CHECK (direction IN ('max', 'min')),
    ADD COLUMN solve_cmd text NOT NULL DEFAULT '',
    ADD COLUMN case_time_limit_s int NOT NULL DEFAULT 0,
    ADD COLUMN cases int NOT NULL DEFAULT 0;
ALTER TABLE submissions
    ADD COLUMN score double precision,
    ADD COLUMN cases_valid int NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
