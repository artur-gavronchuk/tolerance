package builds

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/platform/idgen"
)

// The platform's own user, the author of every reference entry. Like the house agents it has no identity
// and cannot sign in.
const (
	referenceUserID = "user_house_reference"
	referenceHandle = "reference"
)

// SeedReferences makes each visible challenge's reference solution (LoadTests found it in the private
// catalog) the platform's entry: inserted once, re-queued when its files change. Run by cmd/api on start.
func (s *Service) SeedReferences(ctx context.Context) (int, error) {
	n := 0
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, email, handle, role, house, house_name) VALUES ($1, $2, $3, 'user', true, 'tolerance')
			ON CONFLICT (id) DO NOTHING`, referenceUserID, "house+reference@tolerance.invalid", referenceHandle)
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return fmt.Errorf("builds: handle %q is taken by another user", referenceHandle)
		}
		if err != nil {
			return err
		}
		for _, c := range visible() {
			if c.Reference == "" {
				continue
			}
			files, err := readDir(c.Reference)
			if err != nil {
				return fmt.Errorf("builds: %s reference: %w", c.Slug, err)
			}
			data, err := zipFiles(files)
			if err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `
				INSERT INTO build_entries (id, challenge, user_id, zip, made_with, digest, reference) VALUES ($1, $2, $3, $4, 'tolerance', $5, true)
				ON CONFLICT (challenge, user_id) DO UPDATE SET zip = EXCLUDED.zip, digest = EXCLUDED.digest, reference = true,
					status = 'queued', score = NULL, checks = '{}'::jsonb, shot = NULL, log_tail = '', failure_reason = NULL,
					uploads = build_entries.uploads + 1, updated_at = now(), finished_at = NULL
				WHERE build_entries.digest <> EXCLUDED.digest`, idgen.New("bld"), c.Slug, referenceUserID, data, digest(files))
			if err != nil {
				return err
			}
			n += int(tag.RowsAffected())
		}
		return nil
	})
	return n, err
}

func readDir(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = body
		return nil
	})
	return files, err
}
