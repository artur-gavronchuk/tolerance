package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tolerance/contracts/openapi"
)

func TestConnectorDownload(t *testing.T) {
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "arena-darwin-arm64"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	serve := func(dir string) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/connector/download", connectorDownload(dir))
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return srv
	}
	get := func(srv *httptest.Server, query string) (int, string, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/connector/download?"+query, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		openapi.ValidateResponse(t, router, req, resp, body)
		if resp.StatusCode != 200 {
			var p struct{ Code string }
			_ = json.Unmarshal(body, &p)
			return resp.StatusCode, p.Code, ""
		}
		return resp.StatusCode, string(body), resp.Header.Get("Content-Type")
	}

	srv := serve(dir)
	// What macOS on Apple Silicon prints for `uname -s` and `uname -m`.
	if code, body, ct := get(srv, "os=Darwin&arch=arm64"); code != 200 || body != "binary" || ct != "application/octet-stream" {
		t.Fatalf("darwin/arm64: %d %q %q", code, body, ct)
	}
	// Linux on ARM prints aarch64; that file is not built here.
	if code, reason, _ := get(srv, "os=Linux&arch=aarch64"); code != 404 || reason != "connector_unavailable" {
		t.Fatalf("linux/aarch64 without a file: %d %s", code, reason)
	}
	for _, q := range []string{"os=Windows_NT&arch=x86_64", "os=Darwin&arch=ppc", ""} {
		if code, reason, _ := get(srv, q); code != 404 || reason != "unsupported_platform" {
			t.Fatalf("%q: %d %s", q, code, reason)
		}
	}
	// A server with no prebuilt connectors says so instead of guessing a path.
	if code, reason, _ := get(serve(""), "os=Darwin&arch=arm64"); code != 404 || reason != "connector_unavailable" {
		t.Fatalf("no connector dir: %d %s", code, reason)
	}
}
