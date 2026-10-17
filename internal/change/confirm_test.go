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

const sshdConf = "/etc/ssh/sshd_config.d/00-litenode.conf"

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

// sshPortChange is "change the SSH port": sshd + nft.
func sshPortChange() Change {
	return Change{Parts: []Part{
		{Kind: sshdKind(), Files: []File{{Path: sshdConf, Data: []byte("Port 2222\n"), Mode: 0o644}}},
		nftPart("rules-2222"),
	}}
}

func registry() Registry {
	return Registry{"sshd": sshdKind(), "nft": nftKind()}
}

const armCmd = "systemd-run --unit=litenode-rollback --on-active=60 --timer-property=AccuracySec=1s " +
	"/usr/local/bin/litenode internal rollback"

func TestConfirmArmsBeforeApplying(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	id, err := newConfirmer(fx).Apply(context.Background(), sshPortChange())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"systemctl stop litenode-rollback.timer",
		"systemctl reset-failed litenode-rollback.service",
		armCmd,
		"systemctl reload ssh.service",
		"nft -f /etc/litenode/nftables.nft",
	}
	if got := fx.f.Execs(); !slices.Equal(got, want) {
		t.Errorf("execs =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := strings.TrimSpace(string(fx.f.Files["/run/litenode/pending"])); got != id {
		t.Errorf("pending marker = %q, want backup id %q", got, id)
	}
}

func TestConfirmDisarmCancelsTimer(t *testing.T) {
	fx := newFixture(t)
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshPortChange()); err != nil {
		t.Fatal(err)
	}
	if err := c.Disarm(context.Background()); err != nil {
		t.Fatal(err)
	}
	execs := fx.f.Execs()
	if last := execs[len(execs)-1]; last != "systemctl stop litenode-rollback.timer" {
		t.Errorf("last exec = %q, want timer stop", last)
	}
	if _, ok := fx.f.Files["/run/litenode/pending"]; ok {
		t.Error("pending marker left after confirmation")
	}

	// A timer firing after confirmation must do nothing.
	done, err := c.Rollback(context.Background(), registry())
	if err != nil || done {
		t.Errorf("late rollback = (%v, %v), want (false, nil)", done, err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 2222\n" || string(fx.f.Files[nftPath]) != "rules-2222" {
		t.Error("late rollback reverted a confirmed change")
	}
}

func TestConfirmTimeoutRollsBackAllKinds(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshPortChange()); err != nil {
		t.Fatal(err)
	}

	done, err := c.Rollback(context.Background(), registry())
	if err != nil || !done {
		t.Fatalf("rollback = (%v, %v), want (true, nil)", done, err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 22\n" || string(fx.f.Files[nftPath]) != "old-rules" {
		t.Errorf("not every kind restored: sshd %q nft %q", fx.f.Files[sshdConf], fx.f.Files[nftPath])
	}
	execs := fx.f.Execs()
	tail := execs[len(execs)-2:]
	if want := []string{"systemctl reload ssh.service", "nft -f /etc/litenode/nftables.nft"}; !slices.Equal(tail, want) {
		t.Errorf("old configuration not reapplied in order: %q", tail)
	}

	// The user confirms too late.
	if err := c.Disarm(context.Background()); !errors.Is(err, ErrRolledBack) {
		t.Errorf("late confirm err = %v, want ErrRolledBack", err)
	}
}

func TestRollbackUnknownKindStillRestoresFiles(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshPortChange()); err != nil {
		t.Fatal(err)
	}
	done, err := c.Rollback(context.Background(), Registry{"sshd": sshdKind()}) // nft not registered
	if !done || err == nil || !strings.Contains(err.Error(), `unknown kind "nft"`) {
		t.Errorf("rollback = (%v, %v)", done, err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 22\n" || string(fx.f.Files[nftPath]) != "old-rules" {
		t.Error("files not restored")
	}
}

func TestConfirmFailedApplyCancelsTimer(t *testing.T) {
	fx := newFixture(t)
	fx.f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "nft", "-f", nftPath)
	c := newConfirmer(fx)
	if _, err := c.Apply(context.Background(), sshPortChange()); err == nil {
		t.Fatal("want error")
	}
	if _, ok := fx.f.Files["/run/litenode/pending"]; ok {
		t.Error("pending marker left after failed change")
	}
	execs := fx.f.Execs()
	if last := execs[len(execs)-1]; last != "systemctl stop litenode-rollback.timer" {
		t.Errorf("timer not cancelled, last exec %q", last)
	}
}

func TestConfirmArmFailureRestores(t *testing.T) {
	fx := newFixture(t)
	fx.f.Files[sshdConf] = []byte("Port 22\n")
	fx.f.Respond(systemtest.Response{Err: errors.New("systemd-run missing")},
		"systemd-run", "--unit=litenode-rollback", "--on-active=60", "--timer-property=AccuracySec=1s",
		"/usr/local/bin/litenode", "internal", "rollback")
	_, err := newConfirmer(fx).Apply(context.Background(), sshPortChange())
	var ce *Error
	if !errors.As(err, &ce) || ce.Stage != StageConfirm {
		t.Fatalf("err = %v, want StageConfirm", err)
	}
	if string(fx.f.Files[sshdConf]) != "Port 22\n" {
		t.Error("config not restored when the rollback could not be scheduled")
	}
	if _, ok := fx.f.Files["/run/litenode/pending"]; ok {
		t.Error("pending marker left when the rollback could not be scheduled")
	}
}

func TestRegistryRejectsBadNames(t *testing.T) {
	reg := Registry{}
	for _, name := range []string{"", "ssh-d", "SSHD", "ssh1", "a+b"} {
		if err := reg.Register(&Kind{Name: name}); err == nil {
			t.Errorf("Register(%q) succeeded", name)
		}
	}
	if err := reg.Register(&Kind{Name: "sshd"}); err != nil {
		t.Error(err)
	}
}
