package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func servers(t *testing.T, payload []byte) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(payload)
	return srv, hex.EncodeToString(sum[:])
}

func TestFetchVerifies(t *testing.T) {
	payload := []byte("artifact-bytes")
	srv, sha := servers(t, payload)
	staging, store := t.TempDir(), t.TempDir()

	path, err := Fetch(context.Background(), srv.URL, sha, staging, store)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("stored artifact mismatch: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), sha) {
		t.Fatalf("artifact not content-addressed: %s", path)
	}

	// A second fetch reuses the verified file without re-downloading.
	srv.Close()
	if _, err := Fetch(context.Background(), srv.URL, sha, staging, store); err != nil {
		t.Fatalf("cached fetch should not hit the network: %v", err)
	}
}

func TestFetchFailsClosedOnMismatch(t *testing.T) {
	srv, _ := servers(t, []byte("tampered-bytes"))
	staging, store := t.TempDir(), t.TempDir()

	_, err := Fetch(context.Background(), srv.URL, strings.Repeat("ab", 32), staging, store)
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("mismatch accepted: %v", err)
	}
	// Nothing may reach the artifact store, and staging must be clean.
	for _, dir := range []string{store, staging} {
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatalf("unverified data left in %s", dir)
		}
	}
}

func TestFetchRejectsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := Fetch(context.Background(), srv.URL, strings.Repeat("ab", 32), t.TempDir(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("404 accepted: %v", err)
	}
}
