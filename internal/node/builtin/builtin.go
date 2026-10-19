// Package builtin assembles the compiled-in node components, the
// compatibility table and the presets. Adding a protocol, transport or
// security layer means adding its file and registering it here; the menu
// lists presets from the registry without changes.
package builtin

import (
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/protocol"
	"github.com/Aspeternity/litenode/internal/node/security"
	"github.com/Aspeternity/litenode/internal/node/transport"
)

// Preset IDs.
const (
	PresetVLESSRealityVision = "vless-tcp-reality-vision"
	// PresetVLESSEncVision is VLESS + TCP + VLESS Encryption (D-106).
	PresetVLESSEncVision = "vless-tcp-encryption-vision"
	// PresetSS2022 is Shadowsocks 2022 (D-110).
	PresetSS2022 = "shadowsocks-2022"
	// PresetHysteria2 is Hysteria2 with a self-signed certificate (D-117).
	PresetHysteria2 = "hysteria2"
)

// Registry returns the registry of all compiled-in components.
func Registry() (*node.Registry, error) {
	r := node.NewRegistry()
	steps := []func() error{
		func() error { return r.AddProtocol(protocol.VLESS{}) },
		func() error { return r.AddProtocol(protocol.Shadowsocks{}) },
		func() error { return r.AddProtocol(protocol.Hysteria{}) },
		func() error { return r.AddTransport(transport.TCP{}) },
		func() error { return r.AddTransport(transport.Hysteria{}) },
		func() error { return r.AddSecurity(security.Reality{}) },
		func() error { return r.AddSecurity(security.None{}) },
		func() error { return r.AddSecurity(security.TLS{}) },

		// Vision requires VLESS over raw TCP with REALITY or TLS.
		func() error {
			return r.AddCompat(node.Compat{Protocol: "vless", Transport: "tcp", Security: "reality",
				Flows: []string{node.FlowVision, ""}})
		},
		// Security "none" requires VLESS Encryption (Registry.CheckNode).
		func() error {
			return r.AddCompat(node.Compat{Protocol: "vless", Transport: "tcp", Security: "none",
				Flows: []string{node.FlowVision, ""}})
		},
		func() error {
			return r.AddCompat(node.Compat{Protocol: "shadowsocks", Transport: "tcp", Security: "none",
				Flows: []string{""}})
		},
		// The hysteria protocol only runs over its own transport, which
		// always uses TLS.
		func() error {
			return r.AddCompat(node.Compat{Protocol: "hysteria", Transport: "hysteria", Security: "tls",
				Flows: []string{""}})
		},

		func() error {
			return r.AddPreset(node.Preset{
				ID:    PresetVLESSRealityVision,
				Label: "VLESS + TCP + Reality (Vision)",
				Combo: node.Combo{Protocol: "vless", Transport: "tcp", Security: "reality", Flow: node.FlowVision},
				Build: buildVLESSRealityVision,
			})
		},
		func() error {
			return r.AddPreset(node.Preset{
				ID:    PresetVLESSEncVision,
				Label: "VLESS + TCP + Encryption (Vision)",
				Combo: node.Combo{Protocol: "vless", Transport: "tcp", Security: "none", Flow: node.FlowVision},
				Build: buildVLESSEncVision,
			})
		},
		func() error {
			return r.AddPreset(node.Preset{
				ID:    PresetSS2022,
				Label: "Shadowsocks 2022",
				Combo: node.Combo{Protocol: "shadowsocks", Transport: "tcp", Security: "none"},
				Build: buildSS2022,
			})
		},
		func() error {
			return r.AddPreset(node.Preset{
				ID:    PresetHysteria2,
				Label: "Hysteria2",
				Combo: node.Combo{Protocol: "hysteria", Transport: "hysteria", Security: "tls"},
				Build: buildHysteria2,
			})
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return r, nil
}
