package egress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"testing"
)

func addrs(s ...string) []netip.Addr {
	var out []netip.Addr
	for _, a := range s {
		out = append(out, netip.MustParseAddr(a))
	}
	return out
}

func TestNAT64Prefix(t *testing.T) {
	for _, c := range []struct {
		in   []netip.Addr
		want string
	}{
		{addrs("64:ff9b::c000:aa", "64:ff9b::c000:ab"), "64:ff9b::/96"},
		{addrs("2001:db8:1:2:0:0:c000:ab"), "2001:db8:1:2::/96"},
		{addrs("2001:db8::1"), ""},
		{addrs("::ffff:192.0.0.170", "192.0.0.170"), ""}, // no DNS64: mapped or plain IPv4
		{nil, ""},
	} {
		p, ok := NAT64Prefix(c.in)
		if got := ""; ok {
			got = p.String()
			if got != c.want {
				t.Errorf("NAT64Prefix(%v) = %s, want %q", c.in, got, c.want)
			}
		} else if c.want != "" {
			t.Errorf("NAT64Prefix(%v) found nothing, want %s", c.in, c.want)
		}
	}
	if got := Synthesize(netip.MustParsePrefix("64:ff9b::/96"), netip.MustParseAddr("140.82.112.3")); got.String() != "64:ff9b::8c52:7003" {
		t.Errorf("Synthesize = %s", got)
	}
}

// dialRecorder records the addresses dialed and fails every dial.
type dialRecorder struct{ dialed []string }

func (r *dialRecorder) dial(_ context.Context, _, addr string) (net.Conn, error) {
	r.dialed = append(r.dialed, addr)
	return nil, errors.New("refused")
}

func newTestDialer(st Status, dns map[string][]netip.Addr) (*Dialer, *dialRecorder) {
	rec := &dialRecorder{}
	return &Dialer{
		Status: func(context.Context) Status { return st },
		Lookup: func(_ context.Context, network, host string) ([]netip.Addr, error) {
			var out []netip.Addr
			for _, a := range dns[host] {
				if network == "ip" || (network == "ip6") == a.Is6() {
					out = append(out, a)
				}
			}
			if len(out) == 0 {
				return nil, errors.New("no such host")
			}
			return out, nil
		},
		Dial: rec.dial,
	}, rec
}

func TestDialerNAT64(t *testing.T) {
	v6only := Status{IPv6: Family{OK: true}}
	dns := map[string][]netip.Addr{
		"github.com":    addrs("140.82.112.3", "140.82.112.4"),
		"ipv4only.arpa": addrs("64:ff9b::c000:aa", "192.0.0.170"),
		"dual.example":  addrs("192.0.2.1", "2001:db8::1"),
	}
	d, rec := newTestDialer(v6only, dns)
	d.DialContext(t.Context(), "tcp", "github.com:443")
	if want := []string{"[64:ff9b::8c52:7003]:443", "[64:ff9b::8c52:7004]:443"}; !slices.Equal(rec.dialed, want) {
		t.Errorf("IPv4-only host: dialed %v, want %v", rec.dialed, want)
	}
	// A host with IPv6 is dialed by name; an IPv4 literal is mapped.
	rec.dialed = nil
	d.DialContext(t.Context(), "tcp", "dual.example:443")
	d.DialContext(t.Context(), "tcp", "198.51.100.7:80")
	if want := []string{"dual.example:443", "[64:ff9b::c633:6407]:80"}; !slices.Equal(rec.dialed, want) {
		t.Errorf("dialed %v, want %v", rec.dialed, want)
	}

	// IPv4 exit (WARP counts): always direct.
	d, rec = newTestDialer(Status{IPv4: Family{OK: true, WARP: true}, IPv6: Family{OK: true}}, dns)
	d.DialContext(t.Context(), "tcp", "github.com:443")
	if want := []string{"github.com:443"}; !slices.Equal(rec.dialed, want) {
		t.Errorf("with IPv4: dialed %v", rec.dialed)
	}

	// No NAT64: a clear error, nothing dialed.
	delete(dns, "ipv4only.arpa")
	d, rec = newTestDialer(v6only, dns)
	if _, err := d.DialContext(t.Context(), "tcp", "github.com:443"); !errors.Is(err, ErrNoIPv4) || len(rec.dialed) != 0 {
		t.Errorf("no NAT64: err = %v, dialed %v", err, rec.dialed)
	}
}
