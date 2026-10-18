//go:build vps

// Integration test for the test VPS only (M2-01). It changes the real sshd
// configuration, so a systemd timer removes LiteNode's drop-in after three
// minutes in case the test dies halfway. Existing SSH sessions survive.
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o ssh-vps.test ./internal/ssh
//	./ssh-vps.test -test.v -test.run TestVPS      (as root)
package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

const safetyUnit = "litenode-ssh-vps-test"

func vpsSetup(t *testing.T) (context.Context, system.Runner, *state.Store) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	if _, err := os.Stat(ConfigPath); err == nil {
		t.Fatalf("%s already exists; not touching a real LiteNode setup", ConfigPath)
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	r.Exec(ctx, "systemctl", "stop", safetyUnit+".timer")
	r.Exec(ctx, "systemctl", "reset-failed", safetyUnit+".service")
	if _, err := r.Exec(ctx, "systemd-run", "--unit="+safetyUnit, "--on-active=180",
		"/bin/sh", "-c", "rm -f "+ConfigPath+"; systemctl reload "+Unit); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Remove(ConfigPath)
		r.Exec(ctx, "systemctl", "reload", Unit)
		r.Exec(ctx, "systemctl", "stop", safetyUnit+".timer")
	})
	dir := t.TempDir()
	store := &state.Store{Runner: r, Path: filepath.Join(dir, "state.json"), BackupDir: filepath.Join(dir, "backups"), Now: state.NewStore(r).Now}
	return ctx, r, store
}

func vpsApply(ctx context.Context, r system.Runner, store *state.Store, s state.SSH) error {
	st := state.New()
	st.SSH = s
	_, err := change.NewManager(r, store).Apply(ctx, change.Change{
		Parts: []change.Part{{Kind: Kind(store), Files: []change.File{ConfigFile(s)}}},
		State: st,
	})
	return err
}

// TestVPS: with 50-cloud-init.conf saying "PasswordAuthentication yes",
// LiteNode's "no" still takes effect according to sshd -T.
func TestVPS(t *testing.T) {
	ctx, r, store := vpsSetup(t)
	if data, err := os.ReadFile("/etc/ssh/sshd_config.d/50-cloud-init.conf"); err != nil ||
		!regexp.MustCompile(`(?m)^PasswordAuthentication yes`).Match(data) {
		t.Fatalf("expected 50-cloud-init.conf with PasswordAuthentication yes: %v", err)
	}
	before, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: %+v", before)

	off := false
	if err := vpsApply(ctx, r, store, state.SSH{PasswordAuth: &off}); err != nil {
		t.Fatal(err)
	}
	e, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("password off: %+v", e)
	if e.PasswordAuth || e.KbdInteractive {
		t.Errorf("password login still on: %+v", e)
	}

	if err := vpsApply(ctx, r, store, state.SSH{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drop-in not removed: %v", err)
	}
	after, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if after.PasswordAuth != before.PasswordAuth || after.KbdInteractive != before.KbdInteractive {
		t.Errorf("not back to before: %+v", after)
	}
}

// TestVPSPortAccumulates: when the main config has its own Port line
// (Debian test VPS), sshd listens on both ports, so LiteNode's port check
// fails and the change is undone. Skipped where the main config sets no
// port.
func TestVPSPortAccumulates(t *testing.T) {
	main, err := os.ReadFile(MainConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^Port \d+`).Match(main) {
		t.Skip("main config sets no Port")
	}
	ctx, r, store := vpsSetup(t)
	before, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	err = vpsApply(ctx, r, store, state.SSH{Port: 22222})
	t.Logf("apply Port 22222: %v", err)
	if !errors.Is(err, ErrNotEffective) {
		t.Errorf("want ErrNotEffective, got %v", err)
	}
	after, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Ports) != len(before.Ports) || after.Ports[0] != before.Ports[0] {
		t.Errorf("ports %v, want %v", after.Ports, before.Ports)
	}
	if _, err := os.Stat(ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drop-in not removed: %v", err)
	}
}
