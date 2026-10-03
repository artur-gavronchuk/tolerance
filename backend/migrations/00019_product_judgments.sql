-- +goose Up

-- Blind pairwise judgments for site tasks: one per person per unordered pair of entries. entry_a < entry_b
-- always, and winner is relative to that order.
CREATE TABLE product_judgments (
    task_slug  text NOT NULL REFERENCES product_tasks (slug),
    user_id    text NOT NULL REFERENCES users (id),
    entry_a    text NOT NULL REFERENCES product_entries (id) ON DELETE CASCADE,
    entry_b    text NOT NULL REFERENCES product_entries (id) ON DELETE CASCADE,
    winner     text NOT NULL CHECK (winner IN ('a', 'b', 'tie')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, entry_a, entry_b),
    CHECK (entry_a < entry_b)
);
CREATE INDEX product_judgments_task_idx ON product_judgments (task_slug);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE product_judgments;
