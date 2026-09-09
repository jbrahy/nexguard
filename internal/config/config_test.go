package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The concrete directory is platform-specific (os.UserConfigDir reports
// ~/Library/Application Support on macOS, ~/.config on Linux, %AppData% on
// Windows), so derive the expected root the same way the code does and assert
// the invariant that actually matters: every path hangs off <config dir>/avtool
// with the right name.
func TestPathsUnderUserConfigDir(t *testing.T) {
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir: %v", err)
	}
	want := filepath.Join(base, "avtool")

	appDir, err := AppDir()
	if err != nil {
		t.Fatalf("AppDir: %v", err)
	}
	if appDir != want {
		t.Errorf("AppDir = %q, want %q", appDir, want)
	}

	dbPath, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	if dbPath != filepath.Join(want, "avtool.db") {
		t.Errorf("DBPath = %q, want %q", dbPath, filepath.Join(want, "avtool.db"))
	}

	qDir, err := QuarantineDir()
	if err != nil {
		t.Fatalf("QuarantineDir: %v", err)
	}
	if qDir != filepath.Join(want, "quarantine") {
		t.Errorf("QuarantineDir = %q, want %q", qDir, filepath.Join(want, "quarantine"))
	}

	logPath, err := ReportLogPath()
	if err != nil {
		t.Fatalf("ReportLogPath: %v", err)
	}
	if logPath != filepath.Join(want, "detections.log") {
		t.Errorf("ReportLogPath = %q, want %q", logPath, filepath.Join(want, "detections.log"))
	}
}
