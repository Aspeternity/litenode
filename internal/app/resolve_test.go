package app

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
)

type resolverFunc func(network, host string) []net.IP

func (r resolverFunc) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	if ips := r(network, host); ips != nil {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// domainState is a fake system with an enabled forward to la.example.com,
// last resolved to 203.0.113.20.
func domainState(t *testing.T) *systemtest.Fake {
	t.Helper()
	f := supported()
	st := state.New()
	st.Forwards = []forward.Forward{{ID: 1, Enabled: true, Protocol: forward.TCP, ListenPort: "30001",
		TargetHost: "la.example.com", TargetPort: "443", ResolvedIPv4: "203.0.113.20"}}
	st.NextID = 2
	if err := state.NewStore(f).Save(st); err != nil {
		t.Fatal(err)
	}
	return f
}

func resolveTo(ip string) resolverFunc {
	return func(network, host string) []net.IP {
		if network == "ip4" && host == "la.example.com" {
			return []net.IP{net.ParseIP(ip)}
		}
		return nil
	}
}

func TestResolveUpdates(t *testing.T) {
	f := domainState(t)
	r := runEnv(t, Env{Runner: f, Euid: 0, Args: []string{"internal", "resolve"}, LockPath: filepath.Join(t.TempDir(), "l"),
		ForwardResolver: resolveTo("203.0.113.30")}, "")
	if r.code != 0 || !strings.Contains(r.stdout, text.ResolveUpdated(1, "la.example.com", "203.0.113.30", "")) {
		t.Fatalf("%+v", r)
	}
	if conf := string(f.Files[nft.ConfigPath]); !strings.Contains(conf, "dnat ip to 203.0.113.30:443") {
		t.Errorf("rules:\n%s", conf)
	}
}

func TestResolveUnchanged(t *testing.T) {
	f := domainState(t)
	r := runEnv(t, Env{Runner: f, Euid: 0, Args: []string{"internal", "resolve"}, LockPath: filepath.Join(t.TempDir(), "l"),
		ForwardResolver: resolveTo("203.0.113.20")}, "")
	if r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Errorf("%+v", r)
	}
	if _, ok := f.Files[nft.ConfigPath]; ok {
		t.Error("rules written without a new address")
	}
}

// While a menu holds the lock the round is skipped without any change.
func TestResolveSkipsWhenLocked(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "litenode.lock")
	menu, err := state.AcquireLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer menu.Release()
	f := domainState(t)
	before := len(f.Calls())
	r := runEnv(t, Env{Runner: f, Euid: 0, Args: []string{"internal", "resolve"}, LockPath: lockPath,
		ForwardResolver: resolveTo("203.0.113.30")}, "")
	if r.code != 0 || !strings.Contains(r.stdout, text.ResolveSkipped) {
		t.Errorf("%+v", r)
	}
	if calls := f.Calls(); len(calls) != before {
		t.Errorf("system touched while locked: %+v", calls[before:])
	}
}

func TestResolveNeedsRoot(t *testing.T) {
	r := run(t, domainState(t), 1000, "", "internal", "resolve")
	if r.code == 0 || !strings.Contains(r.stderr, text.NotRoot) {
		t.Errorf("%+v", r)
	}
}
