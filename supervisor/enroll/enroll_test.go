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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token != "tok" {
			t.Errorf("bad request body: %+v (%v)", req, err)
		}
		_ = json.NewEncoder(w).Encode(protocol.EnrollResponse{
			RuntimeID:      "rt-1",
			OrganizationID: "org-1",
			MachineToken:   "mt-1",
		})
	}))
	defer srv.Close()

	resp, err := Do(context.Background(), srv.URL, protocol.EnrollRequest{Token: "tok", Name: "n", IntegrationName: "blackbox"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RuntimeID != "rt-1" || resp.MachineToken != "mt-1" {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"runtime_id":"rt-1"}`))
	}))
	defer srv.Close()

	_, err := Do(context.Background(), srv.URL, protocol.EnrollRequest{Token: "tok"})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete response accepted: %v", err)
	}
}
