package maint

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/forwardmgr"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/xray"
)

// newService has Xray installed, ufw inactive, sshd on 22 and a program
// on 30001.
func newService(t *testing.T) (*systemtest.Fake, *Service) {
	t.Helper()
	f := systemtest.New()
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address rem_address   st\n   0: 00000000:0016 00000000:0000 0A\n   1: 00000000:7531 00000000:0000 0A\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Xray 26.10.17 (Xray, Penetrates Everything.)\n"}}, xray.BinPath, "version")
	reg, _ := builtin.Registry()
	store := state.NewStore(f)
	m := change.NewManager(f, store)
	m.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	nodes := &nodemgr.Service{Runner: f, Store: store, Changes: m, Registry: reg,
		Xray: &xray.Installer{Runner: f, Arch: "amd64", Sleep: func(time.Duration) {}}, Rand: rand.Reader}
	fwd := &forwardmgr.Service{Runner: f, Store: store, Changes: m, Executable: "/usr/local/bin/litenode"}
	return f, &Service{Runner: f, Store: store, Nodes: nodes, Forwards: fwd, Confirmer: change.NewConfirmer(m, "/usr/local/bin/litenode"),
		Version: "v0.1.0", Now: func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }}
}

func TestExportFile(t *testing.T) {
	f, svc := newService(t)
	if err := svc.Store.Save(fullState()); err != nil {
		t.Fatal(err)
	}
	path := svc.DefaultExportPath()
	if path != "/root/litenode-export-20261019-120000.json" {
		t.Errorf("default path = %s", path)
	}
	sum, err := svc.Export(path)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Nodes != 1 || f.Modes[path] != ExportFileMode || !strings.Contains(string(f.Files[path]), `"litenode-export"`) {
		t.Errorf("export: %+v mode %o", sum, f.Modes[path])
	}
	if _, err := svc.Read(path); err != nil {
		t.Errorf("read back: %v", err)
	}
}

func TestImport(t *testing.T) {
	f, svc := newService(t)
	f.Files["/proc/sys/net/ipv6/conf/eth0/accept_ra"] = []byte("1\n")
	local := state.New()
	local.NextID = 7
	local.Xray.Version = "v26.10.17"
	local.SSH.Port = 22
	local.Nodes = []node.Node{realityNode(5, 8443, "old", 6)}
	if err := svc.Store.Save(local); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	e := Build(fullState(), "v", time.Now())
	// A program listens on the forward's port 30001: nothing changes.
	_, err := svc.Import(ctx, e)
	var taken *PortsTakenError
	if !errors.As(err, &taken) || len(taken.Claims) != 1 || taken.Claims[0].Port != 30001 {
		t.Fatalf("err = %v, want 30001 taken", err)
	}
	if st, _ := svc.Store.Load(); len(st.Nodes) != 1 || st.Nodes[0].ID != 5 {
		t.Fatal("state changed after a conflict")
	}
	e.Forwards[0].ListenPort = "30002"
	confirm, err := svc.Import(ctx, e)
	if err != nil || confirm {
		t.Fatalf("import: confirm %v, err %v", confirm, err)
	}
	st, _ := svc.Store.Load()
	if len(st.Nodes) != 1 || st.Nodes[0].ID != 1 || len(st.Forwards) != 1 || st.SSH.Port != 22 || st.Firewall.Enabled || st.NextID != 7 {
		t.Errorf("state = %+v", st)
	}
	cfg := string(f.Files[xray.ConfigPath])
	if !strings.Contains(cfg, `"port": 443`) || strings.Contains(cfg, "8443") {
		t.Errorf("xray.json not regenerated:\n%s", cfg)
	}
	if !strings.Contains(string(f.Files["/etc/litenode/nftables.nft"]), "30002") {
		t.Error("forward not in the nftables table")
	}
	if !strings.Contains(string(f.Files["/etc/sysctl.d/99-zz-litenode.conf"]), "ip_forward") {
		t.Error("kernel forwarding not enabled")
	}
}

// With the LiteNode firewall on, an import that changes SSH access waits
// for confirmation.
func TestImportConfirmsSSHChange(t *testing.T) {
	_, svc := newService(t)
	local := state.New()
	local.SSH.Port = 22
	local.Firewall.Enabled = true
	if err := svc.Store.Save(local); err != nil {
		t.Fatal(err)
	}
	src := state.New()
	src.Firewall.Rules = []state.FirewallRule{{ID: 1, Port: "22", Protocol: "tcp", Sources: []string{"192.0.2.1"}}}
	confirm, err := svc.Import(context.Background(), Build(src, "v", time.Now()))
	if err != nil || !confirm {
		t.Fatalf("confirm %v, err %v", confirm, err)
	}
	if err := svc.Confirmer.Disarm(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Removing the rule for 22 changes SSH access again.
	src.Firewall.Rules[0].Port = "8080"
	if confirm, err := svc.Import(context.Background(), Build(src, "v", time.Now())); err != nil || !confirm {
		t.Fatalf("confirm %v, err %v", confirm, err)
	}
	if err := svc.Confirmer.Disarm(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Only other ports: applied directly.
	src.Firewall.Rules[0].Port = "9090"
	if confirm, err := svc.Import(context.Background(), Build(src, "v", time.Now())); err != nil || confirm {
		t.Fatalf("confirm %v, err %v", confirm, err)
	}
}
