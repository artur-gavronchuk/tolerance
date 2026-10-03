-- +goose Up
-- Site entries: objective signals collected in the scoring run (accessibility, load performance, mobile fit).
-- Informational only; never part of the ranking.
ALTER TABLE product_entries ADD COLUMN quality jsonb;

-- +goose Down
ALTER TABLE product_entries DROP COLUMN quality;
