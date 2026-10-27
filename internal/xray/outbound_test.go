package xray

import (
	"encoding/json"
	"testing"

	"github.com/Aspeternity/litenode/internal/node/builtin"
)

func TestGenerateConfigOutboundGolden(t *testing.T) {
	reg, _ := builtin.Registry()
	for mode, name := range map[string]string{
		OutboundDefault:    "xray-empty.golden.json",
		OutboundPreferIPv4: "xray-outbound-prefer-ipv4.golden.json",
		OutboundPreferIPv6: "xray-outbound-prefer-ipv6.golden.json",
		OutboundIPv4Only:   "xray-outbound-ipv4-only.golden.json",
		OutboundIPv6Only:   "xray-outbound-ipv6-only.golden.json",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := GenerateConfig(reg, nil, mode)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(got) {
				t.Fatal("invalid JSON")
			}
			checkGolden(t, name, got)
		})
	}
	if _, err := GenerateConfig(reg, nil, "both"); err == nil {
		t.Error("unknown mode accepted")
	}
}

func TestOnlyIPv6(t *testing.T) {
	for mode, want := range map[string][2]bool{
		OutboundDefault: {false, false}, OutboundPreferIPv4: {false, false}, OutboundPreferIPv6: {false, false},
		OutboundIPv4Only: {false, true}, OutboundIPv6Only: {true, true},
	} {
		v6, ok := OnlyIPv6(mode)
		if v6 != want[0] || ok != want[1] {
			t.Errorf("OnlyIPv6(%q) = %v, %v", mode, v6, ok)
		}
	}
}

func TestOutboundCheck(t *testing.T) {
	cases := []struct {
		mode            string
		v4, v6          bool
		refuse, confirm bool
	}{
		{OutboundIPv4Only, false, true, true, false},
		{OutboundIPv4Only, true, true, false, false},
		{OutboundPreferIPv4, false, true, false, true},
		{OutboundIPv6Only, true, false, true, false},
		{OutboundPreferIPv6, true, false, false, true},
		{OutboundDefault, false, true, false, false},
		{OutboundIPv4Only, false, false, false, false},
	}
	for _, c := range cases {
		r, cf := OutboundCheck(c.mode, c.v4, c.v6)
		if r != c.refuse || cf != c.confirm {
			t.Errorf("OutboundCheck(%q, %v, %v) = %v, %v", c.mode, c.v4, c.v6, r, cf)
		}
	}
}
