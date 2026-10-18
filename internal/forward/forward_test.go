package forward

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestCheck(t *testing.T) {
	ok := Forward{Protocol: TCPUDP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"}
	if err := Check(ok); err != nil {
		t.Fatal(err)
	}
	for want, fs := range map[error][]Forward{
		ErrProtocol:    {{Protocol: "icmp", ListenPort: "1", TargetHost: "1.2.3.4", TargetPort: "1"}},
		ErrPort:        {{Protocol: TCP, ListenPort: "0", TargetHost: "1.2.3.4", TargetPort: "1"}, {Protocol: TCP, ListenPort: "10-5", TargetHost: "1.2.3.4", TargetPort: "1"}, {Protocol: TCP, ListenPort: "1", TargetHost: "1.2.3.4", TargetPort: "65536"}},
		ErrPortLength:  {{Protocol: TCP, ListenPort: "30000-30002", TargetHost: "1.2.3.4", TargetPort: "443"}, {Protocol: TCP, ListenPort: "30000-30002", TargetHost: "1.2.3.4", TargetPort: "40000-40003"}},
		ErrTargetHost:  {{Protocol: TCP, ListenPort: "1", TargetHost: "no space", TargetPort: "1"}, {Protocol: TCP, ListenPort: "1", TargetHost: "", TargetPort: "1"}},
		ErrNoAddresses: {{Protocol: TCP, ListenPort: "1", TargetHost: "la.example.com", TargetPort: "1"}},
	} {
		for _, f := range fs {
			if err := Check(f); !errors.Is(err, want) {
				t.Errorf("Check(%+v) = %v, want %v", f, err, want)
			}
		}
	}
	if err := CheckFields(Forward{Protocol: TCP, ListenPort: "1", TargetHost: "la.example.com", TargetPort: "1"}); err != nil {
		t.Errorf("CheckFields of an unresolved domain: %v", err)
	}
}

func TestAddresses(t *testing.T) {
	for _, c := range []struct {
		f      Forward
		v4, v6 string
	}{
		{Forward{TargetHost: "203.0.113.10"}, "203.0.113.10", ""},
		{Forward{TargetHost: "::ffff:203.0.113.10"}, "203.0.113.10", ""},
		{Forward{TargetHost: "2001:db8::10"}, "", "2001:db8::10"},
		{Forward{TargetHost: "la.example.com", ResolvedIPv4: "203.0.113.10", ResolvedIPv6: "2001:db8::10"}, "203.0.113.10", "2001:db8::10"},
		{Forward{TargetHost: "la.example.com", ResolvedIPv6: "2001:db8::10"}, "", "2001:db8::10"},
	} {
		if c.f.IPv4() != c.v4 || c.f.IPv6() != c.v6 {
			t.Errorf("%s: %q %q, want %q %q", c.f.TargetHost, c.f.IPv4(), c.f.IPv6(), c.v4, c.v6)
		}
	}
}

func TestNormalizeRemark(t *testing.T) {
	if r, err := NormalizeRemark("  "); r != "" || err != nil {
		t.Errorf("empty remark: %q %v", r, err)
	}
	if r, err := NormalizeRemark(" LA "); r != "LA" || err != nil {
		t.Errorf("remark: %q %v", r, err)
	}
	if _, err := NormalizeRemark("a\tb"); err == nil {
		t.Error("accepted control characters")
	}
}

type fakeResolver map[string][]net.IP

func (r fakeResolver) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	if ips, ok := r[network+" "+host]; ok {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func TestResolve(t *testing.T) {
	ctx := context.Background()
	r := fakeResolver{
		"ip4 both.example.com": {net.ParseIP("203.0.113.1"), net.ParseIP("203.0.113.2")},
		"ip6 both.example.com": {net.ParseIP("2001:db8::1")},
		"ip4 v4.example.com":   {net.ParseIP("203.0.113.9")},
	}
	if v4, v6, err := Resolve(ctx, r, "both.example.com", "", ""); v4 != "203.0.113.1" || v6 != "2001:db8::1" || err != nil {
		t.Errorf("both: %q %q %v", v4, v6, err)
	}
	// The address in use stays while the domain still has it.
	if v4, _, _ := Resolve(ctx, r, "both.example.com", "203.0.113.2", ""); v4 != "203.0.113.2" {
		t.Errorf("current address replaced by %q", v4)
	}
	if v4, v6, err := Resolve(ctx, r, "v4.example.com", "", "2001:db8::9"); v4 != "203.0.113.9" || v6 != "" || err != nil {
		t.Errorf("A only: %q %q %v", v4, v6, err)
	}
	if _, _, err := Resolve(ctx, r, "none.example.com", "", ""); err == nil {
		t.Error("resolved a domain without records")
	}
}

// failingResolver fails the lookups of families in fail; others go to r.
type failingResolver struct {
	r    fakeResolver
	fail map[string]error
}

func (f failingResolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	if err, ok := f.fail[network]; ok {
		return nil, err
	}
	return f.r.LookupIP(ctx, network, host)
}

func TestReresolve(t *testing.T) {
	ctx := context.Background()
	r := fakeResolver{
		"ip4 both.example.com": {net.ParseIP("203.0.113.1")},
		"ip6 both.example.com": {net.ParseIP("2001:db8::1")},
		"ip4 v4.example.com":   {net.ParseIP("203.0.113.9")},
	}
	timeout := &net.DNSError{Err: "i/o timeout", Name: "both.example.com", IsTimeout: true}
	for _, c := range []struct {
		name             string
		r                Resolver
		host, cur4, cur6 string
		v4, v6           string
		err              bool
	}{
		{"changed", r, "both.example.com", "203.0.113.5", "2001:db8::5", "203.0.113.1", "2001:db8::1", false},
		{"AAAA removed", r, "v4.example.com", "203.0.113.9", "2001:db8::9", "203.0.113.9", "", false},
		{"AAAA lookup fails", failingResolver{r, map[string]error{"ip6": timeout}}, "both.example.com", "203.0.113.5", "2001:db8::5", "203.0.113.1", "2001:db8::5", true},
		{"no suitable address", failingResolver{r, map[string]error{"ip6": &net.AddrError{Err: "no suitable address found", Addr: "v4.example.com"}}}, "v4.example.com", "203.0.113.9", "2001:db8::9", "203.0.113.9", "", false},
		{"both fail", failingResolver{r, map[string]error{"ip4": timeout, "ip6": timeout}}, "both.example.com", "203.0.113.5", "", "203.0.113.5", "", true},
		{"domain gone", r, "none.example.com", "203.0.113.5", "2001:db8::5", "203.0.113.5", "2001:db8::5", true},
	} {
		v4, v6, err := Reresolve(ctx, c.r, c.host, c.cur4, c.cur6)
		if v4 != c.v4 || v6 != c.v6 || (err != nil) != c.err {
			t.Errorf("%s: %q %q %v", c.name, v4, v6, err)
		}
	}
}
