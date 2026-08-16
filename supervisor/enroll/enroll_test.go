package enroll

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thinre/thinre/protocol"
)

func TestDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != protocol.EnrollPath || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req protocol.EnrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token != "tok" || len(req.Integrations) != 2 {
			t.Errorf("bad request body: %+v (%v)", req, err)
		}
		_ = json.NewEncoder(w).Encode(protocol.EnrollResponse{
			OrganizationID: "org-1",
			Runtimes: []protocol.EnrolledRuntime{
				{IntegrationName: "app-a", RuntimeID: "rt-a", MachineToken: "mt-a"},
				{IntegrationName: "app-b", RuntimeID: "rt-b", MachineToken: "mt-b"},
			},
		})
	}))
	defer srv.Close()

	resp, err := Do(context.Background(), srv.URL, protocol.EnrollRequest{
		Token: "tok",
		Integrations: []protocol.EnrollIntegration{
			{IntegrationName: "app-a", Name: "host/app-a"},
			{IntegrationName: "app-b", Name: "host/app-b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.OrganizationID != "org-1" || len(resp.Runtimes) != 2 || resp.Runtimes[1].MachineToken != "mt-b" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestDoRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid enrollment token"}}`))
	}))
	defer srv.Close()

	_, err := Do(context.Background(), srv.URL, protocol.EnrollRequest{Token: "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid enrollment token") {
		t.Fatalf("expected rejection carrying the server message, got %v", err)
	}
}

func TestDoIncompleteResponse(t *testing.T) {
	// One runtime missing from the answer, one runtime missing its token —
	// both must be rejected as incomplete.
	cases := []string{
		`{"organization_id":"org-1","runtimes":[]}`,
		`{"organization_id":"org-1","runtimes":[{"integration_name":"app-a","runtime_id":"rt-a"}]}`,
	}
	for _, body := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		req := protocol.EnrollRequest{
			Token:        "tok",
			Integrations: []protocol.EnrollIntegration{{IntegrationName: "app-a", Name: "n"}},
		}
		if _, err := Do(context.Background(), srv.URL, req); err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("incomplete response accepted (%s): %v", body, err)
		}
		srv.Close()
	}
}
