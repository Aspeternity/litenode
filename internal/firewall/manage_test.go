package firewall

import (
	"errors"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
)

func ruleState() *state.State {
	st := state.New()
	st.Firewall.Enabled = true
	st.Nodes = []node.Node{{ID: st.AllocID(), Port: 443, Enabled: true}}
	return st
}

func TestRuleOperations(t *testing.T) {
	st := ruleState()
	r := state.FirewallRule{Port: "28388", Protocol: "tcp", Sources: []string{"1.2.3.4"}}
	if err := AddRule(st, r); err != nil {
		t.Fatal(err)
	}
	if err := AddRule(st, r); !errors.Is(err, ErrRuleExists) {
		t.Errorf("duplicate: %v", err)
	}
	if err := AddRule(st, state.FirewallRule{Port: "28388", Protocol: "udp"}); err != nil {
		t.Errorf("same port, other protocol: %v", err)
	}
	if err := AddRule(st, state.FirewallRule{Port: "70000", Protocol: "tcp"}); !errors.Is(err, ErrInvalidPorts) {
		t.Errorf("invalid port: %v", err)
	}
	owner := ManualOwner(st.Firewall.Rules[0].ID)
	if err := UpdateRule(st, owner, state.FirewallRule{Port: "28388", Protocol: "udp"}); !errors.Is(err, ErrRuleExists) {
		t.Errorf("update into a duplicate: %v", err)
	}
	if err := UpdateRule(st, owner, state.FirewallRule{Port: "30000-30010", Protocol: "tcp", Note: "x"}); err != nil {
		t.Fatal(err)
	}
	if got := st.Firewall.Rules[0]; got.Port != "30000-30010" || ManualOwner(got.ID) != owner {
		t.Errorf("updated rule = %+v", got)
	}
	// Automatic rules keep their limit by owner; empty removes it.
	if err := SetSources(st, NodeOwner(1), []string{"5.6.7.8"}); err != nil {
		t.Fatal(err)
	}
	if es := Entries(st, []int{22}); !slices.Equal(es[1].Sources, []string{"5.6.7.8"}) {
		t.Errorf("node sources = %v", es[1].Sources)
	}
	SetSources(st, NodeOwner(1), nil)
	if len(st.Firewall.AutoSources) != 0 {
		t.Errorf("AutoSources = %v", st.Firewall.AutoSources)
	}
	if err := DeleteRule(st, owner); err != nil || len(st.Firewall.Rules) != 1 {
		t.Errorf("DeleteRule: %v, %v", err, st.Firewall.Rules)
	}
	if err := DeleteRule(st, owner); !errors.Is(err, ErrNoRule) {
		t.Errorf("delete twice: %v", err)
	}
}

func TestParseSources(t *testing.T) {
	got, err := ParseSources(" 1.2.3.4，2001:db8::/32  10.0.0.0/8,")
	if err != nil || !slices.Equal(got, []string{"1.2.3.4", "10.0.0.0/8", "2001:db8::/32"}) {
		t.Errorf("ParseSources = %v, %v", got, err)
	}
	if got, err := ParseSources(""); err != nil || got != nil {
		t.Errorf("empty = %v, %v", got, err)
	}
	if _, err := ParseSources("example.com"); !errors.Is(err, ErrInvalidSource) {
		t.Errorf("hostname: %v", err)
	}
}

func TestAffectsSSH(t *testing.T) {
	before := ruleState()
	change := func(f func(st *state.State)) bool {
		after := ruleState()
		f(after)
		return AffectsSSH(before, after, []int{22})
	}
	for name, tc := range map[string]struct {
		f    func(st *state.State)
		want bool
	}{
		"open another port": {func(st *state.State) { AddRule(st, state.FirewallRule{Port: "8080", Protocol: "tcp"}) }, false},
		"ping":              {func(st *state.State) { st.Firewall.BlockPing = true }, false},
		"node sources":      {func(st *state.State) { SetSources(st, NodeOwner(1), []string{"1.2.3.4"}) }, false},
		"ssh sources":       {func(st *state.State) { SetSources(st, OwnerSSH, []string{"1.2.3.4"}) }, true},
		"range over ssh":    {func(st *state.State) { AddRule(st, state.FirewallRule{Port: "20-30", Protocol: "tcp+udp"}) }, true},
		"udp on ssh port":   {func(st *state.State) { AddRule(st, state.FirewallRule{Port: "22", Protocol: "udp"}) }, false},
	} {
		if got := change(tc.f); got != tc.want {
			t.Errorf("%s: AffectsSSH = %v, want %v", name, got, tc.want)
		}
	}
}

func TestCompare(t *testing.T) {
	st := ruleState()
	AddRule(st, state.FirewallRule{Port: "10000-10100", Protocol: "udp", Sources: []string{"1.2.3.4", "1.2.3.5", "10.0.0.0/24"}})
	SetSources(st, NodeOwner(1), []string{"1.2.3.4"})
	want := Config(st, []int{22}).Firewall
	lines, err := nft.InputRules(want)
	if err != nil {
		t.Fatal(err)
	}
	// What nft prints back: sets reordered, sources merged.
	printed := slices.Clone(lines)
	for i, l := range printed {
		switch l {
		case `ip saddr { 1.2.3.4 } tcp dport 443 accept comment "litenode:node:1"`:
			printed[i] = `ip saddr 1.2.3.4 tcp dport 443 accept comment "litenode:node:1"`
		case `ip saddr { 1.2.3.4, 1.2.3.5, 10.0.0.0/24 } udp dport 10000-10100 accept comment "litenode:manual:2"`:
			printed[i] = `ip saddr { 1.2.3.4/31, 10.0.0.0/24 } udp dport 10000-10100 accept comment "litenode:manual:2"`
		}
	}
	if rep, err := Compare(want, &nft.Chain{Policy: "drop", Rules: printed}); err != nil || !rep.OK() {
		t.Errorf("same rules: %+v, %v\n%v", rep, err, lines)
	}
	got := &nft.Chain{Policy: "drop", Rules: append(slices.Clone(printed[:len(printed)-2]),
		`tcp dport 444 accept comment "litenode:node:1"`, `tcp dport 8080 accept comment "litenode:manual:9"`)}
	// The node rule changed its port, the manual rule is gone, an unknown one appeared.
	rep, err := Compare(want, got)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Missing, []string{"manual:2"}) || !slices.Equal(rep.Changed, []string{"node:1"}) ||
		!slices.Equal(rep.Extra, []string{"manual:9"}) || rep.BaseDiffers || rep.NotLoaded {
		t.Errorf("Compare = %+v", rep)
	}
	if rep, _ := Compare(want, &nft.Chain{Policy: "accept", Rules: printed}); !rep.BaseDiffers {
		t.Error("policy accept not reported")
	}
	if rep, _ := Compare(want, nil); !rep.NotLoaded || rep.OK() {
		t.Errorf("not loaded: %+v", rep)
	}
}
