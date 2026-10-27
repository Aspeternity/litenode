package share

import (
	"context"
	"net"
	"net/netip"

	"github.com/Aspeternity/litenode/internal/egress"
)

// Source tells where detected addresses came from.
type Source int

const (
	SourceNone Source = iota
	SourceTrace
	SourceInterface
)

// Detected is the result of public address detection.
type Detected struct {
	IPv4, IPv6 string
	Source     Source
}

// Candidates returns the detected addresses, IPv4 first (the default
// choice when both exist).
func (d Detected) Candidates() []string {
	var out []string
	for _, a := range []string{d.IPv4, d.IPv6} {
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

// Detector finds the public address. Fields are replaceable in tests.
type Detector struct {
	// Egress returns the detected exits (egress.Prober.Status).
	Egress func(ctx context.Context) egress.Status
	// InterfaceAddrs lists the addresses on the network interfaces.
	InterfaceAddrs func() ([]net.Addr, error)
}

// NewDetector returns a Detector using the given exit detection.
func NewDetector(status func(ctx context.Context) egress.Status) *Detector {
	return &Detector{Egress: status, InterfaceAddrs: net.InterfaceAddrs}
}

// Detect uses the address Cloudflare saw for each family. A WARP exit is
// Cloudflare's address, not this host's: that family falls back to a
// public address on the network interfaces. If no family answered at
// all, both fall back to the interfaces (D-041, D-132).
func (d *Detector) Detect(ctx context.Context) Detected {
	st := d.Egress(ctx)
	var local []netip.Addr
	if addrs, err := d.InterfaceAddrs(); err == nil {
		local = publicAddrs(addrs)
	}
	fromLocal := func(v6 bool) string {
		for _, ip := range local {
			if ip.Is6() == v6 {
				return ip.String()
			}
		}
		return ""
	}
	var res Detected
	for _, f := range []struct {
		dest *string
		v6   bool
	}{{&res.IPv4, false}, {&res.IPv6, true}} {
		fam := st.Of(f.v6)
		switch {
		case fam.Native():
			*f.dest = fam.IP.String()
			res.Source = SourceTrace
		case fam.WARP:
			*f.dest = fromLocal(f.v6)
		}
	}
	if !st.IPv4.OK && !st.IPv6.OK {
		res.IPv4, res.IPv6 = fromLocal(false), fromLocal(true)
	}
	if res.Source == SourceNone && (res.IPv4 != "" || res.IPv6 != "") {
		res.Source = SourceInterface
	}
	return res
}

// nonPublic are ranges not reachable from the internet, beyond what
// netip.Addr's predicates cover.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// IsPublic reports whether ip is a global address reachable from the
// internet.
func IsPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func publicAddrs(addrs []net.Addr) []netip.Addr {
	var out []netip.Addr
	for _, a := range addrs {
		prefix, err := netip.ParsePrefix(a.String())
		if err != nil {
			continue
		}
		if ip := prefix.Addr().Unmap(); IsPublic(ip) {
			out = append(out, ip)
		}
	}
	return out
}

// OnInterface reports whether ip is assigned to a local interface. A
// public IP that is not suggests NAT (D-043).
func (d *Detector) OnInterface(ip string) bool {
	want, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addrs, err := d.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Unmap() == want.Unmap() {
			return true
		}
	}
	return false
}
