package builtin

import (
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
)

func TestBuiltinRegistry(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	presets := r.Presets()
	if len(presets) != 1 || presets[0].ID != PresetVLESSRealityVision {
		t.Fatalf("presets = %+v", presets)
	}
	for _, name := range []string{"vless"} {
		if _, ok := r.Protocol(name); !ok {
			t.Errorf("protocol %s missing", name)
		}
	}
	if _, ok := r.Transport("tcp"); !ok {
		t.Error("transport tcp missing")
	}
	if _, ok := r.Security("reality"); !ok {
		t.Error("security reality missing")
	}
	if err := r.Check(node.Combo{Protocol: "vless", Transport: "tcp", Security: "reality", Flow: node.FlowVision}); err != nil {
		t.Errorf("preset combo rejected: %v", err)
	}
}
