// Package security contains node security layer implementations such as
// REALITY, TLS and none.
package security

// Reality is the REALITY security layer.
type Reality struct{}

func (Reality) Name() string  { return "reality" }
func (Reality) Label() string { return "Reality" }
