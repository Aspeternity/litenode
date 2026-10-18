package share

import (
	"context"
	"net"
	"net/netip"
	"slices"

	"github.com/Aspeternity/litenode/internal/node"
)

// Address kinds, shown next to addresses and appended to the remark of
// links that do not use the default address (D-068).
const (
	KindIPv4 = "IPv4"
	KindIPv6 = "IPv6"
)

// AddressKind is KindIPv4, KindIPv6, or "" for a domain.
func AddressKind(addr string) string {
	a, err := netip.ParseAddr(addr)
	switch {
	case err != nil:
		return ""
	case a.Unmap().Is4():
		return KindIPv4
	}
	return KindIPv6
}

// LinkRemark is the remark of a client's link for addr: the client's own
// remark for the default address, otherwise with "-IPv4", "-IPv6" or
// "-<domain>" so that links imported side by side keep apart.
func LinkRemark(remark, addr string, isDefault bool) string {
	if isDefault {
		return remark
	}
	if k := AddressKind(addr); k != "" {
		return remark + "-" + k
	}
	return remark + "-" + addr
}

// LinkFor is Link with the remark for addr.
func LinkFor(reg *node.Registry, n node.Node, c node.Client, addr string, isDefault bool) (string, error) {
	c.Remark = LinkRemark(c.Remark, addr, isDefault)
	return Link(reg, n, c, addr)
}

// MergeDetected replaces the IP addresses in list with the detected ones,
// keeping the user's domains. The default stays first if it is still
// there; otherwise the first detected address (IPv4 first) is.
func MergeDetected(list, detected []string) []string {
	var out []string
	keep := func(a string) {
		if a != "" && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	if len(list) > 0 && (AddressKind(list[0]) == "" || slices.Contains(detected, list[0])) {
		keep(list[0])
	}
	for _, a := range detected {
		keep(a)
	}
	for _, a := range list {
		if AddressKind(a) == "" {
			keep(a)
		}
	}
	return out
}

// Resolver looks up a domain's addresses (net.DefaultResolver).
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DomainCheck is what a domain resolves to and whether that includes one
// of the server's own addresses.
type DomainCheck struct {
	Resolved []string
	Here     bool
}

// CheckDomain resolves domain and compares with the server's addresses: a
// REALITY link must reach this server directly, not a CDN.
func CheckDomain(ctx context.Context, r Resolver, domain string, own []string) (DomainCheck, error) {
	ips, err := r.LookupIPAddr(ctx, domain)
	if err != nil {
		return DomainCheck{}, err
	}
	var c DomainCheck
	for _, ip := range ips {
		a, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			continue
		}
		s := a.Unmap().String()
		c.Resolved = append(c.Resolved, s)
		for _, o := range own {
			if oa, err := netip.ParseAddr(o); err == nil && oa.Unmap() == a.Unmap() {
				c.Here = true
			}
		}
	}
	return c, nil
}
