package security

import "github.com/Aspeternity/litenode/internal/node"

// None is the empty security layer. It is only legal for VLESS nodes with
// VLESS Encryption, which encrypts the traffic itself (DESIGN section 9.7).
type None struct{}

func (None) Name() string  { return "none" }
func (None) Label() string { return "None" }

// StreamSecurity sets streamSettings.security to "none" without settings.
func (None) StreamSecurity(node.Node) (string, string, any, error) {
	return "none", "", nil, nil
}

// ShareParams gives "security=none".
func (None) ShareParams(node.Node) ([]node.Param, error) {
	return []node.Param{{Key: "security", Value: "none"}}, nil
}
