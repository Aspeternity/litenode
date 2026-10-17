// Package protocol contains node protocol implementations such as VLESS,
// Trojan and Shadowsocks.
package protocol

import "github.com/Aspeternity/litenode/internal/node"

// VLESS is the VLESS protocol.
type VLESS struct{}

func (VLESS) Name() string  { return "vless" }
func (VLESS) Label() string { return "VLESS" }

type vlessClient struct {
	ID    string `json:"id"`
	Flow  string `json:"flow,omitempty"`
	Email string `json:"email"`
}

type vlessSettings struct {
	Clients    []vlessClient `json:"clients"`
	Decryption string        `json:"decryption"`
}

// InboundSettings lists the enabled clients. The client remark is the
// Xray email (unique across all nodes, D-039).
func (VLESS) InboundSettings(n node.Node) (any, error) {
	s := vlessSettings{Clients: []vlessClient{}, Decryption: "none"}
	for _, c := range n.Clients {
		if c.Enabled {
			s.Clients = append(s.Clients, vlessClient{ID: c.UUID, Flow: c.Flow, Email: c.Remark})
		}
	}
	return s, nil
}
