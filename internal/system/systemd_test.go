package system_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestSystemctlRecordsCommands(t *testing.T) {
	f := systemtest.New()
	ctx := context.Background()
	if err := system.DaemonReload(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err := system.Systemctl(ctx, f, system.ServiceEnable, "litenode-xray.service"); err != nil {
		t.Fatal(err)
	}
	if err := system.Systemctl(ctx, f, system.ServiceRestart, "litenode-xray.service"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"systemctl daemon-reload",
		"systemctl enable litenode-xray.service",
		"systemctl restart litenode-xray.service",
	}
	if got := f.Execs(); !slices.Equal(got, want) {
		t.Errorf("Execs() = %q, want %q", got, want)
	}
}

func TestSystemctlFailure(t *testing.T) {
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1, Stderr: "Unit x.service not found."}},
		"systemctl", "start", "x.service")
	err := system.Systemctl(context.Background(), f, system.ServiceStart, "x.service")
	var ce *system.CommandError
	if !errors.As(err, &ce) || ce.Result.ExitCode != 1 {
		t.Fatalf("err = %v, want CommandError with exit code 1", err)
	}
}

func TestServiceActive(t *testing.T) {
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}},
		"systemctl", "is-active", "--quiet", "stopped.service")
	f.Respond(systemtest.Response{Err: errors.New("executable file not found")},
		"systemctl", "is-active", "--quiet", "broken.service")
	ctx := context.Background()

	if ok, err := system.ServiceActive(ctx, f, "running.service"); err != nil || !ok {
		t.Errorf("running: got (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := system.ServiceActive(ctx, f, "stopped.service"); err != nil || ok {
		t.Errorf("stopped: got (%v, %v), want (false, nil)", ok, err)
	}
	if _, err := system.ServiceActive(ctx, f, "broken.service"); err == nil {
		t.Error("broken: want error when systemctl cannot run")
	}
	if m := f.Mutations(); len(m) != 0 {
		t.Errorf("queries must not mutate, got %v", m)
	}
}

func TestCommandLineQuoting(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"systemctl", "restart", "x.service"}, "systemctl restart x.service"},
		{[]string{"sh", "-c", "echo hi"}, "sh -c 'echo hi'"},
		{[]string{"echo", "it's"}, `echo 'it'\''s'`},
		{[]string{"echo", ""}, "echo ''"},
	}
	for _, tt := range tests {
		if got := system.CommandLine(tt.args[0], tt.args[1:]...); got != tt.want {
			t.Errorf("CommandLine(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}
