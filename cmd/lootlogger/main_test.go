package main

import (
	"testing"
	"time"
)

func TestDefaultCSVPathMatchesUpstreamFormat(t *testing.T) {
	got := defaultCSVPath(time.Date(2026, 6, 17, 14, 47, 50, 0, time.UTC))
	want := "log-2026-06-17-02-47-50utc.csv"
	if got != want {
		t.Fatalf("defaultCSVPath() = %q, want %q", got, want)
	}
}
