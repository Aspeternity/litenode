package app

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

type result struct {
	code           int
	stdout, stderr string
}

// run executes Run with a fake system. input, if not empty, drives the menu.
func run(t *testing.T, f *systemtest.Fake, euid int, input string, args ...string) result {
	t.Helper()
	return runEnv(t, Env{Runner: f, Euid: euid, Args: args, LockPath: filepath.Join(t.TempDir(), "litenode.lock")}, input)
}

func runEnv(t *testing.T, env Env, input string) result {
	t.Helper()
	var out, errOut strings.Builder
	env.Stdout, env.Stderr = &out, &errOut
	if env.Version == "" {
		env.Version = "dev"
	}
	if env.Executable == "" {
		env.Executable = "/usr/local/bin/litenode"
	}
	if input != "" {
		env.Session = ui.NewSession(strings.NewReader(input), &out, nil, ui.Colors{})
	}
	code := Run(context.Background(), env)
	return result{code, out.String(), errOut.String()}
}

// supported returns a Fake describing a supported Debian 12 system.
func supported() *systemtest.Fake {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte("ID=debian\nVERSION_ID=\"12\"\n")
	return f
}

func TestRunRequiresRoot(t *testing.T) {
	f := systemtest.New()
	r := run(t, f, 1000, "")
	if r.code == 0 || !strings.Contains(r.stderr, text.NotRoot) || r.stdout != "" {
		t.Errorf("%+v", r)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("non-root run touched the system: %v", f.Calls())
	}
}

func TestRunRejectsUnsupportedOS(t *testing.T) {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte("ID=ubuntu\nVERSION_ID=\"20.04\"\n")
	r := run(t, f, 0, "")
	if r.code == 0 || !strings.Contains(r.stderr, text.UnsupportedOS("Ubuntu 20.04")) {
		t.Errorf("%+v", r)
	}
}

func TestRunMissingOSRelease(t *testing.T) {
	if r := run(t, systemtest.New(), 0, ""); r.code == 0 || r.stderr == "" {
		t.Errorf("%+v", r)
	}
}

func TestRunShowsMainMenu(t *testing.T) {
	f := supported()
	f.Files["/proc/meminfo"] = []byte("MemTotal:         996720 kB\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")
	r := run(t, f, 0, "6\n0\n")
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"LiteNode 节点管理  dev", "Debian 12 | KVM | 973M | BBR ✗", text.InDevelopment, " 0. 退出"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, text.DryRunBanner) {
		t.Error("dry-run banner shown in normal mode")
	}
}

func TestRunDryRun(t *testing.T) {
	f := supported()
	r := run(t, f, 0, "0\n", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, text.DryRunBanner) {
		t.Errorf("%+v", r)
	}
	if m := f.Mutations(); len(m) != 0 {
		t.Errorf("dry-run mutated the system: %v", m)
	}
}

func TestRunDryRunRequiresRoot(t *testing.T) {
	if r := run(t, supported(), 1000, "", "--dry-run"); r.code == 0 || !strings.Contains(r.stderr, text.NotRoot) {
		t.Errorf("%+v", r)
	}
}

func TestRunVersion(t *testing.T) {
	for _, arg := range []string{"version", "-v", "--version"} {
		f := systemtest.New()
		// Neither root nor a supported system is needed.
		r := runEnv(t, Env{Runner: f, Euid: 1000, Args: []string{arg}, Version: "v1.2.3"}, "")
		want := "LiteNode v1.2.3 (" + runtime.GOOS + "/" + runtime.GOARCH + ")\n"
		if r.code != 0 || r.stdout != want {
			t.Errorf("%s: %+v, want stdout %q", arg, r, want)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("%s touched the system", arg)
		}
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		r := run(t, systemtest.New(), 1000, "", arg)
		if r.code != 0 || r.stdout != text.Help {
			t.Errorf("%s: %+v", arg, r)
		}
		if strings.Contains(r.stdout, "internal") {
			t.Errorf("%s: help mentions internal subcommands", arg)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	for _, args := range [][]string{{"bogus"}, {"version", "extra"}, {"internal"}, {"internal", "resolve", "x"}, {"internal", "rollback", "sshd"}, {"--dry-run", "x"}} {
		if r := run(t, supported(), 0, "", args...); r.code != 2 || !strings.Contains(r.stderr, "litenode help") {
			t.Errorf("%q: %+v", args, r)
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

	r := runEnv(t, Env{Runner: supported(), Euid: 0, LockPath: lockPath}, "")
	if r.code == 0 || !strings.Contains(r.stderr, text.AlreadyRunning) || r.stdout != "" {
		t.Errorf("%+v", r)
	}
}

func TestRunCorruptState(t *testing.T) {
	f := supported()
	f.Files[state.DefaultPath] = []byte("{not json")
	r := run(t, f, 0, "")
	if r.code == 0 || !strings.Contains(r.stderr, state.DefaultBackupDir) {
		t.Errorf("%+v", r)
	}
	if string(f.Files[state.DefaultPath]) != "{not json" {
		t.Error("corrupt state file must not be overwritten")
	}
}

func TestRunStateTooNew(t *testing.T) {
	f := supported()
	f.Files[state.DefaultPath] = []byte(`{"schema_version": 999}`)
	if r := run(t, f, 0, ""); r.code == 0 || !strings.Contains(r.stderr, text.StateTooNew(state.DefaultPath)) {
		t.Errorf("%+v", r)
	}
}

func TestRunWithoutStateDoesNotWrite(t *testing.T) {
	f := supported()
	if r := run(t, f, 0, ""); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if m := f.Mutations(); len(m) != 0 {
		t.Errorf("first run must not write anything: %v", m)
	}
}

func TestRunInternalRollback(t *testing.T) {
	kinds := change.Registry{}
	if err := kinds.Register(&change.Kind{Name: "sshd"}); err != nil {
		t.Fatal(err)
	}
	// No lock path: rollback must not take the instance lock.
	env := Env{Runner: systemtest.New(), Euid: 0, Args: []string{"internal", "rollback"}, Kinds: kinds}
	if r := runEnv(t, env, ""); r.code != 0 || !strings.Contains(r.stdout, text.RollbackNotNeeded) {
		t.Errorf("%+v", r)
	}
	env.Euid = 1000
	if r := runEnv(t, env, ""); r.code == 0 {
		t.Error("rollback must require root")
	}
}
