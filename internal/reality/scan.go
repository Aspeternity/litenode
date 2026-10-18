package reality

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

// Scan limits (DESIGN section 10.3, D-058).
const (
	ScanConcurrency = 10
	scanTimeout     = 3 * time.Second
	maxScanDomains  = 60
	ScanTop         = 10
)

// ScanRange returns the other addresses of ip's /24 (excluding the network
// and broadcast addresses and ip itself). Only IPv4 is scanned.
func ScanRange(ip netip.Addr) ([]netip.Addr, error) {
	ip = ip.Unmap()
	if !ip.Is4() {
		return nil, errors.New("scan needs an IPv4 address")
	}
	prefix := netip.PrefixFrom(ip, 24).Masked()
	var out []netip.Addr
	for a := prefix.Addr().Next(); prefix.Contains(a); a = a.Next() {
		if a.As4()[3] == 255 {
			break
		}
		if a != ip {
			out = append(out, a)
		}
	}
	return out, nil
}

// Scanner finds REALITY targets near the VPS. The functions are
// replaceable in tests.
type Scanner struct {
	// Names returns the DNS names in the certificate served on ip:443.
	Names func(ctx context.Context, ip netip.Addr) ([]string, error)
	// Resolve returns a domain's addresses.
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	// Check checks a candidate (Checker.Check).
	Check func(ctx context.Context, target string) Result
}

// NewScanner returns a Scanner using the network and checker c (nil:
// set Check before use).
func NewScanner(c *Checker) *Scanner {
	s := &Scanner{Names: CertNames, Resolve: resolve}
	if c != nil {
		s.Check = c.Check
	}
	return s
}

func resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
}

// CertNames handshakes with ip:443 without SNI and returns the
// certificate's DNS names. The certificate is not verified here: names
// only become candidates, and every candidate is fully checked later.
func CertNames(ctx context.Context, ip netip.Addr) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()
	d := net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(ip, 443).String())
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) // no SNI on purpose
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("no certificate")
	}
	names := certs[0].DNSNames
	if len(names) == 0 && certs[0].Subject.CommonName != "" {
		names = []string{certs[0].Subject.CommonName}
	}
	return names, nil
}

// Scan scans the /24 around center (D-058): collects certificate names,
// keeps domains that resolve into the scanned range and have no risky
// keyword, checks them, and returns the usable ones by latency (at most
// ScanTop).
func (s *Scanner) Scan(ctx context.Context, center netip.Addr, progress func(done, total int)) ([]Result, error) {
	ips, err := ScanRange(center)
	if err != nil {
		return nil, err
	}
	prefix := netip.PrefixFrom(center.Unmap(), 24).Masked()

	var mu sync.Mutex
	found := map[string]bool{}
	var order []string
	done := 0
	runLimited(len(ips), ScanConcurrency, func(i int) {
		names, err := s.Names(ctx, ips[i])
		mu.Lock()
		defer mu.Unlock()
		done++
		if progress != nil {
			progress(done, len(ips))
		}
		if err != nil {
			return
		}
		for _, n := range names {
			n = strings.ToLower(strings.TrimSuffix(n, "."))
			if strings.HasPrefix(n, "*.") || !validDomain(n) || found[n] || RiskyKeyword(n) != "" {
				continue
			}
			found[n] = true
			order = append(order, n)
		}
	})
	if len(order) > maxScanDomains {
		order = order[:maxScanDomains]
	}

	var results []Result
	runLimited(len(order), ScanConcurrency, func(i int) {
		host := order[i]
		addrs, err := s.Resolve(ctx, host)
		if err != nil || !slices.ContainsFunc(addrs, func(a netip.Addr) bool { return prefix.Contains(a.Unmap()) }) {
			return // the name is not hosted nearby
		}
		r := s.Check(ctx, host)
		if !r.Usable() || r.LargeHomepage() {
			return
		}
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	slices.SortFunc(results, func(a, b Result) int { return int(a.Latency - b.Latency) })
	if len(results) > ScanTop {
		results = results[:ScanTop]
	}
	return results, nil
}

// runLimited calls fn(0..n-1) with at most limit calls running at once.
func runLimited(n, limit int, fn func(i int)) {
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}()
	}
	wg.Wait()
}
