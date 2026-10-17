package change

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func newConfirmer(fx *fixture) *Confirmer {
	return &Confirmer{Manager: fx.m, Executable: "/usr/local/bin/litenode", PendingDir: "/run/litenode", Seconds: 60}
}

func sshdKind() *Kind {
	return &Kind{
		Name:            "sshd",
		ValidateInPlace: true,
		Validate: func(ctx context.Context, r system.Runner, _ map[string]string) error {
			_, err := r.Query(ctx, "sshd", "-t")
			return err
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			_, err := r.Exec(ctx, "systemctl", "reload", "ssh.service")
			return err
		},
	}
}

const sshdConf = "/etc/ssh/sshd_config.d/00-litenode.conf"

func sshdChange() Change {
	return Change{Kind: sshdKind(), Files: []File{{Path: sshdConf, Data: []byte("Port 2222\n"), Mode: 0o644}}}
}

func TestConfirmArmsBeforeApplying(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	c := newConfirmer(fx)

	id, err := c.Apply(context.Background(), sshdChange())
	if err != nil {
		t.Fatal(err)
	}
	execs := fx.f.Execs()
	want := []string{
		"systemctl stop litenode-rollback-sshd.timer",
		"systemctl reset-failed litenode-rollback-sshd.service",
		"systemd-run --unit=litenode-rollback-sshd --on-active=60 --timer-property=AccuracySec=1s " +
			"/usr/local/bin/litenode internal rollback sshd",
		"systemctl reload ssh.service",
	}
	if !slices.Equal(execs, want) {
		t.Errorf("execs =\n%s\nwant\n%s", strings.Join(execs, "\n"), strings.Join(want, "\n"))
	}
	if got := strings.TrimSpace(string(fx.f.Files["/run/litenode/sshd.pending"])); got != id {
		t.Errorf("pending marker = %q, want backup id %q", got, id)
	}
}

func TestConfirmDisarmCancelsTimer(t *testing.T) {
	fx := newFixture(t)
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshdChange()); err != nil {
		t.Fatal(err)
	}
	if err := c.Disarm(context.Background(), "sshd"); err != nil {
		t.Fatal(err)
	}
	execs := fx.f.Execs()
	if last := execs[len(execs)-1]; last != "systemctl stop litenode-rollback-sshd.timer" {
		t.Errorf("last exec = %q, want timer stop", last)
	}
	if _, ok := fx.f.Files["/run/litenode/sshd.pending"]; ok {
		t.Error("pending marker left after confirmation")
	}
	if string(fx.f.Files[sshdConf]) != "Port 2222\n" {
		t.Error("confirmed change was reverted")
	}

	// A timer firing after confirmation must do nothing.
	done, err := c.Rollback(context.Background(), Registry{"sshd": sshdKind()}, "sshd")
	if err != nil || done {
		t.Errorf("late rollback = (%v, %v), want (false, nil)", done, err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 2222\n" {
		t.Error("late rollback reverted a confirmed change")
	}
}

func TestConfirmTimeoutRollsBack(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshdChange()); err != nil {
		t.Fatal(err)
	}

	done, err := c.Rollback(context.Background(), Registry{"sshd": sshdKind()}, "sshd")
	if err != nil || !done {
		t.Fatalf("rollback = (%v, %v), want (true, nil)", done, err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 22\n" {
		t.Errorf("config = %q, want restored", fx.f.Files[sshdConf])
	}
	if last := fx.f.Execs()[len(fx.f.Execs())-1]; last != "systemctl reload ssh.service" {
		t.Errorf("old config not reapplied, last exec %q", last)
	}

	// The user confirms too late.
	if err := c.Disarm(context.Background(), "sshd"); !errors.Is(err, ErrRolledBack) {
		t.Errorf("late confirm err = %v, want ErrRolledBack", err)
	}
}

func TestConfirmFailedApplyCancelsTimer(t *testing.T) {
	fx := newFixture(t)
	fx.f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "systemctl", "reload", "ssh.service")
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshdChange()); err == nil {
		t.Fatal("want error")
	}
	if _, ok := fx.f.Files["/run/litenode/sshd.pending"]; ok {
		t.Error("pending marker left after failed change")
	}
	execs := fx.f.Execs()
	if last := execs[len(execs)-1]; last != "systemctl stop litenode-rollback-sshd.timer" {
		t.Errorf("timer not cancelled, last exec %q", last)
	}
}

func TestConfirmArmFailureRestores(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	fx.f.Respond(systemtest.Response{Err: errors.New("systemd-run missing")},
		"systemd-run", "--unit=litenode-rollback-sshd", "--on-active=60", "--timer-property=AccuracySec=1s",
		"/usr/local/bin/litenode", "internal", "rollback", "sshd")
	c := newConfirmer(fx)
	_, err := c.Apply(context.Background(), sshdChange())
	var ce *Error
	if !errors.As(err, &ce) || ce.Stage != StageConfirm {
		t.Fatalf("err = %v, want StageConfirm", err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 22\n" {
		t.Error("config not restored when the rollback could not be scheduled")
	}
	if _, ok := fx.f.Files["/run/litenode/sshd.pending"]; ok {
		t.Error("pending marker left when the rollback could not be scheduled")
	}
}

func TestRollbackUnknownKind(t *testing.T) {
	fx := newFixture(t)
	if _, err := newConfirmer(fx).Rollback(context.Background(), Registry{}, "nope"); err == nil {
		t.Error("want error for unknown kind")
	}
}

func TestRegistryRejectsBadNames(t *testing.T) {
	reg := Registry{}
	for _, name := range []string{"", "ssh-d", "SSHD", "ssh1"} {
		if err := reg.Register(&Kind{Name: name}); err == nil {
			t.Errorf("Register(%q) succeeded", name)
		}
	}
	if err := reg.Register(&Kind{Name: "sshd"}); err != nil {
		t.Error(err)
	}
}
