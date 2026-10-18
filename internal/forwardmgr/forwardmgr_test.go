package forwardmgr

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
)

type fakeResolver map[string][]net.IP

func (r fakeResolver) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	if ips, ok := r[network+" "+host]; ok {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func service(t *testing.T) (*systemtest.Fake, *Service) {
	t.Helper()
	f := systemtest.New()
	// sshd listens on 22; a node uses 443.
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address rem_address   st\n   0: 00000000:0016 00000000:0000 0A\n")
	f.Files["/proc/sys/net/ipv6/conf/eth0/accept_ra"] = []byte("1\n")
	store := state.NewStore(f)
	st := state.New()
	st.Nodes = []node.Node{{ID: 3, Port: 443, Enabled: true}}
	st.NextID = 4
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	return f, &Service{Runner: f, Store: store, Changes: change.NewManager(f, store), Executable: "/usr/local/bin/litenode", Resolver: fakeResolver{
		"ip4 la.example.com": {net.ParseIP("203.0.113.20")},
		"ip6 la.example.com": {net.ParseIP("2001:db8::20")},
	}}
}

func activeUFW(f *systemtest.Fake) {
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
}

func TestAdd(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	activeUFW(f)
	got, err := s.Add(ctx, Input{Remark: " LA ", Protocol: forward.TCPUDP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 4 || got.Remark != "LA" || !got.Enabled {
		t.Errorf("Add = %+v", got)
	}
	if conf := string(f.Files[nft.ConfigPath]); !strings.Contains(conf, "dnat ip to 203.0.113.10:443") {
		t.Errorf("rules:\n%s", conf)
	}
	if conf := string(f.Files[sysctl.ConfigPath]); !strings.Contains(conf, "net.ipv4.ip_forward = 1") || strings.Contains(conf, "ipv6") {
		t.Errorf("sysctl:\n%s", conf)
	}
	execs := f.Execs()
	for _, want := range []string{
		"sysctl -p " + sysctl.ConfigPath,
		nft.NftPath + " -f " + nft.ConfigPath,
		"ufw route allow proto tcp to 203.0.113.10 port 443 comment litenode:forward:4",
		"ufw route allow proto udp to 203.0.113.10 port 443 comment litenode:forward:4",
	} {
		if !slices.Contains(execs, want) {
			t.Errorf("missing %q in %v", want, execs)
		}
	}
	if !(slices.Index(execs, "sysctl -p "+sysctl.ConfigPath) < slices.Index(execs, nft.NftPath+" -f "+nft.ConfigPath)) {
		t.Errorf("forwarding not enabled before the rules: %v", execs)
	}
}

func TestAddDomain(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	got, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "la.example.com", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ResolvedIPv4 != "203.0.113.20" || got.ResolvedIPv6 != "2001:db8::20" {
		t.Errorf("resolved %q %q", got.ResolvedIPv4, got.ResolvedIPv6)
	}
	// IPv6 forwarding keeps the router advertisements of eth0.
	if conf := string(f.Files[sysctl.ConfigPath]); !strings.Contains(conf, "net.ipv6.conf.eth0.accept_ra = 2\nnet.ipv6.conf.all.forwarding = 1") {
		t.Errorf("sysctl:\n%s", conf)
	}
	if _, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30002", TargetHost: "none.example.com", TargetPort: "443"}); err == nil {
		t.Error("added a domain that does not resolve")
	}
}

func TestPortConflicts(t *testing.T) {
	ctx := context.Background()
	_, s := service(t)
	in := Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"}
	first, err := s.Add(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ports, owner string
	}{{"30001", "forward:4"}, {"20-25", ""}, {"443", "node:3"}} {
		in.ListenPort, in.TargetPort = c.ports, c.ports
		var pt *PortTakenError
		if _, err := s.Add(ctx, in); !errors.As(err, &pt) || pt.Conflicts[0].Owner != c.owner {
			t.Errorf("port %s: %v", c.ports, err)
		}
	}
	// UDP on the same port is free.
	in.Protocol, in.ListenPort, in.TargetPort = forward.UDP, "30001", "443"
	if _, err := s.Add(ctx, in); err != nil {
		t.Errorf("UDP next to TCP: %v", err)
	}
	// A rule keeps its own port when modified.
	in.Protocol, in.TargetPort = forward.TCP, "8443"
	if err := s.Update(ctx, first.ID, in); err != nil {
		t.Errorf("Update: %v", err)
	}
}

func TestUpdateKeepsIDAndState(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	got, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, got.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, got.ID, Input{Remark: "x", Protocol: forward.UDP, ListenPort: "30002", TargetHost: "2001:db8::10", TargetPort: "443"}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Store.Load()
	if fw := st.Forwards[0]; fw.ID != got.ID || fw.Enabled || fw.ListenPort != "30002" || fw.Remark != "x" {
		t.Errorf("after Update: %+v", fw)
	}
	// No enabled forward: the table and the sysctl file are gone.
	if conf := string(f.Files[nft.ConfigPath]); strings.Contains(conf, "{") {
		t.Errorf("rules of a disabled forward:\n%s", conf)
	}
	if _, ok := f.Files[sysctl.ConfigPath]; ok {
		t.Error("sysctl file kept")
	}
}

func TestEnableChecksPorts(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	got, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, got.ID, false); err != nil {
		t.Fatal(err)
	}
	// A program starts listening on 30001 (0x7531).
	f.Files["/proc/net/tcp"] = append(f.Files["/proc/net/tcp"], []byte("   1: 00000000:7531 00000000:0000 0A\n")...)
	var pt *PortTakenError
	if err := s.SetEnabled(ctx, got.ID, true); !errors.As(err, &pt) {
		t.Errorf("enabled over a listening program: %v", err)
	}
	if err := s.SetEnabled(ctx, 99, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown ID: %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	activeUFW(f)
	got, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := s.Store.Load()
	st.Firewall.AutoSources = map[string][]string{forward.Owner(got.ID): {"198.51.100.7"}}
	if err := s.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	// ufw now has the rule.
	f.Files[ufw.UserRules] = []byte("### tuple ### route:allow tcp 443 203.0.113.10 any 0.0.0.0/0 in comment=6c6974656e6f64653a666f72776172643a34\n")
	if err := s.Delete(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Store.Load()
	if len(st.Forwards) != 0 || len(st.Firewall.AutoSources) != 0 {
		t.Errorf("after Delete: %+v %+v", st.Forwards, st.Firewall.AutoSources)
	}
	if !slices.Contains(f.Execs(), "ufw route delete allow proto tcp to 203.0.113.10 port 443") {
		t.Errorf("ufw rule kept: %v", f.Execs())
	}
}

// A failed step restores the state and the files.
func TestApplyFailureRestores(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1, Stderr: "Error: syntax"}, Err: errors.New("exit status 1")}, nft.NftPath, "-c", "-f", "-")
	if _, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"}); err == nil {
		t.Fatal("Add succeeded")
	}
	st, _ := s.Store.Load()
	if len(st.Forwards) != 0 {
		t.Errorf("state kept the rule: %+v", st.Forwards)
	}
	if _, ok := f.Files[sysctl.ConfigPath]; ok {
		t.Error("sysctl file kept")
	}
}
