package maint

import (
	"context"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/forwardmgr"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/warp"
	"github.com/Aspeternity/litenode/internal/xray"
)

func tuple(rule, comment string) string {
	return "### tuple ### " + rule + " comment=" + hex.EncodeToString([]byte(comment)) + "\n"
}

// installed sets up a machine with a node, a domain forward, ufw rules,
// LiteNode's units and program, and the given firewall setting.
func installed(t *testing.T, firewallOn bool) (*systemtest.Fake, *Service) {
	t.Helper()
	f, svc := newService(t)
	f.Files["/proc/sys/net/ipv6/conf/eth0/accept_ra"] = []byte("1\n")
	f.Files[BinPath] = []byte("litenode")
	f.Files[xray.UnitPath] = []byte("unit")
	f.Files[ssh.ConfigPath] = []byte("Port 22\n")
	f.Files[nft.UnitPath] = []byte("unit")
	f.Files["/etc/systemd/system/"+change.BootUnit] = []byte("unit")
	f.Files["/etc/fstab"] = []byte("/litenode.swap none swap sw 0 0\n")
	f.Files[ufw.UserRules] = []byte(tuple("allow tcp 443 0.0.0.0/0 any 0.0.0.0/0 in", "litenode:node:1") +
		tuple("allow tcp 22 0.0.0.0/0 any 0.0.0.0/0 in", "litenode:ssh") +
		tuple("route:allow tcp 443 198.51.100.7 any 0.0.0.0/0 in", "litenode:forward:3") +
		"### tuple ### allow tcp 80 0.0.0.0/0 any 0.0.0.0/0 in\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "litenode\n"}}, "readlink", LinkPath)
	f.Files[LinkPath] = []byte("link")
	svc.Forwards.Resolver = nil
	st := state.New()
	st.NextID = 10
	st.Xray.Version = "v26.10.17"
	st.SSH.Port = 22
	st.Nodes = []node.Node{realityNode(1, 443, "LA", 2)}
	st.Forwards = []forward.Forward{{ID: 3, Enabled: true, Protocol: "tcp", ListenPort: "30002", TargetHost: "la.example.com", TargetPort: "443", ResolvedIPv4: "198.51.100.7"}}
	st.Firewall = state.Firewall{Enabled: firewallOn, Rules: []state.FirewallRule{{ID: 4, Port: "8080", Protocol: "tcp"}},
		AutoSources: map[string][]string{"ssh": {"192.0.2.1"}, "node:1": {"192.0.2.2"}}}
	if err := svc.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	if err := svc.Forwards.SyncTimer(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	return f, svc
}

func keptKinds(k []Kept) []string {
	var out []string
	for _, x := range k {
		out = append(out, x.What)
	}
	return out
}

func ufwDeletes(f *systemtest.Fake) []string {
	var out []string
	for _, e := range f.Execs() {
		if strings.HasPrefix(e, "ufw delete") || strings.HasPrefix(e, "ufw route delete") {
			out = append(out, e)
		}
	}
	return out
}

func TestUninstallAllFirewallOff(t *testing.T) {
	f, svc := installed(t, false)
	kept, err := svc.UninstallAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for p := range f.Files {
		if strings.HasPrefix(p, state.ConfigDir+"/") || strings.HasPrefix(p, "/usr/local/bin/") || strings.HasPrefix(p, "/usr/local/lib/litenode") ||
			strings.HasPrefix(p, "/etc/systemd/system/litenode") {
			t.Errorf("%s left behind", p)
		}
	}
	if d := ufwDeletes(f); len(d) != 2 || strings.Contains(strings.Join(d, "|"), "port 22") {
		t.Errorf("ufw deletes = %v", d)
	}
	if got := keptKinds(kept); !slices.Equal(got, []string{KeptSSH, KeptUFWSSH, KeptSwap}) {
		t.Errorf("kept = %v", got)
	}
	if string(f.Files[ssh.ConfigPath]) != "Port 22\n" {
		t.Error("SSH config changed")
	}
}

func TestUninstallAllKeepsFirewall(t *testing.T) {
	f, svc := installed(t, true)
	kept, err := svc.UninstallAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	table := string(f.Files[nft.ConfigPath])
	if table == "" || f.Files[nft.UnitPath] == nil {
		t.Fatal("firewall removed")
	}
	if !strings.Contains(table, "8080") || !strings.Contains(table, "22") || strings.Contains(table, "443") || strings.Contains(table, "30002") {
		t.Errorf("table not reduced to the firewall:\n%s", table)
	}
	for p := range f.Files {
		if strings.HasPrefix(p, state.ConfigDir+"/") && p != nft.ConfigPath {
			t.Errorf("%s left behind", p)
		}
	}
	if !slices.Contains(keptKinds(kept), KeptFirewall) {
		t.Errorf("kept = %v", keptKinds(kept))
	}
}

func TestUninstallProgram(t *testing.T) {
	f, svc := installed(t, false)
	before := string(f.Files[xray.ConfigPath])
	kept, err := svc.UninstallProgram(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Files[BinPath] != nil || f.Files[LinkPath] != nil {
		t.Error("program not removed")
	}
	if f.Files[state.DefaultPath] == nil || string(f.Files[xray.ConfigPath]) != before || f.Files[xray.UnitPath] == nil || len(ufwDeletes(f)) != 0 {
		t.Error("state, Xray, table or ufw rules changed")
	}
	unit := string(f.Files[forwardmgr.DefaultUnitDir+"/"+forwardmgr.ResolveService])
	if !strings.Contains(unit, "ConditionPathExists=/usr/local/bin/litenode") {
		t.Errorf("resolve unit:\n%s", unit)
	}
	if len(kept) == 0 {
		t.Error("no kept settings listed")
	}
}

// A foreign lnode is not removed.
func TestUninstallKeepsForeignLink(t *testing.T) {
	f, svc := installed(t, false)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "/opt/other/lnode\n"}}, "readlink", LinkPath)
	if _, err := svc.UninstallProgram(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.Files[LinkPath] == nil {
		t.Error("foreign lnode removed")
	}
}

// DNS written to resolv.conf and WARP are listed as kept; the WARP tunnel
// stays, its check timer goes with the program (D-134).
func TestUninstallAllKeepsDNSAndWARP(t *testing.T) {
	f, svc := installed(t, false)
	f.Files[sysopt.ResolvConfPath] = []byte("# Generated by LiteNode; do not edit.\nnameserver 2606:4700:4700::1111\n")
	f.Files[warp.ConfPath] = []byte("[Interface]\n")
	f.Files["/etc/systemd/system/"+warp.CheckTimer] = []byte("timer")
	f.Files["/etc/systemd/system/"+warp.CheckService] = []byte("service")
	kept, err := svc.UninstallAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Kept{{KeptDNS, sysopt.ResolvConfPath}, {KeptWARP, warp.ConfPath}}
	for _, w := range want {
		if !slices.Contains(kept, w) {
			t.Errorf("kept = %v, missing %v", kept, w)
		}
	}
	if _, ok := f.Files["/etc/systemd/system/"+warp.CheckTimer]; ok {
		t.Error("WARP check timer left behind")
	}
	if _, ok := f.Files[warp.ConfPath]; !ok {
		t.Error("WARP configuration removed")
	}
}
