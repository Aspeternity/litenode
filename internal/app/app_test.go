package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

func run(t *testing.T, f *systemtest.Fake, euid int) (code int, stdout, stderr string) {
	t.Helper()
	return runWithLock(t, f, euid, filepath.Join(t.TempDir(), "litenode.lock"))
}

func runWithLock(t *testing.T, f *systemtest.Fake, euid int, lockPath string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut strings.Builder
	store := state.NewStore(f)
	code = Run(context.Background(), Env{
		Runner: f, Euid: euid, Stdout: &out, Stderr: &errOut,
		LockPath: lockPath, Store: store,
	})
	return code, out.String(), errOut.String()
}

// supported returns a Fake describing a supported Debian 12 system.
func supported() *systemtest.Fake {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte("ID=debian\nVERSION_ID=\"12\"\n")
	return f
}

func TestRunRequiresRoot(t *testing.T) {
	f := systemtest.New()
	code, out, errOut := run(t, f, 1000)
	if code == 0 || !strings.Contains(errOut, text.NotRoot) || out != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("non-root run touched the system: %v", f.Calls())
	}
}

func TestRunRejectsUnsupportedOS(t *testing.T) {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte("ID=ubuntu\nVERSION_ID=\"20.04\"\n")
	code, _, errOut := run(t, f, 0)
	if code == 0 || !strings.Contains(errOut, text.UnsupportedOS("Ubuntu 20.04")) {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunMissingOSRelease(t *testing.T) {
	code, _, errOut := run(t, systemtest.New(), 0)
	if code == 0 || errOut == "" {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunShowsMainMenu(t *testing.T) {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte("ID=debian\nVERSION_ID=\"12\"\n")
	f.Files["/proc/meminfo"] = []byte("MemTotal:         996720 kB\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "docker.service enabled enabled\n"}},
		"systemctl", "list-unit-files", "--no-legend", "--plain", "x-ui.service", "xray.service", "docker.service")
	var out, errOut strings.Builder
	session := ui.NewSession(strings.NewReader("1\n0\n"), &out, nil, ui.Colors{})
	code := Run(context.Background(), Env{
		Runner: f, Euid: 0, Stdout: &out, Stderr: &errOut,
		LockPath: filepath.Join(t.TempDir(), "litenode.lock"), Store: state.NewStore(f),
		Version: "dev", Session: session,
	})
	if code != 0 || errOut.String() != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"LiteNode 节点管理  dev", "Debian 12 | KVM | 973M | BBR ✗", "Docker", text.InDevelopment, " 0. 退出"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunSecondInstanceExits(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "litenode.lock")
	first, err := state.AcquireLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	code, out, errOut := runWithLock(t, supported(), 0, lockPath)
	if code == 0 || !strings.Contains(errOut, text.AlreadyRunning) || out != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestRunCorruptState(t *testing.T) {
	f := supported()
	f.Files[state.DefaultPath] = []byte("{not json")
	code, _, errOut := run(t, f, 0)
	if code == 0 || !strings.Contains(errOut, state.DefaultBackupDir) {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
	if string(f.Files[state.DefaultPath]) != "{not json" {
		t.Error("corrupt state file must not be overwritten")
	}
}

func TestRunStateTooNew(t *testing.T) {
	f := supported()
	f.Files[state.DefaultPath] = []byte(`{"schema_version": 999}`)
	code, _, errOut := run(t, f, 0)
	if code == 0 || !strings.Contains(errOut, text.StateTooNew(state.DefaultPath)) {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunWithoutStateDoesNotWrite(t *testing.T) {
	f := supported()
	if code, _, errOut := run(t, f, 0); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if m := f.Mutations(); len(m) != 0 {
		t.Errorf("first run must not write anything: %v", m)
	}
}

func TestRunInternalRollback(t *testing.T) {
	f := systemtest.New()
	store := state.NewStore(f)
	confirmer := change.NewConfirmer(change.NewManager(f, store), "/usr/local/bin/litenode")
	kinds := change.Registry{}
	if err := kinds.Register(&change.Kind{Name: "sshd"}); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	env := Env{
		Runner: f, Euid: 0, Stdout: &out, Stderr: &errOut, Store: store,
		// No lock path: rollback must not take the instance lock.
		Args: []string{"internal", "rollback", "sshd"}, Confirmer: confirmer, Kinds: kinds,
	}
	if code := Run(context.Background(), env); code != 0 || !strings.Contains(out.String(), text.RollbackNotNeeded) {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}

	env.Args = []string{"bogus"}
	if code := Run(context.Background(), env); code == 0 {
		t.Error("unknown command should fail")
	}
}
