// Package transport contains node transport implementations such as TCP,
// XHTTP, WS and gRPC.
package transport

// TCP is Xray's raw TCP transport.
type TCP struct{}

func (TCP) Name() string  { return "tcp" }
func (TCP) Label() string { return "TCP" }
