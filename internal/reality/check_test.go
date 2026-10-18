package reality

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// server starts a local TLS server whose certificate is valid for
// example.com, configured by fn.
func server(t *testing.T, h2 bool, fn func(*tls.Config), body func(w http.ResponseWriter)) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { body(w) }))
	srv.EnableHTTP2 = h2
	srv.TLS = &tls.Config{}
	if fn != nil {
		fn(srv.TLS)
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return srv, pool
}

// checkerFor dials srv whatever host is asked for.
func checkerFor(srv *httptest.Server, roots *x509.CertPool) *Checker {
	addr := srv.Listener.Addr().String()
	// Go's test server negotiates ML-KEM, like most sites: model an Xray
	// that supports it unless a test says otherwise.
	return &Checker{Timeout: 3 * time.Second, Roots: roots, MLKEMSupported: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}}
}

func small(w http.ResponseWriter) { w.Write([]byte(strings.Repeat("x", 1234))) }

func TestCheckGoodTarget(t *testing.T) {
	srv, roots := server(t, true, nil, small)
	r := checkerFor(srv, roots).Check(context.Background(), "example.com")
	if !r.Usable() || !r.TLS13 || !r.H2 || !r.X25519 || !r.CertOK {
		t.Fatalf("result = %+v", r)
	}
	if r.Target != "example.com:443" || r.HomepageSize != 1234 || r.Latency <= 0 || r.LargeHomepage() {
		t.Errorf("result = %+v", r)
	}
}

func TestCheckFailures(t *testing.T) {
	tests := []struct {
		name  string
		h2    bool
		tls   func(*tls.Config)
		roots bool
		host  string
		check func(Result) bool
	}{
		{"no TLS 1.3", true, func(c *tls.Config) { c.MaxVersion = tls.VersionTLS12 }, true, "example.com",
			func(r Result) bool { return !r.TLS13 && r.CertOK && r.X25519 }},
		{"no H2", false, nil, true, "example.com",
			func(r Result) bool { return !r.H2 && r.TLS13 && r.X25519 }},
		{"no X25519", true, func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.CurveP256} }, true, "example.com",
			func(r Result) bool { return !r.X25519 && r.TLS13 && r.H2 && r.CertOK }},
		{"untrusted certificate", true, nil, false, "example.com",
			func(r Result) bool { return !r.CertOK && r.CertErr != nil && r.TLS13 && r.H2 }},
		{"certificate for another name", true, nil, true, "www.other.com",
			func(r Result) bool { return !r.CertOK && r.TLS13 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, roots := server(t, tt.h2, tt.tls, small)
			if !tt.roots {
				roots = x509.NewCertPool()
			}
			r := checkerFor(srv, roots).Check(context.Background(), tt.host)
			if r.Err != nil || r.Usable() || !tt.check(r) {
				t.Errorf("result = %+v", r)
			}
		})
	}
}

func TestCheckUnreachable(t *testing.T) {
	c := &Checker{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("refused") }}
	r := c.Check(context.Background(), "example.com:8443")
	if r.Err == nil || r.Usable() || r.Target != "example.com:8443" {
		t.Errorf("result = %+v", r)
	}
}

func TestHomepage(t *testing.T) {
	srv, roots := server(t, true, nil, func(w http.ResponseWriter) { w.Write(make([]byte, LargeHomepage+1)) })
	if r := checkerFor(srv, roots).Check(context.Background(), "example.com"); !r.LargeHomepage() || !r.Usable() {
		t.Errorf("large homepage: %+v", r)
	}
	// Redirects are not followed: the size is that of the redirect itself.
	srv, roots = server(t, true, nil, func(w http.ResponseWriter) {
		w.Header().Set("Location", "/big")
		w.WriteHeader(http.StatusMovedPermanently)
	})
	if r := checkerFor(srv, roots).Check(context.Background(), "example.com"); r.HomepageSize != 0 {
		t.Errorf("redirect size = %d, want 0", r.HomepageSize)
	}
}

func TestSplitTarget(t *testing.T) {
	ok := map[string][2]string{
		"www.example.com":       {"www.example.com", "443"},
		" WWW.Example.com:8443": {"www.example.com", "8443"},
		"a-b.example.co.jp":     {"a-b.example.co.jp", "443"},
	}
	for in, want := range ok {
		h, p, err := SplitTarget(in)
		if err != nil || h != want[0] || p != want[1] {
			t.Errorf("SplitTarget(%q) = (%q, %q, %v)", in, h, p, err)
		}
	}
	for _, bad := range []string{"", "localhost", "1.2.3.4", "[::1]:443", "example.com:0", "example.com:70000",
		"example.com:https", "-bad.example.com", "exa mple.com", "例子.com", "https://example.com"} {
		if _, _, err := SplitTarget(bad); err == nil {
			t.Errorf("SplitTarget(%q) accepted", bad)
		}
	}
}

func TestRiskyKeyword(t *testing.T) {
	for host, want := range map[string]string{
		"www.microsoft.com":      "",
		"cdn.example.com":        "cdn",
		"img-cdn1.example.com":   "cdn",
		"dl.example.com":         "dl",
		"dl-eu.example.com":      "dl",
		"www.middle.com":         "",
		"downloads.example.com":  "download",
		"livestream.example.com": "stream",
		"www.speedtest.net":      "speedtest",
		"update.example.com":     "update",
		"mirrors.example.org":    "mirror",
		"video.example.com":      "video",
	} {
		if got := RiskyKeyword(host); got != want {
			t.Errorf("RiskyKeyword(%q) = %q, want %q", host, got, want)
		}
	}
	for _, h := range CommonTargets {
		if kw := RiskyKeyword(h); kw != "" {
			t.Errorf("common target %s has risky keyword %s", h, kw)
		}
		if _, _, err := SplitTarget(h); err != nil {
			t.Errorf("common target %s: %v", h, err)
		}
	}
}

func TestCheckAllOrder(t *testing.T) {
	good, _ := server(t, true, nil, small)
	noH2, _ := server(t, false, nil, small)
	pool := x509.NewCertPool()
	pool.AddCert(good.Certificate())
	pool.AddCert(noH2.Certificate())
	// All targets are example.com; the port picks the server.
	addrs := map[string]string{"1": good.Listener.Addr().String(), "2": noH2.Listener.Addr().String()}
	c := &Checker{Timeout: 3 * time.Second, Roots: pool, MLKEMSupported: true, Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, _ := net.SplitHostPort(addr)
		a, ok := addrs[port]
		if !ok {
			return nil, errors.New("refused")
		}
		var d net.Dialer
		return d.DialContext(ctx, network, a)
	}}
	res := c.CheckAll(context.Background(), []string{"example.com:3", "example.com:2", "example.com:1"}, 2)
	var order []string
	for _, r := range res {
		order = append(order, r.Target)
	}
	want := []string{"example.com:1", "example.com:2", "example.com:3"} // usable, reachable, unreachable
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestCheckMLKEM(t *testing.T) {
	// The server prefers X25519MLKEM768, like most large sites today.
	srv, roots := server(t, true, func(c *tls.Config) {
		c.CurvePreferences = []tls.CurveID{tls.X25519MLKEM768, tls.X25519}
	}, small)
	c := checkerFor(srv, roots)
	c.SetMLKEMSupported(false)
	r := c.Check(context.Background(), "example.com")
	if !r.MLKEM || !r.X25519 || r.Usable() {
		t.Errorf("old Xray: %+v, want MLKEM detected and not usable", r)
	}
	c.SetMLKEMSupported(true)
	if r := c.Check(context.Background(), "example.com"); !r.MLKEM || !r.Usable() {
		t.Errorf("new Xray: %+v, want usable", r)
	}
	// A server that only does X25519 is usable either way.
	srv2, roots2 := server(t, true, func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.X25519} }, small)
	c2 := checkerFor(srv2, roots2)
	c2.SetMLKEMSupported(false)
	if r := c2.Check(context.Background(), "example.com"); r.MLKEM || !r.Usable() {
		t.Errorf("X25519-only server: %+v", r)
	}
}

func TestCheckNoHTTPResponse(t *testing.T) {
	// TLS works but the server closes without an HTTP response.
	// HTTP/1.1 only, so the handler can hijack and drop the connection.
	srv, roots := server(t, false, nil, func(w http.ResponseWriter) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	})
	r := checkerFor(srv, roots).Check(context.Background(), "example.com")
	if r.HomepageSize != -1 || r.Usable() {
		t.Errorf("result = %+v, want unusable without an HTTP response", r)
	}
}
