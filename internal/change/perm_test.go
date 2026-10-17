package change

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// TestConfigDirPermissions runs every writer of the config directory
// against the real filesystem with umask 0 and a pre-existing, too open
// backups directory, then checks that nothing but xray.json is readable by
// anyone except the owner.
func TestConfigDirPermissions(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)

	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	if err := os.Mkdir(backups, 0o755); err != nil {
		t.Fatal(err)
	}
	// The test cannot chown to litenode-xray; use a group the current
	// user belongs to instead.
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatal(err)
	}

	r := system.NewReal(nil)
	store := &state.Store{Runner: r, Path: filepath.Join(dir, "state.json"), BackupDir: backups, Now: time.Now}
	clock := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	m := &Manager{Runner: r, Store: store, BackupDir: backups, Keep: 2,
		Now: func() time.Time { clock = clock.Add(time.Second); return clock }}

	// A state file left too open, e.g. restored by hand.
	if err := os.WriteFile(store.Path, []byte(`{"schema_version": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	xrayJSON := filepath.Join(dir, "xray.json")
	nft := filepath.Join(dir, "nftables.nft")
	kind := &Kind{Name: "xray", Validate: func(context.Context, system.Runner, map[string]string) error { return nil }}
	files := []File{
		{Path: xrayJSON, Data: []byte(`{"private_key":"x"}`), Mode: state.GroupFileMode, Group: g.Name},
		{Path: nft, Data: []byte("table inet litenode {}"), Mode: state.PrivateFileMode},
	}
	// Several successful changes (exercising pruning) and a failed one
	// (exercising restore).
	for range 3 {
		if _, err := m.Apply(context.Background(), Change{Kind: kind, Files: files, State: state.New()}); err != nil {
			t.Fatal(err)
		}
	}
	failing := *kind
	failing.Apply = func(context.Context, system.Runner) error { return errors.New("restart failed") }
	if _, err := m.Apply(context.Background(), Change{Kind: &failing, Files: files, State: state.New()}); err == nil {
		t.Fatal("want failure")
	}

	bad, err := state.AuditPermissions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Errorf("readable by others:\n%s", strings.Join(bad, "\n"))
	}
	fi, err := os.Stat(xrayJSON)
	if err != nil || fi.Mode().Perm() != state.GroupFileMode {
		t.Errorf("xray.json mode = %v, want %04o", fi.Mode().Perm(), state.GroupFileMode)
	}
	if st := fi.Sys().(*syscall.Stat_t); strconv.Itoa(int(st.Gid)) != g.Gid {
		t.Errorf("xray.json group = %d, want %s (also after restore)", st.Gid, g.Gid)
	}
	fi, _ = os.Stat(dir)
	if fi.Mode().Perm() != state.ConfigDirMode {
		t.Errorf("config dir mode = %04o, want %04o", fi.Mode().Perm(), state.ConfigDirMode)
	}
}

func TestApplyRejectsLooseModes(t *testing.T) {
	fx := newFixture(t)
	for _, f := range []File{
		{Path: "/etc/litenode/nftables.nft", Mode: 0o644},
		{Path: "/etc/litenode/xray.json", Mode: 0o640},                         // no group
		{Path: "/etc/litenode/xray.json", Mode: 0o644, Group: "litenode-xray"}, // others can read
	} {
		_, err := fx.m.Apply(context.Background(), Change{Kind: testKind(false), Files: []File{f}})
		if err == nil {
			t.Errorf("mode %04o group %q accepted", f.Mode, f.Group)
		}
	}
	if m := fx.f.Mutations(); len(m) != 0 {
		t.Errorf("rejected change touched the system: %v", m)
	}
	// Files outside the config directory keep their own modes.
	_, err := fx.m.Apply(context.Background(), Change{Kind: testKind(false),
		Files: []File{{Path: "/etc/systemd/system/x.service", Data: []byte("x"), Mode: 0o644}}})
	if err != nil {
		t.Errorf("unit file rejected: %v", err)
	}
}

func TestAuditPermissionsDetects(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o711)
	os.WriteFile(filepath.Join(dir, "state.json"), nil, 0o600)
	os.WriteFile(filepath.Join(dir, "xray.json"), nil, 0o640)
	os.Mkdir(filepath.Join(dir, "backups"), 0o700)
	if bad, _ := state.AuditPermissions(dir); len(bad) != 0 {
		t.Fatalf("clean tree reported: %v", bad)
	}
	os.Chmod(filepath.Join(dir, "backups"), 0o755)
	os.WriteFile(filepath.Join(dir, "backups", "xray.json"), nil, 0o640) // only the top-level one may be shared
	os.Chmod(filepath.Join(dir, "xray.json"), 0o644)
	bad, _ := state.AuditPermissions(dir)
	if len(bad) != 3 {
		t.Errorf("got %d violations, want 3: %v", len(bad), bad)
	}
}
