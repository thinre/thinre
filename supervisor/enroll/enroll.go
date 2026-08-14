// Package enroll implements the Supervisor's one-time enrollment exchange:
// an enrollment token goes in, a machine identity comes back. It runs at
// most once per Supervisor installation — a persisted identity makes the
// Supervisor skip enrollment entirely.
package enroll

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/thinre/thinre/protocol"
)

// client bounds the exchange; enrollment is a single small POST.
var client = &http.Client{Timeout: 30 * time.Second}

// Do exchanges the enrollment token at the cloud API. Any non-2xx answer is
// an error carrying the server's explanation when one is provided.
func Do(ctx context.Context, apiURL string, req protocol.EnrollRequest) (*protocol.EnrollResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+protocol.EnrollPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("enroll request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The cloud's uniform error envelope; fall back to the bare
		// status when the body is not in that shape.
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error.Message != "" {
			return nil, fmt.Errorf("enrollment rejected (%d): %s", resp.StatusCode, e.Error.Message)
		}
		return nil, fmt.Errorf("enrollment rejected: status %d", resp.StatusCode)
	}

	var out protocol.EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode enrollment response: %w", err)
	}
	if out.RuntimeID == "" || out.OrganizationID == "" || out.MachineToken == "" {
		return nil, fmt.Errorf("enrollment response is incomplete")
	}
	return &out, nil
}
