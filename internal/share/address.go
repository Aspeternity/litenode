package share

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Cloudflare trace endpoints, reached by IP so no DNS is needed (D-041).
const (
	TraceURLv4 = "https://1.1.1.1/cdn-cgi/trace"
	TraceURLv6 = "https://[2606:4700:4700::1111]/cdn-cgi/trace"
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
	// Get fetches a URL with a timeout and returns the body.
	Get func(ctx context.Context, url string) ([]byte, error)
	// InterfaceAddrs lists the addresses on the network interfaces.
	InterfaceAddrs func() ([]net.Addr, error)
}

// NewDetector returns a Detector using HTTPS with a 5 second timeout.
func NewDetector() *Detector {
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		Proxy:           nil, // never through a proxy: we want this host's address
	}}
	return &Detector{
		Get: func(ctx context.Context, url string) ([]byte, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		},
		InterfaceAddrs: net.InterfaceAddrs,
	}
}

// Detect asks Cloudflare over IPv4 and IPv6; if neither answers it falls
// back to public addresses on the network interfaces (D-041).
func (d *Detector) Detect(ctx context.Context) Detected {
	var res Detected
	for _, u := range []struct {
		url  string
		dest *string
		v4   bool
	}{{TraceURLv4, &res.IPv4, true}, {TraceURLv6, &res.IPv6, false}} {
		body, err := d.Get(ctx, u.url)
		if err != nil {
			continue
		}
		if ip, ok := ParseTrace(body); ok && ip.Is4() == u.v4 {
			*u.dest = ip.String()
		}
	}
	if res.IPv4 != "" || res.IPv6 != "" {
		res.Source = SourceTrace
		return res
	}
	addrs, err := d.InterfaceAddrs()
	if err != nil {
		return res
	}
	for _, ip := range publicAddrs(addrs) {
		if ip.Is4() && res.IPv4 == "" {
			res.IPv4 = ip.String()
		} else if ip.Is6() && res.IPv6 == "" {
			res.IPv6 = ip.String()
		}
	}
	if res.IPv4 != "" || res.IPv6 != "" {
		res.Source = SourceInterface
	}
	return res
}

// ParseTrace extracts the "ip=" line of a Cloudflare trace response.
func ParseTrace(body []byte) (netip.Addr, bool) {
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "ip="); ok {
			ip, err := netip.ParseAddr(strings.TrimSpace(v))
			return ip.Unmap(), err == nil
		}
	}
	return netip.Addr{}, false
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
