package nodemgr

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/xray"
)

func (fx *fixture) execsSince(n int) string {
	return strings.Join(fx.f.Execs()[n:], "\n")
}

func TestStopStartXray(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	fx.add(t, 443, "A")

	before := len(fx.f.Execs())
	if err := fx.svc.StopXray(ctx); err != nil {
		t.Fatal(err)
	}
	if !fx.state(t).Xray.Stopped {
		t.Error("manual stop not recorded")
	}
	if e := fx.execsSince(before); !strings.Contains(e, "systemctl stop litenode-xray.service") || !strings.Contains(e, "systemctl disable litenode-xray.service") {
		t.Errorf("not stopped and disabled:\n%s", e)
	}
	if err := fx.svc.RestartXray(ctx); err == nil {
		t.Error("restart of a manually stopped xray accepted")
	}

	before = len(fx.f.Execs())
	if err := fx.svc.StartXray(ctx); err != nil {
		t.Fatal(err)
	}
	if fx.state(t).Xray.Stopped {
		t.Error("flag not cleared")
	}
	if e := fx.execsSince(before); !strings.Contains(e, "systemctl enable litenode-xray.service") || !strings.Contains(e, "systemctl restart litenode-xray.service") {
		t.Errorf("not started:\n%s", e)
	}
	if err := fx.svc.RestartXray(ctx); err != nil {
		t.Errorf("restart: %v", err)
	}
}

func TestStartXrayWithoutNodes(t *testing.T) {
	fx := newFixture(t)
	if err := fx.svc.StartXray(context.Background()); !errors.Is(err, ErrNoEnabledNodes) {
		t.Errorf("err = %v, want ErrNoEnabledNodes", err)
	}
}

func TestUninstallXray(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	n := fx.add(t, 443, "A")
	fx.f.Files[ufw.UserRules] = []byte("### tuple ### allow tcp 443 0.0.0.0/0 any 0.0.0.0/0 in comment=" + hex.EncodeToString([]byte(ufw.NodeComment(n.ID))) + "\n")
	fx.f.Files[xray.UnitPath] = []byte("unit")

	before := len(fx.f.Execs())
	if err := fx.svc.UninstallXray(ctx); err != nil {
		t.Fatal(err)
	}
	st := fx.state(t)
	if len(st.Nodes) != 0 || st.Xray.Version != "" {
		t.Errorf("state after uninstall: %+v", st)
	}
	e := fx.execsSince(before)
	for _, want := range []string{"ufw delete allow proto tcp to any port 443", "systemctl stop litenode-xray.service", "userdel litenode-xray"} {
		if !strings.Contains(e, want) {
			t.Errorf("missing %q in\n%s", want, e)
		}
	}
	if _, ok := fx.f.Files[xray.ConfigPath]; ok {
		t.Error("xray.json not removed")
	}
}

func TestXrayStatus(t *testing.T) {
	fx := newFixture(t)
	fx.f.Respond(systemtest.Response{Result: system.Result{Stdout: "ActiveState=active\nActiveEnterTimestampMonotonic=1000000\nMemoryCurrent=29360128\n"}},
		"systemctl", "show", xray.UnitName, "-p", "ActiveState", "-p", "ActiveEnterTimestampMonotonic", "-p", "MemoryCurrent")
	fx.f.Files["/proc/uptime"] = []byte("266401.00 500000.00\n") // 3 days 2 hours after start
	st := fx.svc.XrayStatus(context.Background())
	if !st.Installed || st.Version != "v26.3.27" || !st.Active || st.Memory != 28<<20 {
		t.Fatalf("status = %+v", st)
	}
	if want := 3*24*time.Hour + 2*time.Hour; st.Uptime < want || st.Uptime > want+time.Minute {
		t.Errorf("uptime = %v, want about %v", st.Uptime, want)
	}
}

func TestParseProperties(t *testing.T) {
	p := ParseProperties("ActiveState=inactive\nMemoryCurrent=[not set]\n")
	if p["ActiveState"] != "inactive" || p["MemoryCurrent"] != "[not set]" {
		t.Errorf("got %v", p)
	}
}

func TestSetOutboundIP(t *testing.T) {
	fx := newFixture(t)
	fx.add(t, 443, "LA")
	ctx := context.Background()
	if err := fx.svc.SetOutboundIP(ctx, xray.OutboundPreferIPv6); err != nil {
		t.Fatal(err)
	}
	if fx.state(t).Xray.OutboundIP != xray.OutboundPreferIPv6 || !strings.Contains(string(fx.f.Files[xray.ConfigPath]), `"UseIPv6v4"`) {
		t.Errorf("prefer IPv6 not applied:\n%s", fx.f.Files[xray.ConfigPath])
	}
	if err := fx.svc.SetOutboundIP(ctx, xray.OutboundIPv4Only); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fx.f.Files[xray.ConfigPath]), `"ForceIPv4"`) {
		t.Error("only IPv4 not applied")
	}
	if err := fx.svc.SetOutboundIP(ctx, xray.OutboundDefault); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fx.f.Files[xray.ConfigPath]), "domainStrategy\": \"Force") || fx.state(t).Xray.OutboundIP != "" {
		t.Error("default not restored")
	}
}

func TestStartStopXrayAlreadyInState(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	fx.add(t, 443, "A")
	show := []string{"show", xray.UnitName, "-p", "ActiveState", "--value"}

	fx.f.Respond(systemtest.Response{Result: system.Result{Stdout: "active\n"}}, "systemctl", show...)
	before := len(fx.f.Execs())
	if err := fx.svc.StartXray(ctx); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("start while running = %v, want ErrAlreadyRunning", err)
	}
	if e := fx.execsSince(before); e != "" {
		t.Errorf("start while running changed the system:\n%s", e)
	}

	for _, state := range []string{"inactive", "failed"} {
		fx.f.Respond(systemtest.Response{Result: system.Result{Stdout: state + "\n"}}, "systemctl", show...)
		before = len(fx.f.Execs())
		if err := fx.svc.StopXray(ctx); !errors.Is(err, ErrAlreadyStopped) {
			t.Errorf("stop while %s = %v, want ErrAlreadyStopped", state, err)
		}
		if e := fx.execsSince(before); e != "" || fx.state(t).Xray.Stopped {
			t.Errorf("stop while %s changed the system:\n%s", state, e)
		}
	}

	// Stopped by the user: start clears the flag even though systemd may
	// report it active.
	fx.f.Respond(systemtest.Response{Result: system.Result{Stdout: "active\n"}}, "systemctl", show...)
	if err := fx.svc.StopXray(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.StartXray(ctx); err != nil || fx.state(t).Xray.Stopped {
		t.Errorf("start after manual stop = %v, flag %v", err, fx.state(t).Xray.Stopped)
	}
}

func TestXrayLogsLimits(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	fx.svc.XrayLogs(ctx, 5000)
	fx.svc.XrayLogs(ctx, 0)
	var out strings.Builder
	fx.f.Respond(systemtest.Response{Result: system.Result{Stdout: "line\n"}}, "journalctl", "-u", xray.UnitName, "-f", "-n", "20", "--no-pager", "-o", "short-iso")
	if err := fx.svc.FollowXrayLogs(ctx, &out); err != nil || out.String() != "line\n" {
		t.Errorf("follow = %q, %v", out.String(), err)
	}
	var queries []string
	for _, c := range fx.f.Calls() {
		if c.Op == systemtest.OpQuery {
			queries = append(queries, c.Command)
		}
	}
	calls := strings.Join(queries, "\n")
	for _, want := range []string{"journalctl -u litenode-xray.service -n 1000 ", "journalctl -u litenode-xray.service -n 1 ", "journalctl -u litenode-xray.service -f -n 20"} {
		if !strings.Contains(calls, want) {
			t.Errorf("missing %q in\n%s", want, calls)
		}
	}
}

func TestApplyWithoutXrayProgram(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	a := fx.add(t, 443, "A")
	b := fx.add(t, 8443, "B")
	// The program was deleted by hand (D-095).
	fx.f.Respond(systemtest.Response{Err: errors.New("no such file")}, xray.BinPath, "version")

	// B still needs Xray: deleting A cannot be checked and is refused.
	if err := fx.svc.Delete(ctx, a.ID); !errors.Is(err, ErrXrayMissing) {
		t.Fatalf("delete with a node left = %v, want ErrXrayMissing", err)
	}
	if len(fx.state(t).Nodes) != 2 {
		t.Error("refused delete changed the state")
	}
	// Stopped by the user nothing needs Xray: deleting A goes ahead
	// without "xray run -test".
	st := fx.state(t)
	st.Xray.Stopped = true
	if err := fx.svc.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	before := len(fx.f.Calls())
	if err := fx.svc.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete = %v", err)
	}
	for _, c := range fx.f.Calls()[before:] {
		if strings.Contains(c.Command, "run -test") {
			t.Errorf("validated without the program: %s", c.Command)
		}
	}
	// Without manual stop, deleting the last node needs no Xray either.
	st = fx.state(t)
	st.Xray.Stopped = false
	fx.svc.Store.Save(st)
	if err := fx.svc.Delete(ctx, b.ID); err != nil {
		t.Fatalf("delete last = %v", err)
	}
	if st := fx.state(t); len(st.Nodes) != 0 {
		t.Errorf("state = %+v", st.Nodes)
	}
}
