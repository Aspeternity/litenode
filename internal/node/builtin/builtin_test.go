package builtin

import (
	"testing"
)

func TestBuiltinRegistry(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	presets := r.Presets()
	if len(presets) != 2 || presets[0].ID != PresetVLESSRealityVision || presets[1].ID != PresetVLESSEncVision {
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
	for _, name := range []string{"reality", "none"} {
		if _, ok := r.Security(name); !ok {
			t.Errorf("security %s missing", name)
		}
	}
	for _, p := range presets {
		if err := r.Check(p.Combo); err != nil {
			t.Errorf("preset %s combo rejected: %v", p.ID, err)
		}
	}
}
