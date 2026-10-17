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
)

// Registry returns the registry of all compiled-in components.
func Registry() (*node.Registry, error) {
	r := node.NewRegistry()
	steps := []func() error{
		func() error { return r.AddProtocol(protocol.VLESS{}) },
		func() error { return r.AddTransport(transport.TCP{}) },
		func() error { return r.AddSecurity(security.Reality{}) },

		// Vision requires VLESS over raw TCP with REALITY or TLS.
		func() error {
			return r.AddCompat(node.Compat{Protocol: "vless", Transport: "tcp", Security: "reality",
				Flows: []string{node.FlowVision, ""}})
		},

		func() error {
			return r.AddPreset(node.Preset{
				ID:          PresetVLESSRealityVision,
				Label:       "VLESS + TCP + Reality (Vision)",
				Combo:       node.Combo{Protocol: "vless", Transport: "tcp", Security: "reality", Flow: node.FlowVision},
				Recommended: true,
				Build:       buildVLESSRealityVision,
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
