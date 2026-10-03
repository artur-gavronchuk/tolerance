-- +goose Up

-- Product tasks run one per week. opens_at and deadline now belong to a task's current run and stay NULL until
-- the task is picked (see products.rotate). A recycled task's earlier run is frozen into its own row
-- (frozen = true, slug "<slug>-w<date>") so its entries and votes stay with that run.
ALTER TABLE product_tasks ALTER COLUMN opens_at DROP NOT NULL;
ALTER TABLE product_tasks ALTER COLUMN opens_at DROP DEFAULT;
ALTER TABLE product_tasks ALTER COLUMN deadline DROP NOT NULL;
ALTER TABLE product_tasks ADD COLUMN ord int NOT NULL DEFAULT 0;
ALTER TABLE product_tasks ADD COLUMN frozen boolean NOT NULL DEFAULT false;

-- Tasks somebody already entered become past runs (voting or final); the rest wait for their week.
UPDATE product_tasks t SET
    opens_at = date_trunc('week', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' - interval '7 days',
    deadline = least(t.deadline, now())
WHERE EXISTS (SELECT 1 FROM product_entries e WHERE e.task_slug = t.slug);
UPDATE product_tasks t SET opens_at = NULL, deadline = NULL
WHERE NOT EXISTS (SELECT 1 FROM product_entries e WHERE e.task_slug = t.slug);

-- +goose Down
SELECT 1;
