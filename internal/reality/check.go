// Package reality checks and scans REALITY target websites (DESIGN
// section 10).
package reality

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// LargeHomepage is the homepage size above which a target gets a warning:
// half of the default fallback limit (10 MiB). A homepage near the limit
// would get normal visitors throttled, which reveals that the VPS is not
// the real site (D-051).
const LargeHomepage = 5 << 20

// SlowResolve is the resolving time above which a check suggests another
// DNS server (D-134).
const SlowResolve = 2 * time.Second

// resolveTimeout bounds resolving the target, separately from connecting.
const resolveTimeout = 5 * time.Second

// maxHomepage bounds how much of a homepage is read.
const maxHomepage = 20 << 20

// Result is the outcome of checking one target (DESIGN section 10.2).
type Result struct {
	// Target is "host:port".
	Target string
	Host   string
	// Err is set when the site could not be reached at all.
	Err    error
	TLS13  bool
	H2     bool
	X25519 bool
	// MLKEM is set when the target picks X25519MLKEM768 over X25519, as it
	// would for a browser-like client. REALITY servers before Xray v26.9.8
	// then fail the handshake (D-054).
	MLKEM bool
	// MLKEMSupported copies Checker.MLKEMSupported.
	MLKEMSupported bool
	CertOK         bool
	CertErr        error
	// Latency is the TCP connect plus TLS handshake time.
	Latency time.Duration
	// Resolve is the time taken to resolve the host.
	Resolve time.Duration
	// HomepageSize is the size of the homepage body in bytes, -1 if it
	// could not be fetched.
	HomepageSize int64
	// Keyword is the high-risk keyword in the domain, "" if none.
	Keyword string
}

// Usable reports whether the target meets the REALITY requirements,
// including answering an HTTP request for "/" like a real website does
// (D-058): a host that completes TLS but serves nothing gives the disguise
// away and is often itself a proxy server. High-risk keywords and large
// homepages are warnings, not failures.
func (r Result) Usable() bool {
	return r.Err == nil && r.TLS13 && r.H2 && r.X25519 && r.CertOK && (!r.MLKEM || r.MLKEMSupported) && r.HomepageSize >= 0
}

// SlowDNS reports whether resolving the host was slow enough to suggest
// another DNS server.
func (r Result) SlowDNS() bool { return r.Resolve > SlowResolve }

// LargeHomepage reports whether the homepage is large enough to warn.
func (r Result) LargeHomepage() bool {
	return r.HomepageSize > LargeHomepage
}

// Checker checks targets. The zero value uses the system resolver and
// root certificates.
type Checker struct {
	// Timeout bounds each connection; 0 means 5 seconds.
	Timeout time.Duration
	// Roots overrides the system root certificates (tests).
	Roots *x509.CertPool
	// Dial overrides how "ip:port" is dialed (tests).
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// Resolve overrides how the host is resolved (tests).
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	// MLKEMSupported tells whether the Xray in use handles targets that
	// pick X25519MLKEM768 (xray.SupportsMLKEMTargets).
	MLKEMSupported bool
}

// SetMLKEMSupported records whether the Xray in use handles targets that
// negotiate X25519MLKEM768.
func (c *Checker) SetMLKEMSupported(ok bool) { c.MLKEMSupported = ok }

func (c *Checker) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 5 * time.Second
}

// resolve looks the host up with its own time limit, so a slow DNS server
// is told apart from a slow site (D-134).
func (c *Checker) resolve(ctx context.Context, host string) ([]netip.Addr, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	start := time.Now()
	var addrs []netip.Addr
	var err error
	if c.Resolve != nil {
		addrs, err = c.Resolve(ctx, host)
	} else {
		addrs, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
	if err == nil && len(addrs) == 0 {
		err = fmt.Errorf("no address for %s", host)
	}
	return addrs, time.Since(start), err
}

// dial connects to the resolved addresses in order and returns the first
// connection; on failure it returns the error of the first address.
func (c *Checker) dial(ctx context.Context, addrs []netip.Addr, port string) (net.Conn, error) {
	var first error
	for _, a := range addrs {
		addr := net.JoinHostPort(a.Unmap().String(), port)
		var conn net.Conn
		var err error
		if c.Dial != nil {
			conn, err = c.Dial(ctx, "tcp", addr)
		} else {
			d := net.Dialer{Timeout: c.timeout()}
			conn, err = d.DialContext(ctx, "tcp", addr)
		}
		if err == nil {
			return conn, nil
		}
		if first == nil {
			first = err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, first
}

// handshake connects and performs a TLS handshake offering only the given
// curves. Certificate problems do not fail the handshake; they are
// returned separately so the other properties can still be reported.
func (c *Checker) handshake(ctx context.Context, addrs []netip.Addr, host, port string, curves []tls.CurveID) (tls.ConnectionState, time.Duration, error, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	start := time.Now()
	raw, err := c.dial(ctx, addrs, port)
	if err != nil {
		return tls.ConnectionState{}, 0, nil, err
	}
	defer raw.Close()
	var certErr error
	conn := tls.Client(raw, &tls.Config{
		ServerName:         host,
		NextProtos:         []string{"h2", "http/1.1"},
		MinVersion:         tls.VersionTLS12,
		CurvePreferences:   curves,
		InsecureSkipVerify: true, // verified below, without aborting the handshake
		VerifyConnection: func(cs tls.ConnectionState) error {
			certErr = verifyCert(cs, host, c.Roots)
			return nil
		},
	})
	if err := conn.HandshakeContext(ctx); err != nil {
		return tls.ConnectionState{}, 0, nil, err
	}
	return conn.ConnectionState(), time.Since(start), certErr, nil
}

func verifyCert(cs tls.ConnectionState, host string, roots *x509.CertPool) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("no certificate")
	}
	opts := x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: x509.NewCertPool()}
	for _, ic := range cs.PeerCertificates[1:] {
		opts.Intermediates.AddCert(ic)
	}
	_, err := cs.PeerCertificates[0].Verify(opts)
	return err
}

// Check checks one target, "host" or "host:port" (default port 443).
func (c *Checker) Check(ctx context.Context, target string) Result {
	host, port, err := SplitTarget(target)
	res := Result{Target: net.JoinHostPort(host, port), Host: host, HomepageSize: -1, Keyword: RiskyKeyword(host),
		MLKEMSupported: c.MLKEMSupported}
	if err != nil {
		res.Err = err
		return res
	}

	addrs, took, err := c.resolve(ctx, host)
	res.Resolve = took
	if err != nil {
		res.Err = err
		return res
	}

	// Offer what a browser offers, ML-KEM hybrid first: the choice tells
	// whether the target negotiates X25519MLKEM768 (D-054).
	cs, latency, certErr, err := c.handshake(ctx, addrs, host, port, []tls.CurveID{tls.X25519MLKEM768, tls.X25519})
	if err != nil {
		// Retry with the default curves to report the other properties.
		cs, latency, certErr, err = c.handshake(ctx, addrs, host, port, nil)
		if err != nil {
			res.Err = err
			return res
		}
	}
	res.MLKEM = cs.CurveID == tls.X25519MLKEM768
	if cs.CurveID == tls.X25519 {
		res.X25519 = true
	} else {
		// Offer only X25519: success proves the server supports it.
		_, _, _, err := c.handshake(ctx, addrs, host, port, []tls.CurveID{tls.X25519})
		res.X25519 = err == nil
	}
	res.TLS13 = cs.Version == tls.VersionTLS13
	res.H2 = cs.NegotiatedProtocol == "h2"
	res.CertOK = certErr == nil
	res.CertErr = certErr
	res.Latency = latency
	res.HomepageSize = c.homepageSize(ctx, addrs, host, port)
	return res
}

// homepageSize requests "/" once, without following redirects, and
// returns the body size, or -1 on failure.
func (c *Checker) homepageSize(ctx context.Context, addrs []netip.Addr, host, port string) int64 {
	ctx, cancel := context.WithTimeout(ctx, 2*c.timeout())
	defer cancel()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return c.dial(ctx, addrs, port)
		},
		TLSClientConfig:   &tls.Config{ServerName: host, RootCAs: c.Roots, InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/", nil)
	if err != nil {
		return -1
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxHomepage))
	if err != nil && n == 0 {
		return -1
	}
	return n
}

// SplitTarget parses "host" or "host:port" (default port 443). The host
// must be a domain name: REALITY needs it for serverNames.
func SplitTarget(s string) (host, port string, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	host, port = s, "443"
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, port = h, p
	}
	if !validDomain(host) {
		return host, port, fmt.Errorf("%q is not a domain name", host)
	}
	if n, err := net.LookupPort("tcp", port); err != nil || n < 1 || n > 65535 || fmt.Sprint(n) != port {
		return host, port, fmt.Errorf("invalid port %q", port)
	}
	return host, port, nil
}

func validDomain(h string) bool {
	if len(h) == 0 || len(h) > 253 || net.ParseIP(h) != nil || !strings.Contains(h, ".") {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
