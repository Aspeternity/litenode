package xray

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/Aspeternity/litenode/internal/system"
)

// Node outbound IP modes, stored in xray.outbound_ip (DESIGN section 12,
// D-084). They set the domainStrategy of the freedom outbound and are
// independent of the system gai.conf preference.
const (
	OutboundDefault    = ""
	OutboundPreferIPv4 = "prefer_ipv4"
	OutboundPreferIPv6 = "prefer_ipv6"
	OutboundIPv4Only   = "ipv4_only"
	OutboundIPv6Only   = "ipv6_only"
)

// OutboundModes are the modes in menu order.
var OutboundModes = []string{OutboundDefault, OutboundPreferIPv4, OutboundPreferIPv6, OutboundIPv4Only, OutboundIPv6Only}

// outboundStrategies maps a mode to the freedom domainStrategy. The
// default (AsIs) leaves the field out.
var outboundStrategies = map[string]string{
	OutboundDefault:    "",
	OutboundPreferIPv4: "UseIPv4v6",
	OutboundPreferIPv6: "UseIPv6v4",
	OutboundIPv4Only:   "ForceIPv4",
	OutboundIPv6Only:   "ForceIPv6",
}

func domainStrategy(mode string) (string, error) {
	s, ok := outboundStrategies[mode]
	if !ok {
		return "", fmt.Errorf("unknown outbound ip mode %q", mode)
	}
	return s, nil
}

// OnlyIPv6 reports the family an "only" mode requires: ok is false for the
// modes that do not restrict the family.
func OnlyIPv6(mode string) (ipv6, ok bool) {
	switch mode {
	case OutboundIPv4Only:
		return false, true
	case OutboundIPv6Only:
		return true, true
	}
	return false, false
}

// FamilyReady reports whether this host can reach the internet over IPv4
// (ipv6 false) or IPv6: it needs a default route and a global address of
// that family. IPv4 accepts private addresses behind NAT, which many VPSes
// use; IPv6 needs a public address, since IPv6 is normally not NATed.
func FamilyReady(ctx context.Context, r system.Runner, ipv6 bool) (bool, error) {
	fam := "-4"
	if ipv6 {
		fam = "-6"
	}
	route, err := r.Query(ctx, "ip", fam, "route", "show", "default")
	if err != nil {
		return false, fmt.Errorf("ip %s route: %w", fam, err)
	}
	addrs, err := r.Query(ctx, "ip", "-o", fam, "addr", "show", "scope", "global")
	if err != nil {
		return false, fmt.Errorf("ip %s addr: %w", fam, err)
	}
	return ParseFamilyReady(route.Stdout, addrs.Stdout, ipv6), nil
}

// ParseFamilyReady decides FamilyReady from the output of "ip route show
// default" and "ip -o addr show scope global".
func ParseFamilyReady(routeOut, addrOut string, ipv6 bool) bool {
	if strings.TrimSpace(routeOut) == "" {
		return false
	}
	for _, line := range strings.Split(addrOut, "\n") {
		f := strings.Fields(line)
		for i := 0; i+1 < len(f); i++ {
			if f[i] != "inet" && f[i] != "inet6" {
				continue
			}
			p, err := netip.ParsePrefix(f[i+1])
			if err != nil {
				continue
			}
			a := p.Addr()
			if a.Is6() != ipv6 || !a.IsGlobalUnicast() {
				continue
			}
			if !ipv6 || isPublicIPv6(a) {
				return true
			}
		}
	}
	return false
}

// isPublicIPv6 excludes unique local and documentation addresses.
func isPublicIPv6(a netip.Addr) bool {
	return !a.IsPrivate() && !netip.MustParsePrefix("2001:db8::/32").Contains(a)
}
