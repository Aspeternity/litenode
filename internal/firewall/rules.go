package firewall

import (
	"strconv"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/state"
)

// Rule owners, the "litenode:<owner>" comment of each rule (D-070).
const (
	OwnerSSH = "ssh"
)

// NodeOwner names a node's rule.
func NodeOwner(id int64) string { return "node:" + strconv.FormatInt(id, 10) }

// ManualOwner names one of the user's rules.
func ManualOwner(id int64) string { return "manual:" + strconv.FormatInt(id, 10) }

// Comment is the nft comment of an owner's rule.
func Comment(owner string) string { return "litenode:" + owner }

// SSHPorts are the ports to keep open for SSH: the port LiteNode manages,
// otherwise what sshd uses now (effective, from sshd -T).
func SSHPorts(st *state.State, effective []int) []int {
	if st.SSH.Port > 0 {
		return []int{st.SSH.Port}
	}
	return effective
}

// Config is the nftables configuration the state calls for (DESIGN
// section 14): with the firewall on, the SSH ports, the enabled nodes'
// ports and the user's rules, each with its source limits. sshPorts come
// from SSHPorts.
func Config(st *state.State, sshPorts []int) nft.Config {
	if !st.Firewall.Enabled {
		return nft.Config{}
	}
	f := &nft.Firewall{BlockPing: st.Firewall.BlockPing}
	auto := func(owner, ports, proto string) {
		f.Rules = append(f.Rules, nft.Rule{Ports: ports, Proto: proto, Sources: st.Firewall.AutoSources[owner], Comment: Comment(owner)})
	}
	for _, p := range sshPorts {
		auto(OwnerSSH, strconv.Itoa(p), "tcp")
	}
	for _, n := range st.Nodes {
		if n.Enabled {
			auto(NodeOwner(n.ID), strconv.Itoa(n.Port), "tcp") // every current transport is TCP
		}
	}
	for _, r := range st.Firewall.Rules {
		f.Rules = append(f.Rules, nft.Rule{Ports: r.Port, Proto: r.Protocol, Sources: r.Sources, Comment: Comment(ManualOwner(r.ID))})
	}
	return nft.Config{Firewall: f}
}
