package firewall

import (
	"context"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/state"
)

var update = flag.Bool("update", false, "rewrite golden files")

// forwardState has an IPv4 target, an IPv6 target, a domain with A and
// AAAA records, a domain with only an A record and a disabled rule.
func forwardState() *state.State {
	st := state.New()
	st.Forwards = []forward.Forward{
		{ID: 5, Enabled: true, Protocol: forward.TCPUDP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"},
		{ID: 6, Enabled: true, Protocol: forward.TCP, ListenPort: "30002", TargetHost: "2001:db8::10", TargetPort: "443"},
		{ID: 7, Enabled: true, Protocol: forward.TCPUDP, ListenPort: "30003", TargetHost: "la.example.com", TargetPort: "443", ResolvedIPv4: "203.0.113.20", ResolvedIPv6: "2001:db8::20"},
		{ID: 8, Enabled: true, Protocol: forward.UDP, ListenPort: "30004", TargetHost: "v4.example.com", TargetPort: "443", ResolvedIPv4: "203.0.113.30"},
		{ID: 9, Protocol: forward.TCP, ListenPort: "30005", TargetHost: "203.0.113.40", TargetPort: "443"},
	}
	return st
}

func TestConfigForwardsGolden(t *testing.T) {
	st := forwardState()
	for name, enabled := range map[string]bool{"forwards": false, "forwards-firewall": true} {
		st.Firewall.Enabled = enabled
		st.Firewall.AutoSources = map[string][]string{"forward:7": {"198.51.100.7"}}
		got, err := nft.Generate(Config(st, []int{22}))
		if err != nil {
			t.Fatal(err)
		}
		path := "testdata/" + name + ".nft"
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s differs (go test -update):\n%s", name, got)
		}
	}
}

func TestEntriesForwards(t *testing.T) {
	st := forwardState()
	es := Entries(st, []int{22})
	var got []string
	for _, e := range es {
		if IsForward(e.Owner) {
			got = append(got, e.Owner+" "+e.Ports+" "+e.Proto)
			if st.Forwards[e.ForwardIndex-1].ID != forwardID(e.Owner) {
				t.Errorf("%s has index %d", e.Owner, e.ForwardIndex)
			}
		}
	}
	want := []string{"forward:5 30001 tcp+udp", "forward:6 30002 tcp", "forward:7 30003 tcp+udp", "forward:8 30004 udp"}
	if !slices.Equal(got, want) {
		t.Errorf("forward entries %v, want %v", got, want)
	}
	// Forward rules never open the input chain.
	st.Firewall.Enabled = true
	for _, r := range Config(st, []int{22}).Firewall.Rules {
		if strings.Contains(r.Comment, "forward") {
			t.Errorf("input rule %+v", r)
		}
	}
}

func forwardID(owner string) int64 {
	for _, id := range []int64{5, 6, 7, 8, 9} {
		if forward.Owner(id) == owner {
			return id
		}
	}
	return 0
}

// A forward does not reach sshd, so it never counts as SSH access.
func TestForwardNotSSHAccess(t *testing.T) {
	before := forwardState()
	before.Firewall.Enabled = true
	after := forwardState()
	after.Firewall.Enabled = true
	after.Forwards[0].ListenPort = "20-30"
	after.Forwards[0].TargetPort = "20-30"
	if AffectsSSH(before, after, []int{22}) {
		t.Error("a forward range over the SSH port affects SSH")
	}
}

func TestSyncKindForwards(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	st, _ := s.Store.Load()
	st.Forwards = forwardState().Forwards
	if err := s.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	k := SyncKind(s.Store)
	if err := k.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	conf := string(f.Files[nft.ConfigPath])
	if !strings.Contains(conf, "chain prerouting") || strings.Contains(conf, "chain input") || strings.Contains(conf, "chain forward") {
		t.Errorf("rules with the firewall off:\n%s", conf)
	}
	if !slices.Contains(f.Execs(), nft.NftPath+" -f "+nft.ConfigPath) {
		t.Errorf("rules not loaded: %v", f.Execs())
	}
	// The last forward is deleted: the table is removed.
	st.Forwards = nil
	if err := s.Store.Save(st); err != nil {
		t.Fatal(err)
	}
	if err := k.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	if conf := string(f.Files[nft.ConfigPath]); strings.Contains(conf, "{") {
		t.Errorf("table kept:\n%s", conf)
	}
}
