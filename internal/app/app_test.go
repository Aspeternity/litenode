package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
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

func TestRunShowsStatus(t *testing.T) {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte("ID=debian\nVERSION_ID=\"12\"\n")
	f.Files["/proc/meminfo"] = []byte("MemTotal:         996720 kB\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "docker.service enabled enabled\n"}},
		"systemctl", "list-unit-files", "--no-legend", "--plain", "x-ui.service", "xray.service", "docker.service")
	code, out, errOut := run(t, f, 0)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	for _, want := range []string{"Debian 12 | KVM | 973M | BBR ✗", "Docker"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
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
