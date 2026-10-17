//go:build vps

// Integration test for the test VPS only: drives confirm-or-rollback
// through the real systemd timer and the real litenode binary.
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o change-vps.test ./internal/change
//	LITENODE_BIN=/tmp/litenode ./change-vps.test -test.v -test.run TestVPS   (as root)
package change

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

func TestVPS(t *testing.T) {
	bin := os.Getenv("LITENODE_BIN")
	if os.Geteuid() != 0 || bin == "" {
		t.Skip("run as root on the test VPS with LITENODE_BIN set")
	}
	ctx := context.Background()
	_, statErr := os.Stat(state.ConfigDir)
	createdConfigDir := errors.Is(statErr, os.ErrNotExist)
	dir := t.TempDir()
	t.Cleanup(func() {
		if createdConfigDir {
			os.RemoveAll(state.ConfigDir)
		}
		os.RemoveAll(DefaultPendingDir)
	})

	r := system.NewReal(nil)
	m := NewManager(r, state.NewStore(r))
	c := NewConfirmer(m, bin)
	c.Seconds = 3

	// Two kinds, so the rollback must restore both. The litenode binary
	// has no kinds registered yet: it restores the files and reports that
	// it cannot reapply them, which is enough to check the mechanism.
	a, b := filepath.Join(dir, "a.conf"), filepath.Join(dir, "b.conf")
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(p string) string {
		data, _ := os.ReadFile(p)
		return string(data)
	}
	write(a, "old-a")
	write(b, "old-b")
	change := Change{Parts: []Part{
		{Kind: &Kind{Name: "testa"}, Files: []File{{Path: a, Data: []byte("new-a"), Mode: 0o600}}},
		{Kind: &Kind{Name: "testb"}, Files: []File{{Path: b, Data: []byte("new-b"), Mode: 0o600}}},
	}}

	if _, err := c.Apply(ctx, change); err != nil {
		t.Fatal(err)
	}
	if read(a) != "new-a" || read(b) != "new-b" {
		t.Fatal("change not applied")
	}
	time.Sleep(7 * time.Second)
	if read(a) != "old-a" || read(b) != "old-b" {
		t.Fatalf("timer did not roll back both kinds: a=%q b=%q", read(a), read(b))
	}
	if err := c.Disarm(ctx); !errors.Is(err, ErrRolledBack) {
		t.Fatalf("late confirm = %v, want ErrRolledBack", err)
	}
	t.Log("ok   unconfirmed change rolled back by the systemd timer")

	if _, err := c.Apply(ctx, change); err != nil {
		t.Fatal(err)
	}
	if err := c.Disarm(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(7 * time.Second)
	if read(a) != "new-a" || read(b) != "new-b" {
		t.Fatalf("confirmed change was rolled back: a=%q b=%q", read(a), read(b))
	}
	if active, _ := system.ServiceActive(ctx, r, TimerUnit+".timer"); active {
		t.Fatal("timer still active after confirmation")
	}
	t.Log("ok   confirmed change kept, timer cancelled")
}
