package security

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/node"
)

// testdata/openssl-*.pem were made on the test VPS with
//
//	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
//	  -days 3650 -subj /CN=litenode -keyout key.pem -out cert.pem
//
// and the hashes below with "openssl x509 -fingerprint -sha256" and
// "openssl x509 -pubkey | openssl pkey -pubin -outform der | openssl
// dgst -sha256 -binary | base64".
const (
	opensslCertHex   = "86c5ac5b87fba95509e334af4111e01513bcdf1b2fa29976af1bac7127b3cbd1"
	opensslPubBase64 = "7uP4YvWbazZ/Gy3uqgV/UAJjjzXT6QeGdCc0UP7pfqw="
)

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFingerprintsMatchOpenSSL(t *testing.T) {
	fp, err := CertFingerprints(readFile(t, "testdata/openssl-cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if fp.CertHex() != opensslCertHex || fp.PublicKeyBase64() != opensslPubBase64 {
		t.Errorf("fingerprints = %s %s", fp.CertHex(), fp.PublicKeyBase64())
	}
}

func TestCheckTLS(t *testing.T) {
	cert, key := readFile(t, "testdata/openssl-cert.pem"), readFile(t, "testdata/openssl-key.pem")
	if err := CheckTLS(&node.TLS{ServerName: "litenode", Certificate: cert, Key: key}); err != nil {
		t.Fatalf("openssl pair rejected: %v", err)
	}
	_, otherKey, err := NewSelfSignedCert(rand.Reader, "x", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]*node.TLS{
		"nil":        nil,
		"no name":    {Certificate: cert, Key: key},
		"bad cert":   {ServerName: "x", Certificate: "junk", Key: key},
		"key as pem": {ServerName: "x", Certificate: key, Key: key},
		"other key":  {ServerName: "x", Certificate: cert, Key: otherKey},
	} {
		if CheckTLS(tc) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNewSelfSignedCert(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tl, err := NewTLS(rand.Reader, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckTLS(tl); err != nil {
		t.Fatal(err)
	}
	cert, _ := ParseCert(tl.Certificate)
	if cert.Subject.CommonName != DefaultServerName || len(cert.DNSNames) != 1 || cert.DNSNames[0] != DefaultServerName {
		t.Errorf("names = %q %q", cert.Subject.CommonName, cert.DNSNames)
	}
	if !cert.NotBefore.Equal(now.Add(-24*time.Hour)) || !cert.NotAfter.Equal(now.Add(10*365*24*time.Hour)) {
		t.Errorf("validity = %v .. %v", cert.NotBefore, cert.NotAfter)
	}
	if pub, ok := cert.PublicKey.(*ecdsa.PublicKey); !ok || pub.Curve.Params().Name != "P-256" {
		t.Errorf("public key = %T", cert.PublicKey)
	}
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Errorf("not self-signed: %v", err)
	}
	if !strings.HasPrefix(tl.Key, "-----BEGIN PRIVATE KEY-----\n") {
		t.Errorf("key is not PKCS #8 PEM")
	}
	other, _ := NewTLS(rand.Reader, now)
	a, _ := CertFingerprints(tl.Certificate)
	b, _ := CertFingerprints(other.Certificate)
	if a == b || cert.SerialNumber.Sign() <= 0 {
		t.Error("certificates are not unique")
	}
}

func TestTLSStreamSecurity(t *testing.T) {
	cert, key := readFile(t, "testdata/openssl-cert.pem"), readFile(t, "testdata/openssl-key.pem")
	n := node.Node{TLS: &node.TLS{ServerName: "litenode", Certificate: cert, Key: key}}
	sec, k, v, err := TLS{}.StreamSecurity(n)
	if err != nil || sec != "tls" || k != "tlsSettings" {
		t.Fatalf("%q %q %v", sec, k, err)
	}
	b, _ := json.Marshal(v)
	var got struct {
		Certificates []struct{ Certificate, Key []string }
	}
	if err := json.Unmarshal(b, &got); err != nil || len(got.Certificates) != 1 {
		t.Fatalf("%s: %v", b, err)
	}
	c := got.Certificates[0]
	if strings.Join(c.Certificate, "\n")+"\n" != cert || strings.Join(c.Key, "\n")+"\n" != key ||
		c.Certificate[len(c.Certificate)-1] != "-----END CERTIFICATE-----" {
		t.Errorf("lines do not rebuild the PEM: %s", b)
	}
	params, err := TLS{}.ShareParams(n)
	if err != nil || len(params) != 2 || params[0] != (node.Param{Key: "sni", Value: "litenode"}) ||
		params[1] != (node.Param{Key: "pinSHA256", Value: opensslCertHex}) {
		t.Errorf("params = %v, %v", params, err)
	}
	if _, _, _, err := (TLS{}).StreamSecurity(node.Node{}); err == nil {
		t.Error("missing settings accepted")
	}
}
