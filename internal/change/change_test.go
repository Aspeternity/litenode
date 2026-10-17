package change

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const (
	confPath  = "/etc/litenode/xray.json"
	statePath = "/etc/litenode/state.json"
	backupDir = "/etc/litenode/backups"
)

type fixture struct {
	f     *systemtest.Fake
	m     *Manager
	clock time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fx := &fixture{f: systemtest.New(), clock: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	store := &state.Store{Runner: fx.f, Path: statePath, BackupDir: backupDir, Now: time.Now}
	fx.m = &Manager{Runner: fx.f, Store: store, BackupDir: backupDir, Keep: DefaultKeep,
		Now: func() time.Time { fx.clock = fx.clock.Add(time.Second); return fx.clock }}
	fx.f.Files[confPath] = []byte("old-config")
	fx.f.Files[statePath] = []byte(`{"schema_version": 1, "next_id": 1, "server": {"share_address": "old"}}`)
	return fx
}

func newState(addr string) *state.State {
	st := state.New()
	st.Server.ShareAddress = addr
	return st
}

// testKind validates and applies by running fake commands so tests can
// make either step fail through canned responses.
func testKind(inPlace bool) *Kind {
	return &Kind{
		Name:            "xray",
		ValidateInPlace: inPlace,
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			_, err := r.Query(ctx, "xray", "run", "-test", "-c", paths[confPath])
			return err
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			_, err := r.Exec(ctx, "systemctl", "restart", "litenode-xray.service")
			return err
		},
	}
}

func (fx *fixture) assertOriginal(t *testing.T) {
	t.Helper()
	if got := string(fx.f.Files[confPath]); got != "old-config" {
		t.Errorf("config = %q, want restored old-config", got)
	}
	if got := string(fx.f.Files[statePath]); !strings.Contains(got, `"old"`) {
		t.Errorf("state = %q, want restored original", got)
	}
	for p := range fx.f.Files {
		if strings.Contains(p, stagedMarker) {
			t.Errorf("staged file left behind: %s", p)
		}
	}
}

func TestApplySuccess(t *testing.T) {
	fx := newFixture(t)
	id, err := fx.m.Apply(context.Background(), Change{
		Kind:  testKind(false),
		Files: []File{{Path: confPath, Data: []byte("new-config"), Mode: 0o600}},
		State: newState("new"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(fx.f.Files[confPath]) != "new-config" || !strings.Contains(string(fx.f.Files[statePath]), `"new"`) {
		t.Errorf("change not applied: %q %q", fx.f.Files[confPath], fx.f.Files[statePath])
	}

	// The candidate was validated as a staged copy before replacing the
	// real file, then the service was restarted.
	var cmds []string
	for _, c := range fx.f.Calls() {
		if c.Command != "" {
			cmds = append(cmds, c.Command)
		}
	}
	want := []string{"xray run -test -c /etc/litenode/xray.litenode-new.json", "systemctl restart litenode-xray.service"}
	if !slices.Equal(cmds, want) {
		t.Errorf("commands = %q, want %q", cmds, want)
	}

	b, err := fx.m.LoadBackup(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(fx.f.Files[b.Dir+"/xray.json"]) != "old-config" || !b.StateExisted || !strings.Contains(string(fx.f.Files[b.Dir+"/state.json"]), `"old"`) {
		t.Errorf("backup incomplete: %+v", b)
	}
	if !strings.HasPrefix(id, "xray-20261018-120001") {
		t.Errorf("backup id = %q", id)
	}
}

func TestValidateFailureRestores(t *testing.T) {
	for _, inPlace := range []bool{false, true} {
		t.Run(fmt.Sprintf("inPlace=%v", inPlace), func(t *testing.T) {
			fx := newFixture(t)
			validated := StagedPath(confPath)
			if inPlace {
				validated = confPath
			}
			fx.f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1, Stderr: "bad config"}},
				"xray", "run", "-test", "-c", validated)

			_, err := fx.m.Apply(context.Background(), Change{
				Kind:  testKind(inPlace),
				Files: []File{{Path: confPath, Data: []byte("broken"), Mode: 0o600}},
				State: newState("new"),
			})
			var ce *Error
			if !errors.As(err, &ce) || ce.Stage != StageValidate || ce.RestoreErr != nil {
				t.Fatalf("err = %v, want validate error with clean restore", err)
			}
			fx.assertOriginal(t)
		})
	}
}

func TestApplyFailureRestores(t *testing.T) {
	fx := newFixture(t)
	restart := []string{"systemctl", "restart", "litenode-xray.service"}
	fx.f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, restart[0], restart[1:]...)

	_, err := fx.m.Apply(context.Background(), Change{
		Kind:  testKind(false),
		Files: []File{{Path: confPath, Data: []byte("new-config"), Mode: 0o600}},
		State: newState("new"),
	})
	var ce *Error
	if !errors.As(err, &ce) || ce.Stage != StageApply {
		t.Fatalf("err = %v, want apply error", err)
	}
	fx.assertOriginal(t)
	// Restore reapplies the old configuration: restart runs twice.
	if n := strings.Count(strings.Join(fx.f.Execs(), "\n"), "systemctl restart"); n != 2 {
		t.Errorf("restart ran %d times, want 2 (apply + reapply old)", n)
	}
}

func TestApplyFailureRemovesNewFiles(t *testing.T) {
	fx := newFixture(t)
	delete(fx.f.Files, confPath)
	delete(fx.f.Files, statePath)
	fx.f.WriteErr[statePath] = errors.New("disk full")

	_, err := fx.m.Apply(context.Background(), Change{
		Kind:  testKind(false),
		Files: []File{{Path: confPath, Data: []byte("new-config"), Mode: 0o600}},
		State: newState("new"),
	})
	if err == nil {
		t.Fatal("want error")
	}
	if _, ok := fx.f.Files[confPath]; ok {
		t.Error("file created by the failed change was not removed")
	}
	if _, ok := fx.f.Files[statePath]; ok {
		t.Error("state file created by the failed change was not removed")
	}
}

func TestRemoveFileRestored(t *testing.T) {
	fx := newFixture(t)
	fx.f.Respond(systemtest.Response{Err: errors.New("boom")}, "systemctl", "restart", "litenode-xray.service")
	_, err := fx.m.Apply(context.Background(), Change{
		Kind:  testKind(false),
		Files: []File{{Path: confPath, Remove: true, Mode: 0o600}},
	})
	if err == nil {
		t.Fatal("want error")
	}
	if string(fx.f.Files[confPath]) != "old-config" {
		t.Error("removed file was not restored")
	}
}

func TestBackupKeepsLatestN(t *testing.T) {
	fx := newFixture(t)
	fx.m.Keep = 3
	var ids []string
	for i := range 5 {
		id, err := fx.m.Apply(context.Background(), Change{
			Kind:  testKind(false),
			Files: []File{{Path: confPath, Data: []byte(fmt.Sprint("config-", i)), Mode: 0o600}},
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	// A backup of another kind must not be pruned or counted.
	fx.f.Files[backupDir+"/sshd-20200101-000000.000/manifest.json"] = []byte("{}")
	fx.f.Files[backupDir+"/state-v0-20200101-000000.json"] = []byte("{}")
	if _, err := fx.m.Apply(context.Background(), Change{
		Kind:  testKind(false),
		Files: []File{{Path: confPath, Data: []byte("config-5"), Mode: 0o600}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := fx.m.Backups("xray")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != ids[3] {
		t.Errorf("backups = %v, want the newest 3 starting at %s", got, ids[3])
	}
	for _, old := range ids[:3] {
		for p := range fx.f.Files {
			if strings.Contains(p, old) {
				t.Errorf("pruned backup file still present: %s", p)
			}
		}
	}
	if _, ok := fx.f.Files[backupDir+"/sshd-20200101-000000.000/manifest.json"]; !ok {
		t.Error("backup of another kind was pruned")
	}
}

func TestStagedPathKeepsExtension(t *testing.T) {
	for in, want := range map[string]string{
		"/etc/litenode/xray.json":                 "/etc/litenode/xray.litenode-new.json",
		"/etc/litenode/nftables.nft":              "/etc/litenode/nftables.litenode-new.nft",
		"/etc/ssh/sshd_config.d/00-litenode.conf": "/etc/ssh/sshd_config.d/00-litenode.litenode-new.conf",
		"/etc/litenode/noext":                     "/etc/litenode/noext.litenode-new",
	} {
		if got := StagedPath(in); got != want {
			t.Errorf("StagedPath(%q) = %q, want %q", in, got, want)
		}
	}
}
