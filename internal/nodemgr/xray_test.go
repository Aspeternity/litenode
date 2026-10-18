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
