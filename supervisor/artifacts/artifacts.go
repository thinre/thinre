// Package artifacts downloads and verifies release packages. The rule is
// fail-closed: nothing that has not passed SHA-256 verification ever
// reaches the artifacts directory, and nothing unverified is ever handed
// to a lifecycle hook (RT-ART-006, RT-SEC-003).
package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxSize bounds a package download; a runaway or malicious source must
// not fill the disk (RT-SUP-006).
const MaxSize = 1 << 30 // 1 GiB

var client = &http.Client{Timeout: 15 * time.Minute}

// fetchAttempts and fetchBackoff bound the transient-failure retries
// (architecture §21: retry with backoff, preserve the running version).
const fetchAttempts = 3

var fetchBackoff = 2 * time.Second

// errVerification marks a hash mismatch: retrying cannot help, because the
// source's content is what it is.
var errVerification = fmt.Errorf("verification failed")

// Fetch downloads url into stagingDir, verifies the SHA-256, and moves the
// verified file into artifactsDir under a content-addressed name. It
// returns the final path. Transient failures are retried with backoff;
// verification failures are terminal. On any failure the partial download
// is removed.
func Fetch(ctx context.Context, url, wantSHA256, stagingDir, artifactsDir string) (string, error) {
	var err error
	var path string
	for attempt := 1; attempt <= fetchAttempts; attempt++ {
		path, err = fetchOnce(ctx, url, wantSHA256, stagingDir, artifactsDir)
		if err == nil || errors.Is(err, errVerification) || ctx.Err() != nil {
			return path, err
		}
		select {
		case <-time.After(time.Duration(attempt) * fetchBackoff):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("after %d attempts: %w", fetchAttempts, err)
}

func fetchOnce(ctx context.Context, url, wantSHA256, stagingDir, artifactsDir string) (string, error) {
	want := strings.ToLower(wantSHA256)

	// Content-addressed: if this exact artifact was already verified, a
	// re-download is pointless (and a crashed run can resume for free).
	final := filepath.Join(artifactsDir, want+".pkg")
	if _, err := os.Stat(final); err == nil {
		return final, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: status %d from artifact source", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(stagingDir, "download-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name()) // no-op after successful rename
	}()

	// Hash while streaming; LimitReader enforces the size bound. Reading
	// one byte beyond MaxSize means the source is oversized.
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	if n > MaxSize {
		return "", fmt.Errorf("download: artifact exceeds the %d-byte limit", int64(MaxSize))
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if got != want {
		return "", fmt.Errorf("%w: sha256 %s does not match expected %s", errVerification, got, want)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", fmt.Errorf("store verified artifact: %w", err)
	}
	return final, nil
}
