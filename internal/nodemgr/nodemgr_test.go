package nodemgr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/xray"
)

const procTCPHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

type fixture struct {
	f   *systemtest.Fake
	svc *Service
}

// newFixture has Xray installed (version answers), ufw active, and a
// program listening on 8080.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := systemtest.New()
	f.Files["/proc/net/tcp"] = []byte(procTCPHeader + "   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Xray 26.3.27 (Xray, Penetrates Everything.)\n"}}, xray.BinPath, "version")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	reg, err := builtin.Registry()
	if err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(f)
	m := change.NewManager(f, store)
	m.Now = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	return &fixture{f: f, svc: &Service{
		Runner: f, Store: store, Changes: m, Registry: reg,
		Xray: &xray.Installer{Runner: f, Arch: "amd64", Sleep: func(time.Duration) {}}, Rand: rand.Reader,
	}}
}

func (fx *fixture) state(t *testing.T) *state.State {
	t.Helper()
	st, err := fx.svc.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (fx *fixture) add(t *testing.T, port int, remark string) node.Node {
	t.Helper()
	n, err := fx.svc.Add(context.Background(), AddInput{PresetID: builtin.PresetVLESSRealityVision, Port: port, Target: "www.microsoft.com:443", Remark: remark})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (fx *fixture) inbounds(t *testing.T) []int {
	t.Helper()
	var cfg struct {
		Inbounds []struct{ Port int } `json:"inbounds"`
	}
	if err := json.Unmarshal(fx.f.Files[xray.ConfigPath], &cfg); err != nil {
		t.Fatal(err)
	}
	var ports []int
	for _, in := range cfg.Inbounds {
		ports = append(ports, in.Port)
	}
	return ports
}

func (fx *fixture) ufwExecs() []string {
	var out []string
	for _, e := range fx.f.Execs() {
		if strings.HasPrefix(e, "ufw ") {
			out = append(out, e)
		}
	}
	return out
}

func TestAdd(t *testing.T) {
	fx := newFixture(t)
	n := fx.add(t, 443, "LA")
	st := fx.state(t)
	if len(st.Nodes) != 1 || st.Nodes[0].ID != n.ID || st.Nodes[0].Clients[0].Remark != "LA" || st.Xray.Version != "v26.3.27" {
		t.Fatalf("state = %+v", st)
	}
	if got := fx.inbounds(t); !slices.Equal(got, []int{443}) {
		t.Errorf("inbounds = %v", got)
	}
	if fx.f.Modes[xray.ConfigPath] != 0o640 || !slices.Contains(fx.f.Execs(), "chgrp litenode-xray "+xray.ConfigPath) {
		t.Error("xray.json not 0640 root:litenode-xray")
	}
	execs := strings.Join(fx.f.Execs(), "\n")
	for _, want := range []string{
		"systemctl enable litenode-xray.service",
		"systemctl restart litenode-xray.service",
		"ufw allow proto tcp to any port 443 comment litenode:node:" + itoa(n.ID),
	} {
		if !strings.Contains(execs, want) {
			t.Errorf("missing %q in\n%s", want, execs)
		}
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func TestAddRejects(t *testing.T) {
	fx := newFixture(t)
	fx.add(t, 443, "LA")
	ctx := context.Background()
	in := func(port int, remark string) AddInput {
		return AddInput{PresetID: builtin.PresetVLESSRealityVision, Port: port, Target: "www.microsoft.com:443", Remark: remark}
	}
	var pt *PortTakenError
	if _, err := fx.svc.Add(ctx, in(443, "Tokyo")); !errors.As(err, &pt) || pt.Conflicts[0].Owner == "" {
		t.Errorf("port of another node: %v", err)
	}
	if _, err := fx.svc.Add(ctx, in(8080, "Tokyo")); !errors.As(err, &pt) || pt.Conflicts[0].Owner != "" {
		t.Errorf("port of a program: %v", err)
	}
	if _, err := fx.svc.Add(ctx, in(2053, "la")); !errors.Is(err, node.ErrRemarkDuplicate) {
		t.Errorf("duplicate remark: %v", err)
	}
	if _, err := fx.svc.Add(ctx, in(2053, "")); !errors.Is(err, node.ErrRemarkEmpty) {
		t.Errorf("empty remark: %v", err)
	}
	if len(fx.state(t).Nodes) != 1 {
		t.Error("rejected adds changed the state")
	}
}

func TestModify(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	a := fx.add(t, 443, "A")
	b := fx.add(t, 2053, "B")

	if err := fx.svc.SetPort(ctx, a.ID, 443); err != nil {
		t.Errorf("keeping its own port: %v", err)
	}
	var pt *PortTakenError
	if err := fx.svc.SetPort(ctx, a.ID, 2053); !errors.As(err, &pt) {
		t.Errorf("taking another node's port: %v", err)
	}
	if err := fx.svc.SetPort(ctx, a.ID, 8443); err != nil {
		t.Fatal(err)
	}
	if got := fx.inbounds(t); !slices.Equal(got, []int{8443, 2053}) {
		t.Errorf("inbounds = %v", got)
	}
	execs := strings.Join(fx.ufwExecs(), "\n")
	if !strings.Contains(execs, "ufw allow proto tcp to any port 8443") {
		t.Errorf("new port not opened in ufw:\n%s", execs)
	}

	old := fx.state(t).Nodes[0].Reality
	if err := fx.svc.RegenerateKeys(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	cur := fx.state(t).Nodes[0].Reality
	if cur.PrivateKey == old.PrivateKey || cur.ShortIDs[0] == old.ShortIDs[0] {
		t.Error("keys not regenerated")
	}
	if err := fx.svc.SetTarget(ctx, a.ID, "www.apple.com:443"); err != nil {
		t.Fatal(err)
	}
	if r := fx.state(t).Nodes[0].Reality; r.Target != "www.apple.com:443" || r.ServerNames[0] != "www.apple.com" {
		t.Errorf("target = %+v", r)
	}
	strict := node.FallbackTiers[2].Limit
	if err := fx.svc.SetFallbackLimit(ctx, a.ID, strict); err != nil || fx.state(t).Nodes[0].Reality.FallbackLimit != strict {
		t.Errorf("fallback limit: %v", err)
	}
	if err := fx.svc.SetSharePort(ctx, a.ID, 30001); err != nil || fx.state(t).Nodes[0].SharePort != 30001 {
		t.Errorf("share port: %v", err)
	}
	if err := fx.svc.SetRemark(ctx, a.ID, "b"); !errors.Is(err, node.ErrRemarkDuplicate) {
		t.Errorf("rename to existing remark: %v", err)
	}
	if err := fx.svc.SetRemark(ctx, b.ID, "Tokyo"); err != nil || fx.state(t).Nodes[1].Remark != "Tokyo" {
		t.Errorf("rename: %v", err)
	}
	if err := fx.svc.SetPort(ctx, 999, 1000); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown node: %v", err)
	}
}

func TestEnableDisableDelete(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	a := fx.add(t, 443, "A")
	// What real ufw wrote for the rule added above.
	fx.f.Files[ufw.UserRules] = []byte("### tuple ### allow tcp 443 0.0.0.0/0 any 0.0.0.0/0 in comment=" +
		hex.EncodeToString([]byte(ufw.NodeComment(a.ID))) + "\n")

	if err := fx.svc.SetEnabled(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := fx.inbounds(t); len(got) != 0 {
		t.Errorf("disabled node still in config: %v", got)
	}
	execs := strings.Join(fx.f.Execs(), "\n")
	if !strings.Contains(execs, "systemctl stop litenode-xray.service") || !strings.Contains(execs, "systemctl disable litenode-xray.service") {
		t.Error("xray not stopped without enabled nodes (D-035)")
	}
	if !strings.Contains(execs, "ufw delete allow proto tcp to any port 443") {
		t.Error("ufw rule of disabled node not removed")
	}

	// Another program took the port meanwhile: enabling is refused.
	fx.f.Files["/proc/net/tcp"] = []byte(procTCPHeader + "   0: 00000000:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0\n")
	var pt *PortTakenError
	if err := fx.svc.SetEnabled(ctx, a.ID, true); !errors.As(err, &pt) {
		t.Errorf("enable onto a taken port: %v", err)
	}
	if err := fx.svc.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if len(fx.state(t).Nodes) != 0 {
		t.Error("node not deleted")
	}
}

func TestManualStopIsKept(t *testing.T) {
	fx := newFixture(t)
	fx.add(t, 443, "A")
	st := fx.state(t)
	st.Xray.Stopped = true
	if err := fx.svc.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	before := len(fx.f.Execs())
	fx.add(t, 2053, "B")
	for _, e := range fx.f.Execs()[before:] {
		if strings.Contains(e, "restart litenode-xray") || strings.Contains(e, "enable litenode-xray") {
			t.Errorf("manually stopped xray was started: %s (D-045)", e)
		}
	}
}

func TestFailedChangeRestores(t *testing.T) {
	fx := newFixture(t)
	fx.add(t, 443, "A")
	// ufw breaks: the whole change (xray config and state) is undone.
	fx.f.Respond(systemtest.Response{Err: errors.New("ufw broke")}, "ufw", "allow", "proto", "tcp", "to", "any", "port", "2053", "comment", "litenode:node:3")
	_, err := fx.svc.Add(context.Background(), AddInput{PresetID: builtin.PresetVLESSRealityVision, Port: 2053, Target: "www.microsoft.com:443", Remark: "B"})
	var ce *change.Error
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want change error", err)
	}
	if len(fx.state(t).Nodes) != 1 || !slices.Equal(fx.inbounds(t), []int{443}) {
		t.Error("failed add not rolled back")
	}
}

func TestShareAddress(t *testing.T) {
	fx := newFixture(t)
	if err := fx.svc.SetShareAddress("la.example.com"); err != nil || fx.state(t).Server.ShareAddress != "la.example.com" {
		t.Errorf("share address: %v", err)
	}
}
