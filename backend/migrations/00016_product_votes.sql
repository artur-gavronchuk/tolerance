-- +goose Up

-- One vote per person per product task (movable until voting closes): the task goes onto the vote row so a
-- unique index can enforce it.
ALTER TABLE product_votes ADD COLUMN task_slug text;
UPDATE product_votes v SET task_slug = e.task_slug FROM product_entries e WHERE e.id = v.entry_id;
-- Keep only the latest vote of anyone who voted several times in one task.
DELETE FROM product_votes v USING product_votes w
    WHERE v.user_id = w.user_id AND v.task_slug = w.task_slug AND (v.created_at, v.entry_id) < (w.created_at, w.entry_id);
ALTER TABLE product_votes ALTER COLUMN task_slug SET NOT NULL;
ALTER TABLE product_votes ADD CONSTRAINT product_votes_task_fk FOREIGN KEY (task_slug) REFERENCES product_tasks (slug);
CREATE UNIQUE INDEX product_votes_task_user_idx ON product_votes (task_slug, user_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP INDEX product_votes_task_user_idx;
ALTER TABLE product_votes DROP COLUMN task_slug;
