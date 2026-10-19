package firewall

import (
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/forward"
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

// ForwardOwner names a forward's rule.
func ForwardOwner(id int64) string { return forward.Owner(id) }

// IsForward reports whether owner is a forward. Forwarded traffic is
// filtered in the forward chain, not the input chain.
func IsForward(owner string) bool { return strings.HasPrefix(owner, "forward:") }

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

// Entry is one rule of the firewall, automatic or the user's.
type Entry struct {
	// Owner is OwnerSSH, NodeOwner(id) or ManualOwner(id).
	Owner string
	// Ports is one port or a range; Proto is "tcp", "udp" or "tcp+udp".
	Ports, Proto string
	// Sources limit who may connect; empty means anyone.
	Sources []string
	// NodeIndex is the node's position in the node list (from 1) for node
	// rules, 0 otherwise.
	NodeIndex int
	// ForwardIndex is the forward's position in the forward list (from 1)
	// for forward rules, 0 otherwise.
	ForwardIndex int
	// Note is the user's note of a manual rule.
	Note string
}

// Entries lists the rules the state calls for, whether or not the
// firewall is on: the SSH ports, the enabled nodes' ports, the enabled
// forwards' local ports and the user's rules, each with its source
// limits. sshPorts come from SSHPorts.
func Entries(st *state.State, sshPorts []int) []Entry {
	var es []Entry
	for _, p := range sshPorts {
		es = append(es, Entry{Owner: OwnerSSH, Ports: strconv.Itoa(p), Proto: "tcp", Sources: st.Firewall.AutoSources[OwnerSSH]})
	}
	for i, n := range st.Nodes {
		if n.Enabled {
			o := NodeOwner(n.ID)
			es = append(es, Entry{Owner: o, Ports: strconv.Itoa(n.Port), Proto: n.PortProto(), Sources: st.Firewall.AutoSources[o], NodeIndex: i + 1})
		}
	}
	for i, f := range st.Forwards {
		if f.Enabled {
			o := ForwardOwner(f.ID)
			es = append(es, Entry{Owner: o, Ports: f.ListenPort, Proto: f.Protocol, Sources: st.Firewall.AutoSources[o], ForwardIndex: i + 1})
		}
	}
	for _, r := range st.Firewall.Rules {
		es = append(es, Entry{Owner: ManualOwner(r.ID), Ports: r.Port, Proto: r.Protocol, Sources: r.Sources, Note: r.Note})
	}
	return es
}

// Config is the nftables configuration the state calls for (DESIGN
// sections 11 and 14): the input filter while the firewall is on, and the
// enabled forwards whether or not it is.
func Config(st *state.State, sshPorts []int) nft.Config {
	var c nft.Config
	for _, f := range st.Forwards {
		if f.Enabled {
			o := ForwardOwner(f.ID)
			c.Forwards = append(c.Forwards, nft.Forward{
				Ports: f.ListenPort, Proto: f.Protocol, IPv4: f.IPv4(), IPv6: f.IPv6(), TargetPorts: f.TargetPort,
				Sources: st.Firewall.AutoSources[o], Comment: Comment(o),
			})
		}
	}
	if !st.Firewall.Enabled {
		return c
	}
	c.Firewall = &nft.Firewall{BlockPing: st.Firewall.BlockPing}
	for _, e := range Entries(st, sshPorts) {
		if !IsForward(e.Owner) {
			c.Firewall.Rules = append(c.Firewall.Rules, nft.Rule{Ports: e.Ports, Proto: e.Proto, Sources: e.Sources, Comment: Comment(e.Owner)})
		}
	}
	return c
}
