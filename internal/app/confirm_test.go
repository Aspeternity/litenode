package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ui"
)

// pendingSSHChange turns password login off under confirm-or-rollback and
// returns what awaitConfirm needs.
func pendingSSHChange(t *testing.T) (*systemtest.Fake, *change.Confirmer, change.Registry) {
	t.Helper()
	f := systemtest.New()
	f.Files[ssh.MainConfigPath] = []byte("Include /etc/ssh/sshd_config.d/*.conf\n")
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", ssh.SocketUnit)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "port 22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin yes\n"}}, ssh.SshdPath, "-T")
	store := state.NewStore(f)
	c := change.NewConfirmer(change.NewManager(f, store), "/usr/local/bin/litenode")
	svc := &ssh.Service{Store: store, Confirmer: c}
	off := false
	if err := svc.Change(context.Background(), func(s *state.SSH) { s.PasswordAuth = &off }); err != nil {
		t.Fatal(err)
	}
	reg := change.Registry{}
	for _, k := range svc.Kinds() {
		reg.Register(k)
	}
	return f, c, reg
}

func confirmWith(t *testing.T, c *change.Confirmer, reg change.Registry, input string) (bool, error, string) {
	t.Helper()
	var out bytes.Buffer
	s := ui.NewSession(strings.NewReader(input), &out, nil, ui.Colors{})
	kept, err := awaitConfirm(context.Background(), s, c, reg)
	return kept, err, out.String()
}

func passwordOff(t *testing.T, f *systemtest.Fake) bool {
	t.Helper()
	st, err := state.NewStore(f).Load()
	if err != nil {
		t.Fatal(err)
	}
	_, file := f.Files[ssh.ConfigPath]
	return st.SSH.PasswordAuth != nil && !*st.SSH.PasswordAuth && file
}

func TestAwaitConfirmKeeps(t *testing.T) {
	f, c, reg := pendingSSHChange(t)
	kept, err, out := confirmWith(t, c, reg, "y\n")
	if err != nil || !kept || !strings.Contains(out, "请新开一个终端测试能否登录") || !strings.Contains(out, "60 秒内未确认将自动恢复。 [y/N]: ") || !strings.Contains(out, "已确认，修改已保留。") {
		t.Fatalf("kept=%v err=%v\n%s", kept, err, out)
	}
	if !passwordOff(t, f) {
		t.Error("change not kept")
	}
	if !slices.Contains(f.Execs(), "systemctl stop "+change.TimerUnit+".timer") {
		t.Error("timer not cancelled")
	}
}

func TestAwaitConfirmNoRevertsNow(t *testing.T) {
	f, c, reg := pendingSSHChange(t)
	kept, err, out := confirmWith(t, c, reg, "n\n")
	if err != nil || kept || !strings.Contains(out, "已恢复修改前的配置。") {
		t.Fatalf("kept=%v err=%v\n%s", kept, err, out)
	}
	if passwordOff(t, f) {
		t.Error("change not reverted")
	}
}

func TestAwaitConfirmTooLate(t *testing.T) {
	_, c, reg := pendingSSHChange(t)
	// The timer fires first.
	if done, err := c.Rollback(context.Background(), reg); !done || err != nil {
		t.Fatalf("rollback: %v %v", done, err)
	}
	kept, err, out := confirmWith(t, c, reg, "y\n")
	if err != nil || kept || !strings.Contains(out, "已超时，配置已自动恢复，请重新修改。") {
		t.Fatalf("kept=%v err=%v\n%s", kept, err, out)
	}
}

func TestAwaitConfirmQuitLeavesTimer(t *testing.T) {
	f, c, reg := pendingSSHChange(t)
	kept, err, out := confirmWith(t, c, reg, "")
	if !errors.Is(err, ui.ErrQuit) || kept || !strings.Contains(out, "修改未确认，将在限时到期后自动恢复。") {
		t.Fatalf("kept=%v err=%v\n%s", kept, err, out)
	}
	if _, ok := f.Files[change.DefaultPendingDir+"/pending"]; !ok {
		t.Error("pending marker gone: the timer would not roll back")
	}
}
