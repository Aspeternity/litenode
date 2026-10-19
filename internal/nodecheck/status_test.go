package nodecheck

import (
	"context"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

var t0 = time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

func at(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }

func counts(kv ...any) map[string]nft.ProbeCount {
	m := map[string]nft.ProbeCount{}
	for i := 0; i < len(kv); i += 3 {
		m[kv[i].(string)] = nft.ProbeCount{Arrived: uint64(kv[i+1].(int)), Passed: uint64(kv[i+2].(int))}
	}
	return m
}

func statuses(s Snapshot) []Status {
	var out []Status
	for _, p := range s.Protos {
		out = append(out, p.Status)
	}
	return out
}

func TestNodePorts(t *testing.T) {
	ss := NodePorts(node.Node{Protocol: "shadowsocks", Port: 42001})
	if !slices.Equal(ss, []nft.ProbePort{{Proto: "tcp", Port: 42001}, {Proto: "udp", Port: 42001}}) {
		t.Errorf("shadowsocks: %v", ss)
	}
	re := NodePorts(node.Node{Protocol: "vless", Port: 443})
	if !slices.Equal(re, []nft.ProbePort{{Proto: "tcp", Port: 443}}) {
		t.Errorf("vless: %v", re)
	}
}

func TestStatus(t *testing.T) {
	tr := NewTracker([]string{"tcp", "udp"}, t0)
	s := tr.Update(counts("tcp", 0, 0, "udp", 0, 0), at(1))
	if !slices.Equal(statuses(s), []Status{Unreached, Unreached}) || s.Done {
		t.Fatalf("start: %v", s)
	}
	// 4 passed is not enough; UDP arrives but nothing passes yet.
	s = tr.Update(counts("tcp", 4, 4, "udp", 6, 0), at(2))
	if !slices.Equal(statuses(s), []Status{Waiting, Waiting}) {
		t.Fatalf("2s: %v", statuses(s))
	}
	s = tr.Update(counts("tcp", 9, 5, "udp", 8, 0), at(2.5))
	if !slices.Equal(statuses(s), []Status{Open, Waiting}) {
		t.Fatalf("2.5s: %v", statuses(s))
	}
	// 3 seconds since start without a passed UDP packet: blocked.
	s = tr.Update(counts("tcp", 9, 5, "udp", 8, 0), at(3))
	if !slices.Equal(statuses(s), []Status{Open, Blocked}) || s.Done {
		t.Fatalf("3s: %v done=%v", statuses(s), s.Done)
	}
	// Linger 2 seconds before ending.
	if s = tr.Update(counts("tcp", 9, 5, "udp", 9, 0), at(4.9)); s.Done {
		t.Fatal("ended before linger")
	}
	if s = tr.Update(counts("tcp", 9, 5, "udp", 9, 0), at(5)); !s.Done || s.TimedOut {
		t.Fatalf("5s: done=%v timedout=%v", s.Done, s.TimedOut)
	}
}

func TestBlockedNeedsQuietPassing(t *testing.T) {
	tr := NewTracker([]string{"udp"}, t0)
	tr.Update(counts("udp", 3, 2), at(5))
	// Passed grew at 5s, so 7s is not blocked yet, 8s is.
	if s := tr.Update(counts("udp", 10, 2), at(7)); s.Protos[0].Status != Waiting {
		t.Errorf("7s: %v", s.Protos[0].Status)
	}
	if s := tr.Update(counts("udp", 12, 2), at(8)); s.Protos[0].Status != Blocked {
		t.Errorf("8s: %v", s.Protos[0].Status)
	}
	// Passing resumes: no longer decided, the linger restarts.
	s := tr.Update(counts("udp", 13, 3), at(9))
	if s.Protos[0].Status != Waiting || s.Done {
		t.Errorf("9s: %v done=%v", s.Protos[0].Status, s.Done)
	}
}

func TestLimitAndHints(t *testing.T) {
	tr := NewTracker([]string{"tcp", "udp"}, t0)
	if s := tr.Update(counts("tcp", 9, 9), at(29)); len(s.Hints) != 0 {
		t.Errorf("29s hints: %v", s.Hints)
	}
	if s := tr.Update(counts("tcp", 9, 9), at(30)); !slices.Equal(s.Hints, []Hint{HintUDP}) || s.Done {
		t.Errorf("30s: hints %v done %v", s.Hints, s.Done)
	}
	if s := tr.Update(counts("tcp", 9, 9), at(1800)); !s.Done || !s.TimedOut {
		t.Errorf("limit: done=%v timedout=%v", s.Done, s.TimedOut)
	}

	tr = NewTracker([]string{"tcp"}, t0)
	if s := tr.Update(nil, at(59)); len(s.Hints) != 0 {
		t.Errorf("59s hints: %v", s.Hints)
	}
	if s := tr.Update(nil, at(60)); !slices.Equal(s.Hints, []Hint{HintNoPackets}) {
		t.Errorf("60s hints: %v", s.Hints)
	}
	// No UDP hint for nodes without UDP, nor once something arrived.
	if s := tr.Update(counts("tcp", 1, 0), at(61)); len(s.Hints) != 0 {
		t.Errorf("61s hints: %v", s.Hints)
	}
}

func TestFinal(t *testing.T) {
	s := Snapshot{Protos: []Proto{
		{Name: "tcp", Count: nft.ProbeCount{Arrived: 3, Passed: 1}, Status: Waiting},
		{Name: "udp", Count: nft.ProbeCount{Arrived: 2}, Status: Waiting},
	}}
	got := Final(s)
	if got[0].Status != Open || got[1].Status != Blocked {
		t.Errorf("final: %v", got)
	}
	if s.Protos[0].Status != Waiting {
		t.Error("Final changed the snapshot")
	}
}

func TestConclude(t *testing.T) {
	src := []netip.Addr{netip.MustParseAddr("192.0.2.1")}
	open := Proto{Name: "tcp", Count: nft.ProbeCount{Arrived: 9, Passed: 9, Sources: src}, Status: Open}
	blockedUDP := Proto{Name: "udp", Count: nft.ProbeCount{Arrived: 9}, Status: Blocked}
	for _, tc := range []struct {
		name   string
		protos []Proto
		env    Env
		want   [][]Reason
	}{
		{"open", []Proto{open}, Env{}, [][]Reason{{ReasonOpen}}},
		{"ufw", []Proto{open, blockedUDP}, Env{UFW: true}, [][]Reason{{ReasonOpen}, {ReasonBlockedUFW}}},
		{"firewall", []Proto{open, blockedUDP}, Env{Firewall: true}, [][]Reason{{ReasonOpen}, {ReasonBlockedFirewall}}},
		{"both", []Proto{blockedUDP}, Env{UFW: true, Firewall: true}, [][]Reason{{ReasonBlockedUFW, ReasonBlockedFirewall}}},
		{"other", []Proto{blockedUDP}, Env{Foreign: []string{"ip filter INPUT"}}, [][]Reason{{ReasonBlockedOther}}},
		{"udp only unreached", []Proto{open, {Name: "udp"}}, Env{}, [][]Reason{{ReasonOpen}, {ReasonUnreachedUDP}}},
		{"blocked tcp counts as arrived", []Proto{{Name: "tcp", Status: Blocked}, {Name: "udp"}}, Env{UFW: true}, [][]Reason{{ReasonBlockedUFW}, {ReasonUnreachedUDP}}},
		{"nothing", []Proto{{Name: "tcp"}, {Name: "udp"}}, Env{}, [][]Reason{{ReasonUnreached}, {ReasonUnreached}}},
	} {
		got := Conclude(tc.protos, tc.env)
		for i, f := range got {
			if !slices.Equal(f.Reasons, tc.want[i]) {
				t.Errorf("%s %s: %v, want %v", tc.name, f.Proto, f.Reasons, tc.want[i])
			}
		}
		if tc.name == "open" && !slices.Equal(got[0].Sources, src) {
			t.Errorf("sources: %v", got[0].Sources)
		}
		if tc.name == "other" && !slices.Equal(got[0].Foreign, []string{"ip filter INPUT"}) {
			t.Errorf("foreign: %v", got[0].Foreign)
		}
	}
	if !AnyBlocked([]Proto{open, blockedUDP}) || AnyBlocked([]Proto{open}) {
		t.Error("AnyBlocked")
	}
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	f := systemtest.New()
	if err := Start(ctx, f, []nft.ProbePort{{Proto: "tcp", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if calls[0].Command != "nft -f -" || !strings.Contains(string(calls[0].Data), `tcp dport 443 counter name "arrive_tcp"`) {
		t.Errorf("start: %+v", calls[0])
	}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: `{"nftables": [{"counter": {"name": "pass_tcp", "table": "litenode-probe", "packets": 3}}]}`}},
		"nft", "-j", "list", "table", "inet", "litenode-probe")
	c, err := Read(ctx, f)
	if err != nil || c["tcp"].Passed != 3 {
		t.Errorf("read: %v %v", c, err)
	}
	if err := Stop(ctx, f); err != nil {
		t.Fatal(err)
	}
	if last := f.Calls()[len(f.Calls())-1]; string(last.Data) != removeScript {
		t.Errorf("stop: %q", last.Data)
	}
}

func TestRemoveLeftover(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		listing string
		removes bool
	}{
		{"table inet litenode\n", false},
		{"table inet litenode\ntable inet litenode-probe\n", true},
	} {
		f := systemtest.New()
		f.Respond(systemtest.Response{Result: system.Result{Stdout: tc.listing}}, "nft", "list", "tables")
		if err := RemoveLeftover(ctx, f); err != nil {
			t.Fatal(err)
		}
		if got := len(f.Mutations()) > 0; got != tc.removes {
			t.Errorf("%q: removes=%v", tc.listing, got)
		}
	}
	// nft missing: nothing happens.
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 127}}, "nft", "list", "tables")
	if err := RemoveLeftover(ctx, f); err != nil || len(f.Mutations()) != 0 {
		t.Errorf("no nft: %v %v", err, f.Mutations())
	}
}
