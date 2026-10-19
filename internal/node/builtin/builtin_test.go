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
	if len(presets) != 4 || presets[0].ID != PresetVLESSRealityVision || presets[1].ID != PresetVLESSEncVision ||
		presets[2].ID != PresetSS2022 || presets[3].ID != PresetHysteria2 {
		t.Fatalf("presets = %+v", presets)
	}
	for _, name := range []string{"vless", "shadowsocks", "hysteria"} {
		if _, ok := r.Protocol(name); !ok {
			t.Errorf("protocol %s missing", name)
		}
	}
	for _, name := range []string{"tcp", "hysteria"} {
		if _, ok := r.Transport(name); !ok {
			t.Errorf("transport %s missing", name)
		}
	}
	for _, name := range []string{"reality", "none", "tls"} {
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
