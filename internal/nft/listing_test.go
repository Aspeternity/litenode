package nft

import (
	"fmt"
	"testing"
)

// listing is what nft 1.0.6 (Debian 12) printed for a table written by
// Generate, with sources nft merged.
const listing = `table inet litenode {
	chain input {
		type filter hook input priority filter; policy drop;
		ct state established,related accept
		icmpv6 type { mld-listener-query, nd-router-solicit, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } accept
		ip saddr 1.2.3.4 tcp dport 443 accept comment "litenode:node:3"
		ip saddr { 1.2.3.4/31, 10.0.0.0/24 } udp dport 10000-10100 accept comment "litenode:manual:5"
	}
}
`

func TestParseChain(t *testing.T) {
	c, err := ParseChain(listing)
	if err != nil {
		t.Fatal(err)
	}
	if c.Policy != "drop" || len(c.Rules) != 4 || c.Rules[0] != "ct state established,related accept" {
		t.Errorf("ParseChain = %+v", c)
	}
	if _, err := ParseChain("Error: No such file or directory"); err == nil {
		t.Error("no chain: want error")
	}
}

func TestParseRuleMatchesNftOutput(t *testing.T) {
	for _, tc := range []struct{ written, printed string }{
		{"icmpv6 type { nd-neighbor-solicit, nd-neighbor-advert, nd-router-solicit, nd-router-advert, mld-listener-query } accept",
			"icmpv6 type { mld-listener-query, nd-router-solicit, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } accept"},
		{`ip saddr { 1.2.3.4 } tcp dport 443 accept comment "litenode:node:3"`, `ip saddr 1.2.3.4 tcp dport 443 accept comment "litenode:node:3"`},
		{`ip saddr { 10.0.0.0/25, 1.2.3.5, 10.0.0.128/25, 1.2.3.4 } udp dport 10000-10100 accept comment "litenode:manual:5"`,
			`ip saddr { 1.2.3.4/31, 10.0.0.0/24 } udp dport 10000-10100 accept comment "litenode:manual:5"`},
		{`ip6 saddr { 2001:db8::/32, ::1 } meta l4proto { tcp, udp } th dport 28388 accept comment "litenode:manual:5"`,
			`ip6 saddr { ::1, 2001:db8::/32 } meta l4proto { tcp, udp } th dport 28388 accept comment "litenode:manual:5"`},
	} {
		w, err := ParseRule(tc.written)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ParseRule(tc.printed)
		if err != nil {
			t.Fatal(err)
		}
		if !w.Equal(p) {
			t.Errorf("%q != %q:\n%+v\n%+v", tc.written, tc.printed, w, p)
		}
	}
	a, _ := ParseRule(`ip saddr 1.2.3.4 tcp dport 443 accept comment "litenode:node:3"`)
	for _, other := range []string{
		`ip saddr 1.2.3.5 tcp dport 443 accept comment "litenode:node:3"`,
		`ip saddr 1.2.3.4 tcp dport 444 accept comment "litenode:node:3"`,
		`ip saddr 1.2.3.4 tcp dport 443 accept comment "litenode:node:4"`,
		`tcp dport 443 accept comment "litenode:node:3"`,
	} {
		if b, _ := ParseRule(other); a.Equal(b) {
			t.Errorf("%q equals %q", other, "ip saddr 1.2.3.4 ...")
		}
	}
}

func TestNormalizeSources(t *testing.T) {
	got, err := NormalizeSources([]string{"2001:db8::1", "10.1.0.0/16", "1.2.3.4", "10.0.0.0/8", "1.2.3.4", "::ffff:5.6.7.8"})
	if err != nil {
		t.Fatal(err)
	}
	want := "[1.2.3.4 5.6.7.8 10.0.0.0/8 2001:db8::1]"
	if s := fmt.Sprint(got); s != want {
		t.Errorf("NormalizeSources = %s, want %s", s, want)
	}
	if _, err := NormalizeSources([]string{"1.2.3"}); err == nil {
		t.Error("invalid source accepted")
	}
}
