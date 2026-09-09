package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbrahy/AntiVirus/internal/detections"
	"github.com/jbrahy/AntiVirus/internal/hashdb"
	"github.com/jbrahy/AntiVirus/internal/store"
)

type fakeNotifier struct {
	calls []string
}

func (f *fakeNotifier) Notify(title, message string) error {
	f.calls = append(f.calls, title+": "+message)
	return nil
}

func TestFileHandlerEnqueuesAndNotifiesOnMatch(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "avtool.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	badPath := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(badPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	hash, err := hashFileForTest(badPath)
	if err != nil {
		t.Fatalf("hashFileForTest: %v", err)
	}
	if err := hashdb.Upsert(db, []hashdb.Entry{{Hash: hash, Name: "TestVirus", Source: "manual", AddedAt: time.Now()}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	goodPath := filepath.Join(dir, "good.bin")
	if err := os.WriteFile(goodPath, []byte("harmless"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	n := &fakeNotifier{}
	stats := &watchStats{}
	handler := newFileHandler(db, n, os.Stderr, stats)

	handler(badPath)
	handler(goodPath)

	pending, err := detections.ListPending(db)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 || pending[0].Path != badPath {
		t.Fatalf("pending = %+v", pending)
	}
	if len(n.calls) != 1 {
		t.Fatalf("notifier calls = %v, want 1", n.calls)
	}

	scanned, matches := stats.snapshot()
	if scanned != 2 {
		t.Fatalf("scanned = %d, want 2", scanned)
	}
	if matches != 1 {
		t.Fatalf("matches = %d, want 1", matches)
	}
}

func TestRunHeartbeatPrintsPeriodically(t *testing.T) {
	stats := &watchStats{}
	stats.recordScan()
	stats.recordScan()
	stats.recordMatch()

	buf := &bytes.Buffer{}
	stop := make(chan struct{})
	go func() {
		time.Sleep(120 * time.Millisecond)
		close(stop)
	}()

	runHeartbeat(buf, stats, 30*time.Millisecond, stop)

	out := buf.String()
	if !strings.Contains(out, "2 files scanned") {
		t.Fatalf("heartbeat output = %q, want it to mention 2 files scanned", out)
	}
	if !strings.Contains(out, "1 matches") {
		t.Fatalf("heartbeat output = %q, want it to mention 1 matches", out)
	}
	if strings.Count(out, "watching:") < 2 {
		t.Fatalf("heartbeat output = %q, want at least 2 heartbeat lines", out)
	}
}

func TestRunHeartbeatStopsOnStopChannel(t *testing.T) {
	stats := &watchStats{}
	buf := &bytes.Buffer{}
	stop := make(chan struct{})
	close(stop)

	done := make(chan struct{})
	go func() {
		runHeartbeat(buf, stats, time.Hour, stop)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runHeartbeat did not return after stop was closed")
	}
}

func TestWatchQuietFlagRegistered(t *testing.T) {
	flag := watchCmd.Flags().Lookup("quiet")
	if flag == nil {
		t.Fatal("expected a --quiet flag on watch command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--quiet default = %q, want false", flag.DefValue)
	}
}

func TestValidateWatchPaths(t *testing.T) {
	dir := t.TempDir()
	if err := validateWatchPaths([]string{dir}); err != nil {
		t.Fatalf("existing dir: %v", err)
	}
	missing := filepath.Join(dir, "nope")
	if err := validateWatchPaths([]string{missing}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing path error = %v", err)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateWatchPaths([]string{file}); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file path error = %v", err)
	}
}
