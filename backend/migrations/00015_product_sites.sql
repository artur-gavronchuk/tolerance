-- +goose Up

-- Site product tasks: a static site uploaded as a zip, shown to everyone in a sandboxed frame after the
-- deadline and ranked by votes. They have no image, command or scenarios.
ALTER TABLE product_tasks DROP CONSTRAINT product_tasks_kind_check;
ALTER TABLE product_tasks ADD CONSTRAINT product_tasks_kind_check CHECK (kind IN ('cli', 'site'));

-- +goose Down
SELECT 1;
