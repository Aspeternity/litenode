package transport

import "github.com/Aspeternity/litenode/internal/node"

// Hysteria is Xray's Hysteria2 transport (QUIC, DESIGN section 9.10). It
// only carries the hysteria protocol.
type Hysteria struct{}

func (Hysteria) Name() string  { return "hysteria" }
func (Hysteria) Label() string { return "Hysteria2" }

func (Hysteria) Network() string { return "hysteria" }

type hysteriaSettings struct {
	Version int `json:"version"`
}

// StreamSettings selects version 2. Congestion control stays at Xray's
// default: without a server bandwidth (quicParams.brutalUp) the server
// sends with BBR whatever bandwidth the client declares (D-118).
func (Hysteria) StreamSettings(node.Node) (string, any, error) {
	return "hysteriaSettings", hysteriaSettings{Version: 2}, nil
}

// ShareParams: Hysteria2 links have no transport parameters.
func (Hysteria) ShareParams(node.Node) ([]node.Param, error) { return nil, nil }
