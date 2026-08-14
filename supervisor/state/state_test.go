package state

import "testing"

func TestLoadAbsent(t *testing.T) {
	l, err := Load(t.TempDir())
	if err != nil || l.ObservedVersion != "" || l.InFlight != nil {
		t.Fatalf("absent state should be empty, got %+v (%v)", l, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Local{
		ObservedVersion: "1.2.3",
		LastGeneration:  7,
		InFlight: &Operation{
			Generation:   8,
			Version:      "1.3.0",
			Phase:        "upgrading",
			ArtifactPath: "/var/lib/thinre/artifacts/abc.pkg",
		},
	}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ObservedVersion != want.ObservedVersion || got.LastGeneration != want.LastGeneration {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.InFlight == nil || *got.InFlight != *want.InFlight {
		t.Fatalf("in-flight operation mismatch: %+v", got.InFlight)
	}
}
