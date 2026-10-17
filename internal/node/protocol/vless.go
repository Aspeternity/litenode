// Package protocol contains node protocol implementations such as VLESS,
// Trojan and Shadowsocks.
package protocol

// VLESS is the VLESS protocol.
type VLESS struct{}

func (VLESS) Name() string  { return "vless" }
func (VLESS) Label() string { return "VLESS" }
