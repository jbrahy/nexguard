package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionCommandPrintsBuildStampedVersion pins the contract the release
// build depends on: `version` is a package var that goreleaser-style
// -ldflags "-X main.version=vX.Y.Z" overwrites, and the version command
// prints whatever it holds. Without this, every released binary reports the
// same string and a bug report can't say which build it came from.
func TestVersionCommandPrintsBuildStampedVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "avtool.db")

	original := version
	t.Cleanup(func() { version = original })
	version = "v9.9.9-test"

	out := runCLI(t, dbPath, "version")
	if !strings.Contains(out, "avtool v9.9.9-test") {
		t.Fatalf("version output = %q, want it to contain the build-stamped version", out)
	}
}

// TestVersionCommandDefaultsToDev covers the unstamped build: `go build`
// with no -ldflags must still print something honest rather than claiming a
// release version it isn't.
func TestVersionCommandDefaultsToDev(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "avtool.db")

	out := runCLI(t, dbPath, "version")
	if !strings.Contains(out, "avtool dev") {
		t.Fatalf("version output = %q, want %q for an unstamped build", out, "avtool dev")
	}
}
