package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
)

// NAT64 discovery (RFC 7050): a DNS64 resolver answers the IPv4-only name
// ipv4only.arpa with AAAA records that embed its well-known addresses.
const nat64Name = "ipv4only.arpa"

var wellKnown = []netip.Addr{netip.MustParseAddr("192.0.0.170"), netip.MustParseAddr("192.0.0.171")}

// NAT64Prefix finds the /96 NAT64 prefix in the AAAA answers for
// ipv4only.arpa. Other prefix lengths are not used by providers in
// practice and are not recognized (D-131).
func NAT64Prefix(answers []netip.Addr) (netip.Prefix, bool) {
	for _, a := range answers {
		if !a.Is6() || a.Is4In6() {
			continue
		}
		b := a.As16()
		v4 := netip.AddrFrom4([4]byte(b[12:]))
		for _, w := range wellKnown {
			if v4 == w {
				p, _ := a.Prefix(96)
				return p, true
			}
		}
	}
	return netip.Prefix{}, false
}

// Synthesize embeds an IPv4 address in a /96 NAT64 prefix.
func Synthesize(prefix netip.Prefix, v4 netip.Addr) netip.Addr {
	b := prefix.Addr().As16()
	copy(b[12:], v4.AsSlice())
	return netip.AddrFrom16(b)
}

// Dialer connects through the provider's NAT64 when this host has no IPv4
// exit, so downloads from IPv4-only hosts such as github.com still work
// (DESIGN section 19.3). It changes nothing on the system; TLS still runs
// end to end with the original host name.
type Dialer struct {
	// Status returns the exits (Prober.Status).
	Status func(ctx context.Context) Status
	// Lookup resolves a host name ("ip": both families).
	Lookup func(ctx context.Context, network, host string) ([]netip.Addr, error)
	// Dial connects; nil uses net.Dialer.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	mu     sync.Mutex
	probed bool
	prefix netip.Prefix
}

// NewDialer returns a Dialer using the system resolver.
func NewDialer(status func(ctx context.Context) Status) *Dialer {
	return &Dialer{Status: status, Lookup: net.DefaultResolver.LookupNetIP}
}

// ErrNoIPv4 means the host has only IPv4 addresses and this machine has
// neither an IPv4 exit nor a NAT64 prefix.
var ErrNoIPv4 = errors.New("no IPv4 exit and no NAT64: host has only IPv4 addresses")

func (d *Dialer) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.Dial != nil {
		return d.Dial(ctx, network, addr)
	}
	var nd net.Dialer
	return nd.DialContext(ctx, network, addr)
}

// nat64 returns the NAT64 prefix, discovering it on first use.
func (d *Dialer) nat64(ctx context.Context) (netip.Prefix, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.probed {
		d.probed = true
		if as, err := d.Lookup(ctx, "ip6", nat64Name); err == nil {
			d.prefix, _ = NAT64Prefix(as)
		}
	}
	return d.prefix, d.prefix.IsValid()
}

// DialContext dials addr directly unless this host lacks an IPv4 exit
// but has IPv6 and the host name has no IPv6 address: then it dials the
// NAT64 addresses of its IPv4 addresses.
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	st := d.Status(ctx)
	if st.IPv4.OK || !st.IPv6.OK {
		return d.dial(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var v4 []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		v4 = []netip.Addr{ip}
	} else {
		ips, err := d.Lookup(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if ip = ip.Unmap(); ip.Is6() {
				return d.dial(ctx, network, addr) // reachable over IPv6
			}
			v4 = append(v4, ip)
		}
	}
	if len(v4) == 0 || !v4[0].Unmap().Is4() {
		return d.dial(ctx, network, addr)
	}
	prefix, ok := d.nat64(ctx)
	if !ok {
		return nil, fmt.Errorf("dial %s: %w", addr, ErrNoIPv4)
	}
	var last error
	for _, ip := range v4 {
		c, err := d.dial(ctx, network, net.JoinHostPort(Synthesize(prefix, ip.Unmap()).String(), port))
		if err == nil {
			return c, nil
		}
		last = err
	}
	return nil, last
}
