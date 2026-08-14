package identity

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLoadAbsent(t *testing.T) {
	id, err := Load(t.TempDir())
	if err != nil || id != nil {
		t.Fatalf("absent identity should be (nil, nil), got (%v, %v)", id, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Identity{
		RuntimeID:      "rt-1",
		OrganizationID: "org-1",
		MachineToken:   "secret-token",
		EnrolledAt:     time.Now().UTC().Truncate(time.Second),
	}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if *got != want {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}

	// The credential file must not be world-readable. File modes are not
	// meaningful on Windows, so assert only where they are.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, fileName))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("identity file mode = %o, want 600", perm)
		}
	}

	// No temp file may be left behind after a successful save.
	if _, err := os.Stat(filepath.Join(dir, fileName+".tmp")); !os.IsNotExist(err) {
		t.Fatal("temporary identity file left behind")
	}
}

func TestLoadIncomplete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"runtime_id":"rt-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("incomplete identity accepted")
	}
}
