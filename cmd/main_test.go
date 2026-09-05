package main

import "testing"

func TestFormatVersionOmitsEmptyBuildMetadata(t *testing.T) {
	got := formatVersion("1.0.0", "", "")
	if got != "1.0.0" {
		t.Fatalf("formatVersion() = %q, want %q", got, "1.0.0")
	}
}

func TestFormatVersionIncludesCommitAndBuildTime(t *testing.T) {
	got := formatVersion("1.0.0", "2bc932bed218", "2026-09-05T08:40:00Z")
	want := "1.0.0 (commitId=2bc932bed218, buildTime=2026-09-05T08:40:00Z)"
	if got != want {
		t.Fatalf("formatVersion() = %q, want %q", got, want)
	}
}
