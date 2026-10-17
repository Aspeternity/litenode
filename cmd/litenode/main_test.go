package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVersionInjection builds the real binary with and without -ldflags and
// runs "version", which needs neither root nor a supported system.
func TestVersionInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	dir := t.TempDir()
	platform := runtime.GOOS + "/" + runtime.GOARCH
	for _, tt := range []struct {
		name, ldflags, want string
	}{
		{"injected", "-X main.version=v0.0.1-test", "LiteNode v0.0.1-test (" + platform + ")"},
		{"default", "", "LiteNode dev (" + platform + ")"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bin := filepath.Join(dir, tt.name)
			build := exec.Command("go", "build", "-ldflags", tt.ldflags, "-o", bin, ".")
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, out)
			}
			out, err := exec.Command(bin, "version").Output()
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(string(out)); got != tt.want {
				t.Errorf("version = %q, want %q", got, tt.want)
			}
		})
	}
}
