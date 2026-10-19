package firewall

import (
	"reflect"
	"testing"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
)

func TestConfig(t *testing.T) {
	st := state.New()
	st.Nodes = []node.Node{{ID: 3, Port: 443, Enabled: true}, {ID: 4, Port: 8443},
		{ID: 5, Port: 34567, Protocol: "shadowsocks", Enabled: true}}
	st.Firewall.Rules = []state.FirewallRule{{ID: 9, Port: "30000-30010", Protocol: "tcp+udp", Sources: []string{"1.2.3.4"}}}
	st.Firewall.AutoSources = map[string][]string{"node:3": {"203.0.113.7"}}
	if c := Config(st, []int{22}); !c.Empty() {
		t.Errorf("firewall off: %+v", c)
	}
	st.Firewall.Enabled = true
	got := Config(st, SSHPorts(st, []int{22}))
	want := &nft.Firewall{Rules: []nft.Rule{
		{Ports: "22", Proto: "tcp", Comment: "litenode:ssh"},
		{Ports: "443", Proto: "tcp", Sources: []string{"203.0.113.7"}, Comment: "litenode:node:3"},
		// Shadowsocks 2022 serves UDP too (DESIGN section 9.8).
		{Ports: "34567", Proto: "tcp+udp", Comment: "litenode:node:5"},
		{Ports: "30000-30010", Proto: "tcp+udp", Sources: []string{"1.2.3.4"}, Comment: "litenode:manual:9"},
	}}
	if !reflect.DeepEqual(got.Firewall, want) {
		t.Errorf("got %+v\nwant %+v", got.Firewall, want)
	}
	// A port LiteNode manages wins over what sshd uses now (it is about
	// to move there).
	st.SSH.Port = 22222
	if p := SSHPorts(st, []int{22}); len(p) != 1 || p[0] != 22222 {
		t.Errorf("SSHPorts = %v", p)
	}
}
