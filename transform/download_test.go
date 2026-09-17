package transform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadFileStall(t *testing.T) {
	original := downloadStallTimeout
	downloadStallTimeout = 200 * time.Millisecond
	defer func() { downloadStallTimeout = original }()

	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ok.zip") {
			_, _ = w.Write([]byte("complete"))
			return
		}
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-release // keeps the connection open without sending anything else
	}))
	defer func() {
		close(release)
		ts.Close()
	}()
	s := &share{base: ts.URL, token: "token"}
	dir := t.TempDir()

	t.Run("stalled download fails instead of hanging", func(t *testing.T) {
		done := make(chan error, 1)
		go func() {
			done <- s.downloadFile(context.Background(), "stuck.zip", filepath.Join(dir, "stuck.zip"), 0, nil)
		}()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "stalled") {
				t.Errorf("expected a stall error, got %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("expected the stalled download to give up")
		}
		if _, err := os.Stat(filepath.Join(dir, "stuck.zip")); err == nil {
			t.Error("expected no file for a failed download")
		}
	})

	t.Run("healthy download completes", func(t *testing.T) {
		dest := filepath.Join(dir, "ok.zip")
		if err := s.downloadFile(context.Background(), "ok.zip", dest, 0, nil); err != nil {
			t.Fatalf("expected no error, got %s", err)
		}
		b, err := os.ReadFile(dest)
		if err != nil || string(b) != "complete" {
			t.Errorf("expected the file content, got %q (%v)", b, err)
		}
	})
}
