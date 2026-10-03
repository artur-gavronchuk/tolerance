-- +goose Up
ALTER TABLE product_entries ADD COLUMN bench_spread_ms double precision;

-- +goose Down
ALTER TABLE product_entries DROP COLUMN bench_spread_ms;
