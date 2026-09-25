package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tolerance/internal/platform/httpx"
)

// What `uname -s` and `uname -m` print, mapped to the Go targets the
// connector is built for. It runs agents through `sh -c`, so there is no
// Windows build.
var (
	connectorOS   = map[string]string{"darwin": "darwin", "linux": "linux"}
	connectorArch = map[string]string{"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}
)

var errConnectorUnavailable = httpx.New(http.StatusNotFound, "connector_unavailable",
	"This server has no prebuilt connector for that platform; build it from the repository: cd backend && go build -o arena ./cmd/arena")

// connectorDownload serves the prebuilt connector for ?os=&arch=. dir holds
// arena-<goos>-<goarch> files (the api image builds them); an empty dir means
// this server has none, e.g. a native run without `make connector`.
func connectorDownload(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		goos := connectorOS[strings.ToLower(r.URL.Query().Get("os"))]
		goarch := connectorArch[strings.ToLower(r.URL.Query().Get("arch"))]
		if goos == "" || goarch == "" {
			httpx.WriteError(w, r, httpx.New(http.StatusNotFound, "unsupported_platform",
				"The connector is built for macOS and Linux on amd64 and arm64; pass os=$(uname -s)&arch=$(uname -m)"))
			return
		}
		if dir == "" {
			httpx.WriteError(w, r, errConnectorUnavailable)
			return
		}
		f, err := os.Open(filepath.Join(dir, "arena-"+goos+"-"+goarch))
		if err != nil {
			httpx.WriteError(w, r, errConnectorUnavailable)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		// The binary only changes on deploy; let Cloudflare and the browser
		// cache it instead of re-fetching on every `arena connect`.
		// http.ServeContent honors an ETag set on w for If-None-Match/If-Range.
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, st.ModTime().UnixNano(), st.Size()))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="arena"`)
		http.ServeContent(w, r, "arena", st.ModTime(), f)
	}
}
