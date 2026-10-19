package nft

import (
	"net/netip"
	"os"
	"slices"
	"testing"
)

func TestGenerateProbeGolden(t *testing.T) {
	for name, ps := range map[string][]ProbePort{
		"probe-tcp":     {{"tcp", 443}},
		"probe-tcp-udp": {{"tcp", 42001}, {"udp", 42001}},
	} {
		got, err := GenerateProbe(ps)
		if err != nil {
			t.Fatal(err)
		}
		golden(t, name, got)
	}
}

func TestGenerateProbeRejects(t *testing.T) {
	for _, ps := range [][]ProbePort{
		nil,
		{{"icmp", 1}},
		{{"tcp", 0}},
		{{"udp", 65536}},
		{{"tcp", 1}, {"tcp", 2}},
	} {
		if _, err := GenerateProbe(ps); err == nil {
			t.Errorf("%v: no error", ps)
		}
	}
}

func TestParseProbe(t *testing.T) {
	// Captured on the test machines (DESIGN 9.9): nft 0.9.8 on Debian 11
	// with no traffic, nft 1.1.3 on Debian 13 after one IPv4 and one IPv6
	// connection to port 22.
	for _, tc := range []struct {
		file string
		want ProbeCount
	}{
		{"probe-0.9.8.json", ProbeCount{}},
		{"probe-1.1.3.json", ProbeCount{Arrived: 10, Passed: 10, Sources: []netip.Addr{
			netip.MustParseAddr("62.84.164.181"), netip.MustParseAddr("2401:1da0:22:d3::a")}}},
	} {
		data, err := os.ReadFile("testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseProbe(data)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if len(got) != 1 {
			t.Fatalf("%s: got %v", tc.file, got)
		}
		g := got["tcp"]
		if g.Arrived != tc.want.Arrived || g.Passed != tc.want.Passed || !slices.Equal(g.Sources, tc.want.Sources) {
			t.Errorf("%s: got %+v, want %+v", tc.file, g, tc.want)
		}
	}
}

func TestParseProbeElemObjectsAndUDP(t *testing.T) {
	data := `{"nftables": [
{"counter": {"family": "inet", "name": "arrive_udp", "table": "litenode-probe", "packets": 7}},
{"counter": {"family": "inet", "name": "pass_udp", "table": "litenode-probe", "packets": 0}},
{"counter": {"family": "inet", "name": "arrive_udp", "table": "other", "packets": 99}},
{"set": {"family": "inet", "name": "src_udp6", "table": "litenode-probe", "elem": [{"elem": {"val": "2001:db8::1", "expires": 3}}]}},
{"set": {"family": "inet", "name": "src_udp4", "table": "litenode-probe", "elem": ["192.0.2.1"]}}
]}`
	got, err := ParseProbe([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	u := got["udp"]
	want := []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")}
	if u.Arrived != 7 || u.Passed != 0 || !slices.Equal(u.Sources, want) {
		t.Errorf("got %+v", u)
	}
	if _, err := ParseProbe([]byte(`{"nftables": [{"set": {"name": "src_tcp4", "table": "litenode-probe", "elem": ["x"]}}]}`)); err == nil {
		t.Error("bad element: no error")
	}
	if _, err := ParseProbe([]byte(`not json`)); err == nil {
		t.Error("bad json: no error")
	}
}
