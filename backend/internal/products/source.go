package products

import (
	"bytes"
	"context"
	"sort"
	"unicode/utf8"

	"tolerance/internal/submissions"
)

// SourceFile is one text file of a published entry, for reading in the browser.
type SourceFile struct {
	Path      string `json:"path"`
	Size      int    `json:"size"`
	Content   string `json:"content,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}

const (
	maxSourceFiles = 60
	maxSourceFile  = 24 << 10
	maxSourceTotal = 160 << 10
)

// Source lists a published entry's files with the text ones readable (capped), so people can read what they
// are voting on without downloading it.
func (s *Service) Source(ctx context.Context, entryID string) ([]SourceFile, error) {
	data, err := s.Zip(ctx, entryID)
	if err != nil {
		return nil, err
	}
	files, err := submissions.ReadZip(data)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := []SourceFile{}
	total := 0
	for _, p := range paths {
		if len(out) >= maxSourceFiles {
			break
		}
		body := files[p]
		f := SourceFile{Path: p, Size: len(body)}
		switch {
		case !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0:
			f.Binary = true
		case total+min(len(body), maxSourceFile) > maxSourceTotal:
			f.Truncated = true
		default:
			if len(body) > maxSourceFile {
				body, f.Truncated = body[:maxSourceFile], true
				for !utf8.Valid(body) {
					body = body[:len(body)-1]
				}
			}
			f.Content = string(body)
			total += len(body)
		}
		out = append(out, f)
	}
	return out, nil
}
