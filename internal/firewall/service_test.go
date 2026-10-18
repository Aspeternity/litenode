package firewall

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// ufwSSHRule is a rule LiteNode added to ufw (comment "litenode:ssh").
const ufwSSHRule = "### tuple ### allow tcp 22 0.0.0.0/0 any 0.0.0.0/0 in comment=6c6974656e6f64653a737368\n"

func service(t *testing.T) (*systemtest.Fake, *Service) {
	t.Helper()
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "port 22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"}}, ssh.SshdPath, "-T")
	no := systemtest.Response{Result: system.Result{ExitCode: 3}}
	f.Respond(no, "systemctl", "is-active", "--quiet", FirewalldUnit)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "systemctl", "cat", DockerUnit)
	store := state.NewStore(f)
	st := state.New()
	st.Nodes = []node.Node{{ID: 3, Port: 443, Enabled: true}}
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	return f, &Service{Store: store, Confirmer: change.NewConfirmer(change.NewManager(f, store), "/usr/local/bin/litenode")}
}

func hasExec(f *systemtest.Fake, prefix string) bool {
	return slices.ContainsFunc(f.Execs(), func(e string) bool { return strings.HasPrefix(e, prefix) })
}

func TestEnable(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.Files[ufw.UserRules] = []byte(ufwSSHRule)
	if p := s.Check(ctx); p.UFW || p.Firewalld || p.Docker || p.NftMissing || p.Foreign.Found() {
		t.Errorf("Check = %+v", p)
	}
	if err := s.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	conf := string(f.Files[nft.ConfigPath])
	for _, want := range []string{"policy drop", `tcp dport 22 accept comment "litenode:ssh"`, `tcp dport 443 accept comment "litenode:node:3"`} {
		if !strings.Contains(conf, want) {
			t.Errorf("rules lack %q:\n%s", want, conf)
		}
	}
	if !hasExec(f, "systemd-run") {
		t.Error("no rollback timer: enabling must be confirmed")
	}
	if !hasExec(f, "ufw delete allow") {
		t.Errorf("LiteNode's rule left in the stopped ufw: %v", f.Execs())
	}
	if st, _ := s.Store.Load(); !st.Firewall.Enabled {
		t.Error("state not enabled")
	}
	if err := s.Enable(ctx); !errors.Is(err, ErrEnabled) {
		t.Errorf("second Enable: %v", err)
	}
}

func TestEnableRefusesOtherFirewalls(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	if err := s.Enable(ctx); !errors.Is(err, ErrUFWActive) {
		t.Errorf("ufw active: %v", err)
	}
	f, s = service(t)
	f.Respond(systemtest.Response{}, "systemctl", "is-active", "--quiet", FirewalldUnit)
	before := len(f.Mutations())
	if err := s.Enable(ctx); !errors.Is(err, ErrFirewalldActive) {
		t.Errorf("firewalld active: %v", err)
	}
	if len(f.Mutations()) != before {
		t.Errorf("refused enable changed the system: %v", f.Mutations()[before:])
	}
}

func TestEnableInstallsNft(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 127}}, nft.NftPath, "--version")
	if p := s.Check(ctx); !p.NftMissing {
		t.Error("missing nft not reported")
	}
	if err := s.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	if !hasExec(f, "env DEBIAN_FRONTEND=noninteractive apt-get install -y nftables") {
		t.Errorf("nftables not installed: %v", f.Execs())
	}
}

func TestDisableAndReset(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	if err := s.Disable(ctx); !errors.Is(err, ErrDisabled) {
		t.Errorf("Disable while off: %v", err)
	}
	if err := s.Reset(ctx); !errors.Is(err, ErrDisabled) {
		t.Errorf("Reset while off: %v", err)
	}
	st, _ := s.Store.Load()
	st.Firewall = state.Firewall{Enabled: true, BlockPing: true,
		Rules:       []state.FirewallRule{{ID: 1, Port: "8080", Protocol: "tcp"}},
		AutoSources: map[string][]string{"node:3": {"1.2.3.4"}}}
	s.Store.Save(st)

	if err := s.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(f.Files[nft.ConfigPath]), "chain input") {
		t.Errorf("disabled firewall still filters:\n%s", f.Files[nft.ConfigPath])
	}
	st, _ = s.Store.Load()
	if st.Firewall.Enabled || len(st.Firewall.Rules) != 1 {
		t.Errorf("after Disable: %+v (the user's rules stay for next time)", st.Firewall)
	}

	st.Firewall.Enabled = true
	s.Store.Save(st)
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Store.Load()
	if fw := st.Firewall; !fw.Enabled || fw.BlockPing || len(fw.Rules) != 0 || len(fw.AutoSources) != 0 {
		t.Errorf("after Reset: %+v", fw)
	}
	if !strings.Contains(string(f.Files[nft.ConfigPath]), `comment "litenode:node:3"`) {
		t.Error("reset lost the automatic rules")
	}
}

func TestEntries(t *testing.T) {
	st := state.New()
	st.Nodes = []node.Node{{ID: 7, Port: 8443}, {ID: 3, Port: 443, Enabled: true}}
	st.Firewall.Rules = []state.FirewallRule{{ID: 2, Port: "53", Protocol: "udp", Note: "dns"}}
	got := Entries(st, []int{22})
	want := []Entry{
		{Owner: "ssh", Ports: "22", Proto: "tcp"},
		{Owner: "node:3", Ports: "443", Proto: "tcp", NodeIndex: 2},
		{Owner: "manual:2", Ports: "53", Proto: "udp", Note: "dns"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Owner != want[i].Owner || got[i].Ports != want[i].Ports || got[i].NodeIndex != want[i].NodeIndex || got[i].Note != want[i].Note {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
