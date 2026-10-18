package xray

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
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

func TestParseFamilyReady(t *testing.T) {
	const route4 = "default via 62.84.164.1 dev eth0 proto static\n"
	const route6 = "default via fe80::1 dev eth0 metric 1024 pref medium\n"
	const addr4 = "2: eth0    inet 62.84.164.181/24 brd 62.84.164.255 scope global eth0\\       valid_lft forever preferred_lft forever\n"
	const addr4NAT = "2: eth0    inet 10.0.0.5/24 brd 10.0.0.255 scope global eth0\n"
	const addr6 = "2: eth0    inet6 2a0e:1234::5/64 scope global \\       valid_lft forever preferred_lft forever\n"
	const addr6ULA = "2: eth0    inet6 fd00::5/64 scope global \n"
	for _, c := range []struct {
		name        string
		route, addr string
		ipv6, want  bool
	}{
		{"ipv4", route4, addr4, false, true},
		{"ipv4 behind NAT", route4, addr4NAT, false, true},
		{"ipv4 no route", "", addr4, false, false},
		{"ipv4 no address", route4, "", false, false},
		{"ipv6", route6, addr6, true, true},
		{"ipv6 no route", "", addr6, true, false},
		{"ipv6 unique local only", route6, addr6ULA, true, false},
		{"ipv6 asked, ipv4 listed", route6, addr4, true, false},
	} {
		if got := ParseFamilyReady(c.route, c.addr, c.ipv6); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFamilyReadyCommands(t *testing.T) {
	r := systemtest.New()
	r.Respond(systemtest.Response{Result: system.Result{Stdout: "default via fe80::1 dev eth0\n"}}, "ip", "-6", "route", "show", "default")
	r.Respond(systemtest.Response{Result: system.Result{Stdout: "2: eth0    inet6 2a0e:1234::5/64 scope global\n"}}, "ip", "-o", "-6", "addr", "show", "scope", "global")
	ok, err := FamilyReady(context.Background(), r, true)
	if err != nil || !ok {
		t.Fatalf("FamilyReady = %v, %v", ok, err)
	}
}
