-- +goose Up
ALTER TABLE product_tasks ADD COLUMN bench jsonb;
ALTER TABLE product_entries ADD COLUMN bench_ms double precision;

-- +goose Down
ALTER TABLE product_entries DROP COLUMN bench_ms;
ALTER TABLE product_tasks DROP COLUMN bench;
