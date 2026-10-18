package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// TestOperationsHold: adding a node (link and QR code) and exporting
// (summary and warning) wait for Enter in a terminal (D-096).
func TestOperationsHold(t *testing.T) {
	f := nodeSystem()
	input := strings.Join([]string{
		"1", "1", "1", "", "3", "www.example.com", "", "", "0", // add node, Enter, back
		"9", "1", "/root/x.json", "", "0", "0", // export, Enter, back, quit
	}, "\n") + "\n"
	var out strings.Builder
	s := ui.NewSession(strings.NewReader(input), &out, nil, ui.Colors{})
	s.Interactive = true
	code := Run(context.Background(), Env{
		Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
		TargetChecker: fakeChecker{}, AddressDetector: fakeDetector{},
		Stdout: &out, Stderr: &out, Version: "dev", Executable: "/usr/local/bin/litenode",
		Session: s,
	})
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out.String())
	}
	if got := strings.Count(out.String(), text.PressEnter); got != 2 {
		t.Errorf("pauses = %d, want 2:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "/root/x.json") || f.Modes["/root/x.json"] == 0 {
		t.Errorf("export did not run:\n%s", out.String())
	}
}
