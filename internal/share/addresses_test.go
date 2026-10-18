package share

import (
	"context"
	"net"
	"slices"
	"testing"
)

func TestAddressKindAndRemark(t *testing.T) {
	for addr, want := range map[string]string{"203.0.113.10": KindIPv4, "2001:db8::1": KindIPv6, "::ffff:1.2.3.4": KindIPv4, "la.example.com": ""} {
		if got := AddressKind(addr); got != want {
			t.Errorf("AddressKind(%s) = %q", addr, got)
		}
	}
	if got := LinkRemark("phone", "203.0.113.10", true); got != "phone" {
		t.Errorf("default: %s", got)
	}
	if got := LinkRemark("phone", "2001:db8::1", false); got != "phone-IPv6" {
		t.Errorf("ipv6: %s", got)
	}
	if got := LinkRemark("phone", "la.example.com", false); got != "phone-la.example.com" {
		t.Errorf("domain: %s", got)
	}
}

func TestMergeDetected(t *testing.T) {
	for _, tc := range []struct {
		list, detected, want []string
	}{
		{nil, []string{"1.1.1.1", "2001:db8::1"}, []string{"1.1.1.1", "2001:db8::1"}},
		// The default domain stays first; old IPs are replaced.
		{[]string{"la.example.com", "9.9.9.9"}, []string{"1.1.1.1"}, []string{"la.example.com", "1.1.1.1"}},
		// A default IPv6 still present stays the default.
		{[]string{"2001:db8::1", "1.1.1.1"}, []string{"1.1.1.1", "2001:db8::1"}, []string{"2001:db8::1", "1.1.1.1"}},
		// A default IP no longer detected gives way to the new IPv4.
		{[]string{"9.9.9.9", "a.example.com"}, []string{"1.1.1.1"}, []string{"1.1.1.1", "a.example.com"}},
	} {
		if got := MergeDetected(tc.list, tc.detected); !slices.Equal(got, tc.want) {
			t.Errorf("MergeDetected(%v, %v) = %v, want %v", tc.list, tc.detected, got, tc.want)
		}
	}
}

type fakeResolver map[string][]string

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	var out []net.IPAddr
	for _, s := range f[host] {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	if out == nil {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return out, nil
}

func TestCheckDomain(t *testing.T) {
	r := fakeResolver{"here.example.com": {"203.0.113.10", "2001:db8::1"}, "cdn.example.com": {"104.16.1.1"}}
	own := []string{"203.0.113.10", "2001:db8::1"}
	c, err := CheckDomain(context.Background(), r, "here.example.com", own)
	if err != nil || !c.Here || len(c.Resolved) != 2 {
		t.Errorf("here: %+v %v", c, err)
	}
	if c, _ := CheckDomain(context.Background(), r, "cdn.example.com", own); c.Here {
		t.Errorf("cdn: %+v", c)
	}
	if _, err := CheckDomain(context.Background(), r, "none.example.com", own); err == nil {
		t.Error("unresolvable domain accepted")
	}
}
