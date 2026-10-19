// Package ports decides whether ports are free (DESIGN section 2, D-037):
// a port is taken if any program listens on it, or if a node or forward
// in the state uses it (forwards are nftables DNAT and do not listen).
package ports

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/system"
)

// Proto is a transport protocol.
type Proto string

const (
	TCP Proto = "tcp"
	UDP Proto = "udp"
)

// Key identifies one port of one protocol.
type Key struct {
	Proto Proto
	Port  int
}

// tcpListen is the TCP_LISTEN state in /proc/net/tcp*.
const tcpListen = "0A"

// ParseProcNet returns the local ports in a /proc/net/{tcp,tcp6,udp,udp6}
// table. For TCP only listening sockets count; every bound UDP socket
// does. Any local address counts, including 127.0.0.1 (D-037).
func ParseProcNet(data []byte, proto Proto) []int {
	var out []int
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 {
			continue
		}
		_, portHex, ok := strings.Cut(f[1], ":")
		if !ok {
			continue
		}
		if proto == TCP && f[3] != tcpListen {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil || port == 0 {
			continue
		}
		out = append(out, int(port))
	}
	return out
}

// Listening reads the ports programs listen on. Missing tables (no IPv6)
// are skipped.
func Listening(r system.Runner) (map[Key]bool, error) {
	set := map[Key]bool{}
	for _, t := range []struct {
		path  string
		proto Proto
	}{{"/proc/net/tcp", TCP}, {"/proc/net/tcp6", TCP}, {"/proc/net/udp", UDP}, {"/proc/net/udp6", UDP}} {
		data, err := r.ReadFile(t.path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", t.path, err)
		}
		for _, p := range ParseProcNet(data, t.proto) {
			set[Key{t.proto, p}] = true
		}
	}
	return set, nil
}

// Claim is a port used by an item in the state.
type Claim struct {
	Key
	// Owner identifies the item, e.g. "node:3", "forward:5" or "ssh".
	Owner string
}

// NodeOwner names a node as a claim owner.
func NodeOwner(id int64) string { return "node:" + strconv.FormatInt(id, 10) }

// NodeKeys lists the keys a node uses on port: TCP, and UDP for nodes
// that serve it (Shadowsocks 2022).
func NodeKeys(n node.Node, port int) []Key {
	if n.UsesUDP() {
		return []Key{{TCP, port}, {UDP, port}}
	}
	return []Key{{TCP, port}}
}

// NodeClaims lists the ports of all nodes, enabled or not.
func NodeClaims(nodes []node.Node) []Claim {
	var out []Claim
	for _, n := range nodes {
		for _, k := range NodeKeys(n, n.Port) {
			out = append(out, Claim{k, NodeOwner(n.ID)})
		}
	}
	return out
}

// ForwardClaims lists the local ports of all forwards, enabled or not:
// their DNAT takes the traffic of these ports from any program listening
// there.
func ForwardClaims(fs []forward.Forward) []Claim {
	var out []Claim
	for _, f := range fs {
		from, to, err := forward.ParsePorts(f.ListenPort)
		if err != nil {
			continue
		}
		for _, k := range Range(f.Protocol, from, to) {
			out = append(out, Claim{k, forward.Owner(f.ID)})
		}
	}
	return out
}

// StateClaims lists the ports of every node and forward in the state.
func StateClaims(nodes []node.Node, fs []forward.Forward) []Claim {
	return append(NodeClaims(nodes), ForwardClaims(fs)...)
}

// Conflict explains why a wanted port is taken.
type Conflict struct {
	Key
	// Owner is the state item using the port, "" if a program listens.
	Owner string
}

// Checker answers whether ports are free.
type Checker struct {
	Listening map[Key]bool
	Claims    []Claim
}

// Check returns the conflicts for the wanted ports. self are the ports
// the item being changed holds now (its owner name and current ports):
// they are ignored both as claims and as listening sockets, because the
// item's own program (Xray, sshd) listens on them.
func (c Checker) Check(want []Key, self string, selfPorts []Key) []Conflict {
	own := map[Key]bool{}
	for _, k := range selfPorts {
		own[k] = true
	}
	var out []Conflict
	for _, k := range want {
		if conflict, ok := c.owner(k, self); ok {
			out = append(out, Conflict{k, conflict})
			continue
		}
		if c.Listening[k] && !own[k] {
			out = append(out, Conflict{k, ""})
		}
	}
	return out
}

func (c Checker) owner(k Key, self string) (string, bool) {
	for _, cl := range c.Claims {
		if cl.Key == k && cl.Owner != self {
			return cl.Owner, true
		}
	}
	return "", false
}

// Range expands ports from..to (inclusive) for a protocol, "tcp+udp"
// giving both.
func Range(protos string, from, to int) []Key {
	var ps []Proto
	switch protos {
	case "tcp":
		ps = []Proto{TCP}
	case "udp":
		ps = []Proto{UDP}
	default:
		ps = []Proto{TCP, UDP}
	}
	var out []Key
	for p := from; p <= to; p++ {
		for _, pr := range ps {
			out = append(out, Key{pr, p})
		}
	}
	return out
}

// RandomFree picks a port in 10000-60000 for a new node n when the
// default is taken, free for every key the node needs (NodeKeys). It
// gives up after many tries and returns 0. A nil rng uses the global
// source.
func (c Checker) RandomFree(rng *rand.Rand, n node.Node) int {
	intN := rand.IntN
	if rng != nil {
		intN = rng.IntN
	}
	for range 1000 {
		p := 10000 + intN(50001)
		if len(c.Check(NodeKeys(n, p), "", nil)) == 0 {
			return p
		}
	}
	return 0
}
