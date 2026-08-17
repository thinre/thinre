package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testManifest = `
apiVersion: thinre.io/v1
kind: Integration
metadata:
  name: myapp
package:
  upgrade:
    executable: /opt/myapp/upgrade.sh
    timeout: 60s
health:
  check:
    executable: /opt/myapp/health.sh
    timeout: 10s
`

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "myapp.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeCloud implements just enough of the API for publish: org listing,
// integration listing/creation, and version creation.
type fakeCloud struct {
	orgs         []map[string]string
	integrations []map[string]string
	created      []string // names of integrations created
	published    []map[string]string
}

func (f *fakeCloud) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"unauthorized"}}`))
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_ = json.NewEncoder(w).Encode(f.orgs)
		}
	})
	mux.HandleFunc("GET /api/v1/integrations", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.Header.Get("X-Thinre-Org") == "" {
			t.Error("integration list without organization header")
		}
		_ = json.NewEncoder(w).Encode(f.integrations)
	})
	mux.HandleFunc("POST /api/v1/integrations", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.created = append(f.created, body.Name)
		id := "int-" + body.Name
		f.integrations = append(f.integrations, map[string]string{"id": id, "name": body.Name})
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})
	mux.HandleFunc("POST /api/v1/integrations/{id}/versions", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["integration_id"] = r.PathValue("id")
		f.published = append(f.published, body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "ver-1"})
	})
	return mux
}

func TestPublishExistingIntegration(t *testing.T) {
	cloud := &fakeCloud{
		orgs:         []map[string]string{{"id": "org-1", "name": "acme"}},
		integrations: []map[string]string{{"id": "int-myapp", "name": "myapp"}},
	}
	srv := httptest.NewServer(cloud.handler(t))
	defer srv.Close()

	// No -org: the single organization is picked automatically.
	err := runPublish([]string{"-api", srv.URL, "-token", "test-token", "-version", "2", writeManifest(t, testManifest)})
	if err != nil {
		t.Fatal(err)
	}
	if len(cloud.published) != 1 || cloud.published[0]["version"] != "2" || cloud.published[0]["integration_id"] != "int-myapp" {
		t.Fatalf("unexpected publish: %+v", cloud.published)
	}
	if !strings.Contains(cloud.published[0]["manifest"], "name: myapp") {
		t.Fatalf("manifest content not sent verbatim")
	}
	if len(cloud.created) != 0 {
		t.Fatalf("no integration should have been created: %v", cloud.created)
	}
}

func TestPublishCreatesIntegration(t *testing.T) {
	cloud := &fakeCloud{orgs: []map[string]string{{"id": "org-1", "name": "acme"}}}
	srv := httptest.NewServer(cloud.handler(t))
	defer srv.Close()

	path := writeManifest(t, testManifest)

	// Without -create the unknown integration is an error…
	err := runPublish([]string{"-api", srv.URL, "-token", "test-token", "-version", "1", path})
	if err == nil || !strings.Contains(err.Error(), "-create") {
		t.Fatalf("expected the -create hint, got %v", err)
	}
	// …with it, the integration is created first.
	err = runPublish([]string{"-api", srv.URL, "-token", "test-token", "-version", "1", "-create", path})
	if err != nil {
		t.Fatal(err)
	}
	if len(cloud.created) != 1 || cloud.created[0] != "myapp" || len(cloud.published) != 1 {
		t.Fatalf("unexpected state: created=%v published=%v", cloud.created, cloud.published)
	}
}

func TestPublishAmbiguousOrganization(t *testing.T) {
	cloud := &fakeCloud{orgs: []map[string]string{
		{"id": "org-1", "name": "acme"}, {"id": "org-2", "name": "globex"},
	}}
	srv := httptest.NewServer(cloud.handler(t))
	defer srv.Close()

	err := runPublish([]string{"-api", srv.URL, "-token", "test-token", "-version", "1", writeManifest(t, testManifest)})
	if err == nil || !strings.Contains(err.Error(), "globex") {
		t.Fatalf("expected the organization listing, got %v", err)
	}
}

func TestPublishRejectsInvalidManifestLocally(t *testing.T) {
	// No server at all: a bad manifest must fail before any network call.
	err := runPublish([]string{"-api", "http://127.0.0.1:1", "-token", "t", "-version", "1",
		writeManifest(t, "apiVersion: thinre.io/v1\nkind: Integration\nmetadata:\n  name: NOPE_UPPER\n")})
	if err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("expected a local validation error, got %v", err)
	}
}

func TestPublishSurfacesServerError(t *testing.T) {
	cloud := &fakeCloud{
		orgs:         []map[string]string{{"id": "org-1", "name": "acme"}},
		integrations: []map[string]string{{"id": "int-myapp", "name": "myapp"}},
	}
	mux := http.NewServeMux()
	mux.Handle("/", cloud.handler(t))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/versions") {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"message":"version already exists"}}`))
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()

	err := runPublish([]string{"-api", srv.URL, "-token", "test-token", "-version", "1", writeManifest(t, testManifest)})
	if err == nil || !strings.Contains(err.Error(), "version already exists") {
		t.Fatalf("expected the server message, got %v", err)
	}
}
