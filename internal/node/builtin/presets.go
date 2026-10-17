package builtin

import (
	"fmt"
	"net"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/security"
)

// buildVLESSRealityVision creates a VLESS + TCP + REALITY node with one
// Vision client: new x25519 key pair, one 8-hex-digit shortId, a v4 UUID,
// the chrome fingerprint and the default fallback limit.
func buildVLESSRealityVision(in node.BuildInput) (node.Node, error) {
	host, _, err := net.SplitHostPort(in.Target)
	if err != nil || host == "" {
		return node.Node{}, fmt.Errorf("invalid reality target %q", in.Target)
	}
	priv, pub, err := security.NewX25519(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	sid, err := security.NewShortID(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	uuid, err := node.NewUUID(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	return node.Node{
		ID: in.NodeID, Remark: in.Remark, Enabled: true, Port: in.Port,
		Protocol: "vless", Transport: "tcp", Security: "reality",
		Reality: &node.Reality{
			Target:        in.Target,
			ServerNames:   []string{host},
			PrivateKey:    priv,
			PublicKey:     pub,
			ShortIDs:      []string{sid},
			Fingerprint:   node.DefaultFingerprint,
			FallbackLimit: node.DefaultFallbackLimit,
		},
		Clients: []node.Client{{
			ID: in.ClientID, Remark: in.Remark, UUID: uuid, Flow: node.FlowVision, Enabled: true,
		}},
	}, nil
}
