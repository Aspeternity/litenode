// Package egress detects whether this host can reach the internet over
// IPv4 and IPv6, and whether an exit is Cloudflare WARP (DESIGN section
// 19.1, D-132).
package egress

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Cloudflare trace endpoints, reached by IP so no DNS is needed (D-041).
const (
	TraceURLv4 = "https://1.1.1.1/cdn-cgi/trace"
	TraceURLv6 = "https://[2606:4700:4700::1111]/cdn-cgi/trace"
)

// Timeout bounds each trace request.
const Timeout = 3 * time.Second

// Family is the exit of one address family.
type Family struct {
	// OK is set when the trace answered over this family.
	OK bool
	// IP is the address the internet sees.
	IP netip.Addr
	// WARP is set when the exit is Cloudflare WARP: IP then belongs to
	// Cloudflare, not to this host.
	WARP bool
}

// Native reports a working exit that is not WARP.
func (f Family) Native() bool { return f.OK && !f.WARP }

// Status is the exit of both families.
type Status struct {
	IPv4, IPv6 Family
}

// Of returns the family of ipv6.
func (s Status) Of(ipv6 bool) Family {
	if ipv6 {
		return s.IPv6
	}
	return s.IPv4
}

// Prober detects the exits at most once per run (until Reset).
type Prober struct {
	// Get fetches a URL and returns the body; it must time out by itself.
	Get func(ctx context.Context, url string) ([]byte, error)

	mu  sync.Mutex
	cur *run
}

// run is one detection; status is valid once done is closed.
type run struct {
	done   chan struct{}
	status Status
}

// New returns a Prober using HTTPS with Timeout per request.
func New() *Prober {
	client := &http.Client{Timeout: Timeout, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		Proxy:           nil, // never through a proxy: we want this host's exit
	}}
	return &Prober{Get: func(ctx context.Context, url string) ([]byte, error) {
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
	}}
}

// Start begins detection in the background unless it already ran or is
// running.
func (p *Prober) Start(ctx context.Context) {
	p.start(ctx)
}

func (p *Prober) start(ctx context.Context) *run {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cur != nil {
		return p.cur
	}
	r := &run{done: make(chan struct{})}
	p.cur = r
	go func() {
		r.status = p.detect(ctx)
		close(r.done)
	}()
	return r
}

// Status returns the detected exits, waiting for detection to finish.
func (p *Prober) Status(ctx context.Context) Status {
	r := p.start(ctx)
	select {
	case <-r.done:
		return r.status
	case <-ctx.Done():
		return Status{}
	}
}

// Reset makes the next Status detect again, e.g. after WARP changed.
func (p *Prober) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cur = nil
}

func (p *Prober) detect(ctx context.Context) Status {
	var st Status
	var wg sync.WaitGroup
	for _, f := range []struct {
		url  string
		dest *Family
		v6   bool
	}{{TraceURLv4, &st.IPv4, false}, {TraceURLv6, &st.IPv6, true}} {
		wg.Go(func() {
			body, err := p.Get(ctx, f.url)
			if err != nil {
				return
			}
			if t, ok := ParseTrace(body); ok && t.IP.Is6() == f.v6 {
				*f.dest = Family{OK: true, IP: t.IP, WARP: t.WARP}
			}
		})
	}
	wg.Wait()
	return st
}

// Trace is the part of a Cloudflare trace response LiteNode uses.
type Trace struct {
	IP   netip.Addr
	WARP bool
}

// ParseTrace reads the "ip=" and "warp=" lines of a trace response. WARP
// is "on" for the free service and "plus" for WARP+.
func ParseTrace(body []byte) (Trace, bool) {
	var t Trace
	ok := false
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		k, v, found := strings.Cut(sc.Text(), "=")
		if !found {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "ip":
			ip, err := netip.ParseAddr(v)
			if err != nil {
				return Trace{}, false
			}
			t.IP, ok = ip.Unmap(), true
		case "warp":
			t.WARP = v == "on" || v == "plus"
		}
	}
	return t, ok
}
