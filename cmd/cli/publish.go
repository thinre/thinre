package main

// publish sends a locally authored integration manifest to Thinre Cloud as
// a new integration version. The host copy stays the source of truth —
// publishing flows host → cloud, never the reverse, so the cloud can
// validate releases and bundles against the same contract the supervisor
// enforces without ever being able to change what runs on the machine.

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	integrationspec "github.com/thinre/thinre/integration-spec"
)

const publishUsage = `usage: thinre publish [flags] <manifest.yaml>

Publishes a local integration manifest to Thinre Cloud as a new version
of the integration named in the manifest's metadata.name.

flags:
  -api URL       cloud API base URL          (env THINRE_API_URL)
  -token TOKEN   bearer token                (env THINRE_TOKEN)
  -org ID        organization id; optional when the user belongs to
                 exactly one organization    (env THINRE_ORG)
  -version V     version to publish, e.g. 2 (required)
  -create        create the integration first if it does not exist yet
`

func runPublish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, publishUsage) }
	api := fs.String("api", os.Getenv("THINRE_API_URL"), "")
	token := fs.String("token", os.Getenv("THINRE_TOKEN"), "")
	org := fs.String("org", os.Getenv("THINRE_ORG"), "")
	version := fs.String("version", "", "")
	create := fs.Bool("create", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("exactly one manifest path is required")
	}
	switch {
	case *api == "":
		return fmt.Errorf("the cloud API URL is required (-api or THINRE_API_URL)")
	case *token == "":
		return fmt.Errorf("a token is required (-token or THINRE_TOKEN)")
	case *version == "":
		return fmt.Errorf("-version is required")
	}

	// Validate locally before any network call — the same parser the
	// cloud and the supervisor use, so a rejection here is identical to
	// a rejection there.
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := integrationspec.Parse(data)
	if err != nil {
		return err
	}
	name := manifest.Metadata.Name

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := &apiClient{base: strings.TrimRight(*api, "/"), token: *token, org: *org}

	if c.org == "" {
		if c.org, err = c.soleOrganization(ctx); err != nil {
			return err
		}
	}

	integrationID, err := c.findIntegration(ctx, name)
	if err != nil {
		return err
	}
	if integrationID == "" {
		if !*create {
			return fmt.Errorf("integration %q does not exist in the organization; pass -create to create it", name)
		}
		if integrationID, err = c.createIntegration(ctx, name); err != nil {
			return err
		}
		fmt.Printf("created integration %s (%s)\n", name, integrationID)
	}

	body, _ := json.Marshal(map[string]string{"version": *version, "manifest": string(data)})
	if err := c.do(ctx, http.MethodPost, "/api/v1/integrations/"+integrationID+"/versions", body, nil); err != nil {
		return err
	}
	fmt.Printf("published %s version %s\n", name, *version)
	return nil
}

// apiClient is the minimal authenticated HTTP surface publish needs.
type apiClient struct {
	base  string
	token string
	org   string
}

// soleOrganization resolves the organization when none was given: it is
// only unambiguous for users who belong to exactly one.
func (c *apiClient) soleOrganization(ctx context.Context) (string, error) {
	var orgs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/organizations", nil, &orgs); err != nil {
		return "", err
	}
	switch len(orgs) {
	case 0:
		return "", fmt.Errorf("the user belongs to no organization")
	case 1:
		return orgs[0].ID, nil
	}
	names := make([]string, len(orgs))
	for i, o := range orgs {
		names[i] = fmt.Sprintf("%s (%s)", o.Name, o.ID)
	}
	return "", fmt.Errorf("the user belongs to several organizations; pass -org: %s", strings.Join(names, ", "))
}

func (c *apiClient) findIntegration(ctx context.Context, name string) (string, error) {
	var integrations []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/integrations", nil, &integrations); err != nil {
		return "", err
	}
	for _, i := range integrations {
		if i.Name == name {
			return i.ID, nil
		}
	}
	return "", nil
}

func (c *apiClient) createIntegration(ctx context.Context, name string) (string, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	var created struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/integrations", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// do performs one API call; non-2xx answers surface the server's error
// envelope message when present.
func (c *apiClient) do(ctx context.Context, method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.org != "" {
		req.Header.Set("X-Thinre-Org", c.org)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error.Message != "" {
			return fmt.Errorf("%s %s: %s (%d)", method, path, e.Error.Message, resp.StatusCode)
		}
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
