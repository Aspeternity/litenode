//go:build vps

// Checks real websites from the test VPS (M1-05):
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o reality-vps.test ./internal/reality
//	./reality-vps.test -test.v -test.run TestVPS
package reality

import (
	"context"
	"testing"
)

func TestVPS(t *testing.T) {
	c := &Checker{}
	c.SetMLKEMSupported(true) // as with the default Xray (D-054)
	ctx := context.Background()
	for _, r := range c.CheckAll(ctx, CommonTargets, 10) {
		t.Logf("%-24s %5dms  TLS1.3=%-5v H2=%-5v X25519=%-5v cert=%-5v mlkem=%-5v homepage=%dB usable=%v err=%v",
			r.Host, r.Latency.Milliseconds(), r.TLS13, r.H2, r.X25519, r.CertOK, r.MLKEM, r.HomepageSize, r.Usable(), r.Err)
	}
	if r := c.Check(ctx, "www.microsoft.com"); !r.Usable() {
		t.Errorf("www.microsoft.com should be usable: %+v", r)
	}
	for target, check := range map[string]func(Result) bool{
		"tls-v1-2.badssl.com:1012": func(r Result) bool { return r.Err == nil && !r.TLS13 },
		"expired.badssl.com":       func(r Result) bool { return r.Err == nil && !r.CertOK },
		"wrong.host.badssl.com":    func(r Result) bool { return r.Err == nil && !r.CertOK },
		"self-signed.badssl.com":   func(r Result) bool { return r.Err == nil && !r.CertOK },
	} {
		r := c.Check(ctx, target)
		t.Logf("%-28s TLS1.3=%v H2=%v X25519=%v cert=%v (%v) err=%v", target, r.TLS13, r.H2, r.X25519, r.CertOK, r.CertErr, r.Err)
		if !check(r) || r.Usable() {
			t.Errorf("%s: unexpected result %+v", target, r)
		}
	}
}
