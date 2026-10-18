package firewall

import (
	"context"
	"crypto/rand"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestSyncKindDisabled(t *testing.T) {
	f, s := service(t)
	n := len(f.Mutations())
	if err := SyncKind(s.Store).Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if m := f.Mutations(); len(m) != n {
		t.Errorf("changed the system while the firewall is off: %v", m[n:])
	}
}

func TestSyncKindFollowsState(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	st, _ := s.Store.Load()
	st.Firewall.Enabled = true
	st.Nodes[0].Port = 8443
	if err := s.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	k := SyncKind(s.Store)
	if err := k.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	conf := string(f.Files[nft.ConfigPath])
	if !strings.Contains(conf, `tcp dport 8443 accept comment "litenode:node:3"`) || !strings.Contains(conf, `tcp dport 22 accept comment "litenode:ssh"`) {
		t.Errorf("rules:\n%s", conf)
	}
	if !slices.Contains(f.Execs(), nft.NftPath+" -f "+nft.ConfigPath) {
		t.Errorf("rules not loaded: %v", f.Execs())
	}
	// Nothing changed: nothing is loaded again.
	n := len(f.Execs())
	if err := k.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	if len(f.Execs()) != n {
		t.Errorf("unchanged rules loaded again: %v", f.Execs()[n:])
	}
}

func TestSyncKindRejected(t *testing.T) {
	f, s := service(t)
	st, _ := s.Store.Load()
	st.Firewall.Enabled = true
	s.Store.Save(st)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1, Stderr: "syntax error"}}, nft.NftPath, "-c", "-f", "-")
	if err := SyncKind(s.Store).Apply(context.Background(), f); err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("err = %v", err)
	}
	if _, ok := f.Files[nft.ConfigPath]; ok {
		t.Error("rejected rules written")
	}
}

// sshAt is sshd -T for sshd on port.
func sshAt(port string) string {
	return "port " + port + "\nlistenaddress 0.0.0.0:" + port + "\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"
}

// TestSSHPortChangeSyncsFirewall moves sshd from 2222 to 2200 and back
// (rolling back to the port sshd chose itself) with the firewall on.
func TestSSHPortChangeSyncsFirewall(t *testing.T) {
	ctx := context.Background()
	f, fw := service(t)
	st, _ := fw.Store.Load()
	st.Firewall.Enabled = true
	fw.Store.Save(st)
	listenOn := func(port, hex string) {
		f.Respond(systemtest.Response{Result: system.Result{Stdout: sshAt(port)}}, ssh.SshdPath, "-T")
		f.Files["/proc/net/tcp"] = []byte("  sl  local_address rem_address   st\n   0: 00000000:" + hex + " 00000000:0000 0A\n")
	}
	listenOn("2222", "08AE")
	f.Files[ssh.MainConfigPath] = []byte("Include /etc/ssh/sshd_config.d/*.conf\n")
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", ssh.SocketUnit)
	svc := &ssh.Service{Store: fw.Store, Confirmer: fw.Confirmer, Rand: rand.Reader, Firewall: SyncKind(fw.Store)}

	f.OnCommand("systemctl reload "+ssh.Unit, func() { listenOn("2200", "0898") })
	if _, err := svc.SetPort(ctx, 2200); err != nil {
		t.Fatal(err)
	}
	conf := string(f.Files[nft.ConfigPath])
	if !strings.Contains(conf, "tcp dport 2200 accept") || strings.Contains(conf, "dport 2222") {
		t.Errorf("rules after the port change:\n%s", conf)
	}
	// The firewall follows sshd.
	execs := f.Execs()
	if slices.Index(execs, nft.NftPath+" -f "+nft.ConfigPath) < slices.Index(execs, "systemctl reload "+ssh.Unit) {
		t.Errorf("firewall loaded before sshd reloaded: %v", execs)
	}
	if err := svc.Confirmer.Disarm(ctx); err != nil {
		t.Fatal(err)
	}

	f.OnCommand("systemctl reload "+ssh.Unit, func() { listenOn("2222", "08AE") })
	if _, err := svc.RollBackLast(ctx); err != nil {
		t.Fatal(err)
	}
	conf = string(f.Files[nft.ConfigPath])
	if !strings.Contains(conf, "tcp dport 2222 accept") || strings.Contains(conf, "dport 2200") {
		t.Errorf("rules after rolling back:\n%s", conf)
	}
}

// The registry of the rollback subcommand must know the kind.
func TestSyncKindName(t *testing.T) {
	reg := change.Registry{}
	reg.Register(SyncKind(nil))
	if _, ok := reg["firewall"]; !ok {
		t.Errorf("registry = %v", reg)
	}
}
