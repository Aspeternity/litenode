package builtin

import (
	"fmt"
	"net"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/protocol"
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

// buildVLESSEncVision builds a VLESS Encryption node (DESIGN section 9.7):
// an X25519 authentication key pair and the first client with Vision.
func buildVLESSEncVision(in node.BuildInput) (node.Node, error) {
	priv, pub, err := security.NewX25519(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	uuid, err := node.NewUUID(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	return node.Node{
		ID: in.NodeID, Remark: in.Remark, Enabled: true, Port: in.Port,
		Protocol: "vless", Transport: "tcp", Security: "none",
		VLESSEncryption: &node.VLESSEncryption{PrivateKey: priv, PublicKey: pub},
		Clients: []node.Client{{
			ID: in.ClientID, Remark: in.Remark, UUID: uuid, Flow: node.FlowVision, Enabled: true,
		}},
	}, nil
}

// buildSS2022 builds a Shadowsocks 2022 node (DESIGN section 9.8): a
// server key and the first client with its own user key.
func buildSS2022(in node.BuildInput) (node.Node, error) {
	serverKey, err := protocol.NewSS2022Key(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	userKey, err := protocol.NewSS2022Key(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	return node.Node{
		ID: in.NodeID, Remark: in.Remark, Enabled: true, Port: in.Port,
		Protocol: "shadowsocks", Transport: "tcp", Security: "none",
		Shadowsocks: &node.Shadowsocks{Method: protocol.SS2022Method, ServerKey: serverKey},
		Clients: []node.Client{{
			ID: in.ClientID, Remark: in.Remark, Key: userKey, Enabled: true,
		}},
	}, nil
}

// buildHysteria2 builds a Hysteria2 node (DESIGN section 9.10): a new
// self-signed certificate and the first client with its own password.
func buildHysteria2(in node.BuildInput) (node.Node, error) {
	t, err := security.NewTLS(in.Rand, in.Now)
	if err != nil {
		return node.Node{}, err
	}
	pw, err := protocol.NewHysteriaPassword(in.Rand)
	if err != nil {
		return node.Node{}, err
	}
	return node.Node{
		ID: in.NodeID, Remark: in.Remark, Enabled: true, Port: in.Port,
		Protocol: "hysteria", Transport: "hysteria", Security: "tls",
		TLS: t,
		Clients: []node.Client{{
			ID: in.ClientID, Remark: in.Remark, Key: pw, Enabled: true,
		}},
	}, nil
}
