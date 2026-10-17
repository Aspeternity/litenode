// Package transport contains node transport implementations such as TCP,
// XHTTP, WS and gRPC.
package transport

import "github.com/Aspeternity/litenode/internal/node"

// TCP is Xray's raw TCP transport.
type TCP struct{}

func (TCP) Name() string  { return "tcp" }
func (TCP) Label() string { return "TCP" }

// Network is "tcp", which Xray still accepts as the name of the raw
// transport (renamed "raw" in v24.9), so older versions work too.
func (TCP) Network() string { return "tcp" }

func (TCP) StreamSettings(node.Node) (string, any, error) { return "", nil, nil }

// ShareParams for raw TCP: type=tcp.
func (TCP) ShareParams(node.Node) ([]node.Param, error) {
	return []node.Param{{Key: "type", Value: "tcp"}}, nil
}
