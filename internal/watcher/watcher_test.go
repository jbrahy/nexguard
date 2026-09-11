// internal/watcher/watcher_test.go
package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatchReturnsErrorWhenAllPathsFail(t *testing.T) {
	nonexistent := []string{
		filepath.Join(t.TempDir(), "does-not-exist-1"),
		filepath.Join(t.TempDir(), "does-not-exist-2"),
	}

	stop := make(chan struct{})
	defer close(stop)

	done := make(chan error, 1)
	go func() {
		done <- Watch(nonexistent, func(path string) {}, stop)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error when all watched paths fail to add, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return promptly when all given paths were nonexistent")
	}
}

func TestWatchDebouncesMultipleWrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.bin")

	var mu sync.Mutex
	var calls []string
	stop := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- Watch([]string{dir}, func(path string) {
			mu.Lock()
			calls = append(calls, path)
			mu.Unlock()
		}, stop)
	}()

	time.Sleep(50 * time.Millisecond) // let the watcher register

	for i := 0; i < 3; i++ {
		if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond) // let debounce settle
	close(stop)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after stop was closed")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("got %d onFile calls, want 1 (debounced): %v", len(calls), calls)
	}
	if calls[0] != target {
		t.Fatalf("call path = %q, want %q", calls[0], target)
	}
}

// TestWatchToleratesSymlinkCycle covers a watched tree containing a
// self-referential symlink: dir/sub/up points back at dir, so anything that
// walks the tree following symlinks recurses forever. Watch currently calls
// fsnotify's Add once per given path and never walks, so no cycle can reach
// it; this test pins that down, and fails by timing out rather than hanging
// the suite if a recursive walk is ever added without cycle detection.
func TestWatchToleratesSymlinkCycle(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Symlink(dir, filepath.Join(sub, "up")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	target := filepath.Join(dir, "file.bin")

	var mu sync.Mutex
	var calls []string
	stop := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- Watch([]string{dir}, func(path string) {
			mu.Lock()
			calls = append(calls, path)
			mu.Unlock()
		}, stop)
	}()

	time.Sleep(50 * time.Millisecond) // let the watcher register

	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// The watcher must still be serving events with the cycle present, and
	// must have reached its event loop at all: a walk that recursed into the
	// cycle would never get here.
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		n := len(calls)
		mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned before reporting the write: %v", err)
		case <-deadline:
			t.Fatal("Watch did not report a write within 5s with a symlink cycle in the tree")
		case <-time.After(25 * time.Millisecond):
		}
	}

	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after stop was closed")
	}
}
