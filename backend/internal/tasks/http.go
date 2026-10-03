package tasks

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

// Summary is the public face of a task.
type Summary struct {
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	Language   string  `json:"language"`
	Difficulty int     `json:"difficulty"`
	TaskMD     string  `json:"task_md"`
	Kind       string  `json:"kind"`
	Direction  *string `json:"direction"`
	RepoURL    string  `json:"repo_url"`
}

// Get loads the public summary of an active or past task.
func Get(ctx context.Context, pool *db.Pool, slug string) (Summary, error) {
	var s Summary
	err := pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT slug, title, language, difficulty, task_md, kind, direction FROM tasks WHERE slug = $1`, slug).
			Scan(&s.Slug, &s.Title, &s.Language, &s.Difficulty, &s.TaskMD, &s.Kind, &s.Direction)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, httpx.NotFound()
	}
	s.RepoURL = "/api/v1/tasks/" + s.Slug + "/repo.zip"
	return s, err
}

// TaskFile is the task statement added at the root of the downloaded repo zip.
const TaskFile = "TASK.md"

// RepoZip builds the zip a person downloads: the task's repo files at the zip root, plus TASK.md when the
// repo does not carry its own.
func RepoZip(repoTar []byte, taskMD string) ([]byte, error) {
	files, err := ReadTar(repoTar)
	if err != nil {
		return nil, err
	}
	if _, ok := files[TaskFile]; !ok && taskMD != "" {
		files[TaskFile] = []byte(taskMD)
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RegisterPublicRoutes mounts GET /tasks/{slug}/repo.zip.
func RegisterPublicRoutes(mux *http.ServeMux, pool *db.Pool) {
	mux.HandleFunc("GET /api/v1/tasks/{slug}/repo.zip", func(w http.ResponseWriter, r *http.Request) {
		var tarball []byte
		var taskMD string
		err := pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT repo_tar, task_md FROM tasks WHERE slug = $1`, r.PathValue("slug")).Scan(&tarball, &taskMD)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		if err == nil {
			var z []byte
			if z, err = RepoZip(tarball, taskMD); err == nil {
				w.Header().Set("Content-Type", "application/zip")
				w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("slug")+`.zip"`)
				_, _ = w.Write(z)
				return
			}
		}
		httpx.WriteError(w, r, err)
	})
}
