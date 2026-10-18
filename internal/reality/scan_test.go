package reality

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestScanRange(t *testing.T) {
	ips, err := ScanRange(netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 253 || ips[0].String() != "203.0.113.1" || ips[252].String() != "203.0.113.254" {
		t.Errorf("range: %d addresses, %v .. %v", len(ips), ips[0], ips[len(ips)-1])
	}
	if slices.Contains(ips, netip.MustParseAddr("203.0.113.10")) {
		t.Error("own address scanned")
	}
	if _, err := ScanRange(netip.MustParseAddr("2001:db8::1")); err == nil {
		t.Error("IPv6 accepted")
	}
}

func TestScan(t *testing.T) {
	center := netip.MustParseAddr("203.0.113.10")
	certs := map[string][]string{
		"203.0.113.20": {"shop.example.com", "*.example.com"},    // wildcard skipped
		"203.0.113.21": {"cdn.example.net"},                      // risky keyword
		"203.0.113.22": {"far.example.org"},                      // resolves elsewhere
		"203.0.113.23": {"blog.example.io"},                      // nearby, usable, slower
		"203.0.113.24": {"slow.example.io", "SHOP.example.com."}, // duplicate name
		"203.0.113.25": {"weak.example.io"},                      // nearby but fails checks
	}
	dns := map[string]string{
		"shop.example.com": "203.0.113.20", "far.example.org": "198.51.100.1",
		"blog.example.io": "203.0.113.23", "slow.example.io": "203.0.113.24", "weak.example.io": "203.0.113.25",
	}
	latency := map[string]time.Duration{"shop.example.com": 5, "blog.example.io": 9, "slow.example.io": 30}

	var mu sync.Mutex
	running, peak := 0, 0
	var checked []string
	s := &Scanner{
		Names: func(_ context.Context, ip netip.Addr) ([]string, error) {
			mu.Lock()
			running++
			peak = max(peak, running)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
			if n, ok := certs[ip.String()]; ok {
				return n, nil
			}
			return nil, errors.New("no tls")
		},
		Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			if a, ok := dns[host]; ok {
				return []netip.Addr{netip.MustParseAddr(a)}, nil
			}
			return nil, errors.New("nxdomain")
		},
		Check: func(_ context.Context, host string) Result {
			mu.Lock()
			checked = append(checked, host)
			mu.Unlock()
			ok := host != "weak.example.io"
			return Result{Target: host + ":443", Host: host, TLS13: ok, H2: ok, X25519: ok, CertOK: ok, Latency: latency[host] * time.Millisecond}
		},
	}
	var lastDone, total int
	res, err := s.Scan(context.Background(), center, func(d, n int) { lastDone, total = d, n })
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	for _, r := range res {
		hosts = append(hosts, r.Host)
	}
	if want := []string{"shop.example.com", "blog.example.io", "slow.example.io"}; !slices.Equal(hosts, want) {
		t.Errorf("results = %v, want %v", hosts, want)
	}
	if peak > ScanConcurrency {
		t.Errorf("peak concurrency %d > %d", peak, ScanConcurrency)
	}
	if lastDone != 253 || total != 253 {
		t.Errorf("progress %d/%d", lastDone, total)
	}
	slices.Sort(checked)
	// far.example.org resolves outside the /24 and cdn.example.net is
	// risky: neither is checked.
	if want := []string{"blog.example.io", "shop.example.com", "slow.example.io", "weak.example.io"}; !slices.Equal(checked, want) {
		t.Errorf("checked %v, want %v", checked, want)
	}
}
