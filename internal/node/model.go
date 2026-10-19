// Package node defines the node and client models and the
// protocol/transport/security registry.
//
// A node combines a protocol (vless, ...), a transport (tcp, ...) and a
// security layer (reality, ...). Each component's own settings live in a
// typed optional field of Node (D-039), so the state file reads like
// DESIGN section 5.
package node

// Node is one inbound, as stored in the state file.
type Node struct {
	ID      int64  `json:"id"`
	Remark  string `json:"remark"`
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	// SharePort is the port put in share links when it differs from Port
	// (NAT machines, D-043). 0 means Port.
	SharePort int    `json:"share_port,omitempty"`
	Protocol  string `json:"protocol"`
	Transport string `json:"transport"`
	Security  string `json:"security"`
	// Reality is set when Security is "reality".
	Reality *Reality `json:"reality,omitempty"`
	// VLESSEncryption is set for VLESS nodes with VLESS Encryption
	// (security "none", DESIGN section 9.7).
	VLESSEncryption *VLESSEncryption `json:"vless_encryption,omitempty"`
	// Shadowsocks is set for Shadowsocks 2022 nodes (DESIGN section 9.8).
	Shadowsocks *Shadowsocks `json:"shadowsocks,omitempty"`
	// TLS is set when Security is "tls" (Hysteria2, DESIGN section 9.10).
	TLS     *TLS     `json:"tls,omitempty"`
	Clients []Client `json:"clients"`
}

// TLS holds a node's self-signed certificate (D-117). Clients pin its
// fingerprint instead of trusting a CA, so ServerName is only what they
// send as SNI.
type TLS struct {
	ServerName string `json:"server_name"`
	// Certificate and Key are PEM encoded.
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

// Shadowsocks holds the method and the server key of a Shadowsocks 2022
// node. Each client has its own user key (Client.Key).
type Shadowsocks struct {
	Method    string `json:"method"`
	ServerKey string `json:"server_key"`
}

// UsesTCP reports whether the node's inbound listens on TCP: all nodes
// except Hysteria2, which is QUIC over UDP only (DESIGN section 9.10).
func (n Node) UsesTCP() bool { return n.Protocol != "hysteria" }

// UsesUDP reports whether the node's inbound listens on UDP: Shadowsocks
// 2022 (network "tcp,udp", DESIGN section 9.8) and Hysteria2.
func (n Node) UsesUDP() bool { return n.Protocol == "shadowsocks" || n.Protocol == "hysteria" }

// PortProto is the protocol of the node's port as in firewall rules:
// "tcp", "udp" or "tcp+udp".
func (n Node) PortProto() string {
	switch {
	case n.UsesTCP() && n.UsesUDP():
		return "tcp+udp"
	case n.UsesUDP():
		return "udp"
	}
	return "tcp"
}

// VLESSEncryption holds the X25519 authentication key pair of VLESS
// Encryption, encoded like "xray x25519" (D-107). The server keeps the
// private key; clients use the public key.
type VLESSEncryption struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

// Reality holds the REALITY security settings of a node.
type Reality struct {
	// Target is the site unverified connections fall back to, "host:port".
	Target        string        `json:"target"`
	ServerNames   []string      `json:"server_names"`
	PrivateKey    string        `json:"private_key"`
	PublicKey     string        `json:"public_key"`
	ShortIDs      []string      `json:"short_ids"`
	Fingerprint   string        `json:"fingerprint"`
	FallbackLimit FallbackLimit `json:"fallback_limit"`
}

// FallbackLimit throttles fallback connections so the VPS cannot be used
// as a free relay to the target site (DESIGN section 10.4).
type FallbackLimit struct {
	AfterBytes       int64 `json:"after_bytes"`
	BytesPerSec      int64 `json:"bytes_per_sec"`
	BurstBytesPerSec int64 `json:"burst_bytes_per_sec"`
}

// Client is one user of a node. Its remark is the Xray "email" and is
// unique across all clients of all nodes. VLESS clients have a UUID,
// Shadowsocks 2022 and Hysteria2 clients a key instead.
type Client struct {
	ID     int64  `json:"id"`
	Remark string `json:"remark"`
	UUID   string `json:"uuid,omitempty"`
	// Key is the Shadowsocks 2022 user key (standard base64) or the
	// Hysteria2 password.
	Key     string `json:"key,omitempty"`
	Flow    string `json:"flow,omitempty"`
	Enabled bool   `json:"enabled"`
}

// FlowVision is the XTLS Vision flow.
const FlowVision = "xtls-rprx-vision"

// DefaultFallbackLimit is the fallback throttle from the Xray docs example
// (DESIGN section 10.4): about 15 MB, then 1 MB/s.
var DefaultFallbackLimit = FallbackLimit{AfterBytes: 10 << 20, BytesPerSec: 1 << 20, BurstBytesPerSec: 5 << 20}

// DefaultFingerprint is the uTLS fingerprint put in share links.
const DefaultFingerprint = "chrome"
