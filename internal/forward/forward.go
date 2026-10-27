// Package forward is the port forwarding model (DESIGN section 11): rules
// that forward a local port or port range to a target with nftables DNAT.
// It has no system access.
package forward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/share"
)

// Protocols a rule forwards.
const (
	TCP    = "tcp"
	UDP    = "udp"
	TCPUDP = "tcp+udp"
)

// Forward is one forwarding rule in the state file.
type Forward struct {
	ID int64 `json:"id"`
	// Remark is optional.
	Remark  string `json:"remark"`
	Enabled bool   `json:"enabled"`
	// Protocol is TCP, UDP or TCPUDP.
	Protocol string `json:"protocol"`
	// ListenPort is one port ("30001") or a range ("30000-30010").
	ListenPort string `json:"listen_port"`
	// TargetHost is an IP address or a domain name.
	TargetHost string `json:"target_host"`
	// TargetPort is a port, or a range as long as ListenPort.
	TargetPort string `json:"target_port"`
	// ResolvedIPv4 and ResolvedIPv6 are the A and AAAA records of a domain
	// target, "" if it has none (D-046). Unused for IP targets.
	ResolvedIPv4 string `json:"resolved_ipv4"`
	ResolvedIPv6 string `json:"resolved_ipv6"`
}

// Owner names the rule in port claims, firewall rules and ufw comments.
func Owner(id int64) string { return "forward:" + strconv.FormatInt(id, 10) }

// Errors of Check, mapped to user-facing text by the menu.
var (
	ErrProtocol    = errors.New("unknown protocol")
	ErrPort        = errors.New("invalid port or port range")
	ErrPortLength  = errors.New("target port range differs in length")
	ErrTargetHost  = errors.New("invalid target address")
	ErrNoAddresses = errors.New("target has no IPv4 or IPv6 address")
)

// Check validates the rule (not whether its ports are free): its fields,
// and that it has a target address.
func Check(f Forward) error {
	if err := CheckFields(f); err != nil {
		return err
	}
	if f.IPv4() == "" && f.IPv6() == "" {
		return ErrNoAddresses
	}
	return nil
}

// Unreachable reports whether a target with the given address families
// can only be reached over a family the machine has no exit for (D-134):
// connections are forwarded within their family (D-046). ipv6 names that
// family. When neither exit works the detection tells nothing.
func Unreachable(targetV4, targetV6, exitV4, exitV6 bool) (ipv6, unreachable bool) {
	if !exitV4 && !exitV6 || targetV4 && exitV4 || targetV6 && exitV6 {
		return false, false
	}
	return !targetV4, targetV4 || targetV6
}

// CheckFields validates what the user enters, before a domain target is
// resolved.
func CheckFields(f Forward) error {
	switch f.Protocol {
	case TCP, UDP, TCPUDP:
	default:
		return ErrProtocol
	}
	lf, lt, err := ParsePorts(f.ListenPort)
	if err != nil {
		return ErrPort
	}
	tf, tt, err := ParsePorts(f.TargetPort)
	if err != nil {
		return ErrPort
	}
	if lt-lf != tt-tf {
		return ErrPortLength
	}
	if !share.ValidAddress(f.TargetHost) {
		return ErrTargetHost
	}
	return nil
}

// NormalizeRemark trims the remark; an empty remark is allowed, otherwise
// it follows the node remark format (D-039). Forward remarks need not be
// unique.
func NormalizeRemark(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	return node.NormalizeRemark(s)
}

// IsDomain reports whether the target is a domain name.
func (f Forward) IsDomain() bool {
	_, err := netip.ParseAddr(f.TargetHost)
	return err != nil
}

// IPv4 is the IPv4 address IPv4 connections go to, "" if none: IPv4
// connections are only forwarded to IPv4 targets (D-046).
func (f Forward) IPv4() string {
	if !f.IsDomain() {
		if a, _ := netip.ParseAddr(f.TargetHost); a.Unmap().Is4() {
			return a.Unmap().String()
		}
		return ""
	}
	return f.ResolvedIPv4
}

// IPv6 is the IPv6 address IPv6 connections go to, "" if none.
func (f Forward) IPv6() string {
	if !f.IsDomain() {
		if a, _ := netip.ParseAddr(f.TargetHost); a.Is6() && !a.Is4In6() {
			return a.String()
		}
		return ""
	}
	return f.ResolvedIPv6
}

// ParsePorts reads "443" or "10000-10100".
func ParsePorts(s string) (from, to int, err error) {
	a, b, isRange := strings.Cut(s, "-")
	if from, err = parsePort(a); err != nil {
		return 0, 0, err
	}
	to = from
	if isRange {
		if to, err = parsePort(b); err != nil {
			return 0, 0, err
		}
		if to <= from {
			return 0, 0, fmt.Errorf("port range %q is empty", s)
		}
	}
	return from, to, nil
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || strconv.Itoa(p) != s || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return p, nil
}

// Resolver looks up addresses; *net.Resolver implements it.
type Resolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// Resolve looks up the A and AAAA records of a domain separately (D-046).
// An address in use now is kept while the domain still has it, so a
// domain with several records does not switch on every lookup. It fails
// only when the domain has neither.
func Resolve(ctx context.Context, r Resolver, host, curV4, curV6 string) (v4, v6 string, err error) {
	v4s, err4 := r.LookupIP(ctx, "ip4", host)
	v6s, err6 := r.LookupIP(ctx, "ip6", host)
	v4, v6 = pick(v4s, curV4, true), pick(v6s, curV6, false)
	if v4 == "" && v6 == "" {
		if err := errors.Join(err4, err6); err != nil {
			return "", "", fmt.Errorf("resolve %s: %w", host, err)
		}
		return "", "", ErrNoAddresses
	}
	return v4, v6, nil
}

// Reresolve looks up a domain target again for the resolve timer (DESIGN
// section 11). Unlike Resolve, a family whose lookup fails for any reason
// other than the record not existing keeps its current address, so a DNS
// outage does not drop working rules; a domain with neither record also
// keeps both. err reports what could not be looked up; v4 and v6 are
// always usable.
func Reresolve(ctx context.Context, r Resolver, host, curV4, curV6 string) (v4, v6 string, err error) {
	v4s, err4 := r.LookupIP(ctx, "ip4", host)
	v6s, err6 := r.LookupIP(ctx, "ip6", host)
	v4, v6 = pick(v4s, curV4, true), pick(v6s, curV6, false)
	var errs []error
	if err4 != nil && !notFound(err4) {
		v4 = curV4
		errs = append(errs, err4)
	}
	if err6 != nil && !notFound(err6) {
		v6 = curV6
		errs = append(errs, err6)
	}
	if v4 == "" && v6 == "" {
		return curV4, curV6, fmt.Errorf("resolve %s: %w", host, ErrNoAddresses)
	}
	if len(errs) > 0 {
		return v4, v6, fmt.Errorf("resolve %s: %w", host, errors.Join(errs...))
	}
	return v4, v6, nil
}

// notFound reports whether err means the record does not exist, as
// opposed to the lookup failing. The Go resolver reports a domain without
// records of the asked family as an AddrError.
func notFound(err error) bool {
	var de *net.DNSError
	if errors.As(err, &de) {
		return de.IsNotFound
	}
	var ae *net.AddrError
	return errors.As(err, &ae) && ae.Err == "no suitable address found"
}

func pick(ips []net.IP, current string, want4 bool) string {
	var first string
	for _, ip := range ips {
		a, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		a = a.Unmap()
		if a.Is4() != want4 {
			continue
		}
		if a.String() == current {
			return current
		}
		if first == "" {
			first = a.String()
		}
	}
	return first
}
