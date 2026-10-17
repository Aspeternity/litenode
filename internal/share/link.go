// Package share generates share links, terminal QR codes and detects the
// public address (DESIGN section 9.5).
package share

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Aspeternity/litenode/internal/node"
)

// Link builds the share link of client c of node n. addr is the share
// address (IP or domain); the port is the node's share port if set, else
// its listening port (D-043).
func Link(reg *node.Registry, n node.Node, c node.Client, addr string) (string, error) {
	if addr == "" {
		return "", errors.New("share address not set")
	}
	if err := reg.CheckNode(n); err != nil {
		return "", err
	}
	p, _ := reg.Protocol(n.Protocol)
	t, _ := reg.Transport(n.Transport)
	s, _ := reg.Security(n.Security)
	sp, err := s.ShareParams(n)
	if err != nil {
		return "", fmt.Errorf("node %d: %w", n.ID, err)
	}
	tp, err := t.ShareParams(n)
	if err != nil {
		return "", fmt.Errorf("node %d: %w", n.ID, err)
	}
	port := n.Port
	if n.SharePort > 0 {
		port = n.SharePort
	}
	return p.ShareLink(n, c, addr, port, append(sp, tp...))
}

// ValidAddress reports whether s can be used as a share address: an IP
// address or a domain name.
func ValidAddress(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if len(s) == 0 || len(s) > 253 || !strings.Contains(s, ".") {
		return false
	}
	for _, label := range strings.Split(strings.ToLower(s), ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
