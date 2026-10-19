package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/Aspeternity/litenode/internal/node"
)

// DefaultServerName is the name in self-signed certificates and the SNI
// clients send (D-118). Clients pin the certificate, so the name is not
// verified; a common name keeps the handshake unremarkable and carries
// nothing that identifies LiteNode.
const DefaultServerName = "www.bing.com"

// CertValidity is how long a self-signed certificate is valid (D-117).
const CertValidity = 10 * 365 * 24 * time.Hour

// certBackdate starts validity a day early so a peer whose clock is
// slightly behind still accepts a new certificate.
const certBackdate = 24 * time.Hour

// NewSelfSignedCert creates an ECDSA P-256 certificate for name, valid
// for CertValidity from now, and returns it and its PKCS #8 private key
// PEM encoded. Since Go 1.26 the key itself always comes from the system
// generator; rand provides the serial number.
func NewSelfSignedCert(rand io.Reader, name string, now time.Time) (certPEM, keyPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand)
	if err != nil {
		return "", "", fmt.Errorf("generate certificate key: %w", err)
	}
	serial, err := randSerial(rand)
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		DNSNames:              []string{name},
		NotBefore:             now.Add(-certBackdate),
		NotAfter:              now.Add(CertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("encode certificate key: %w", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, nil
}

// randSerial returns a positive 128-bit serial number.
func randSerial(rand io.Reader) (*big.Int, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand, b); err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	b[0] &= 0x7f
	b[0] |= 0x40
	return new(big.Int).SetBytes(b), nil
}

// NewTLS returns TLS settings with a new self-signed certificate for
// DefaultServerName.
func NewTLS(rand io.Reader, now time.Time) (*node.TLS, error) {
	cert, key, err := NewSelfSignedCert(rand, DefaultServerName, now)
	if err != nil {
		return nil, err
	}
	return &node.TLS{ServerName: DefaultServerName, Certificate: cert, Key: key}, nil
}

// Fingerprints are the SHA-256 hashes clients pin: of the whole
// certificate (Xray pinnedPeerCertSha256, mihomo fingerprint, Hysteria
// pinSHA256) and of its public key, SubjectPublicKeyInfo (sing-box
// certificate_public_key_sha256).
type Fingerprints struct {
	Cert      [32]byte
	PublicKey [32]byte
}

// CertHex is the certificate hash as lowercase hex without colons.
func (f Fingerprints) CertHex() string { return fmt.Sprintf("%x", f.Cert) }

// PublicKeyBase64 is the public key hash in standard base64, as in HTTP
// public key pinning.
func (f Fingerprints) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(f.PublicKey[:])
}

// ParseCert decodes the first certificate of a PEM string.
func ParseCert(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	return cert, nil
}

// CertFingerprints returns the pinned hashes of a PEM certificate.
func CertFingerprints(certPEM string) (Fingerprints, error) {
	cert, err := ParseCert(certPEM)
	if err != nil {
		return Fingerprints{}, err
	}
	return Fingerprints{Cert: sha256.Sum256(cert.Raw), PublicKey: sha256.Sum256(cert.RawSubjectPublicKeyInfo)}, nil
}

// CheckTLS checks that the certificate and key parse and belong together
// (importing a backup, D-117).
func CheckTLS(t *node.TLS) error {
	if t == nil {
		return errors.New("tls settings missing")
	}
	if t.ServerName == "" {
		return errors.New("tls server name missing")
	}
	if _, err := ParseCert(t.Certificate); err != nil {
		return err
	}
	if _, err := tls.X509KeyPair([]byte(t.Certificate), []byte(t.Key)); err != nil {
		return fmt.Errorf("certificate and key do not match: %w", err)
	}
	return nil
}

// TLS is the TLS security layer with the node's own certificate.
type TLS struct{}

func (TLS) Name() string  { return "tls" }
func (TLS) Label() string { return "TLS" }

type tlsCert struct {
	Certificate []string `json:"certificate"`
	Key         []string `json:"key"`
}

type tlsSettings struct {
	Certificates []tlsCert `json:"certificates"`
}

// StreamSecurity embeds the certificate and key in the config as lines
// (D-118), so no separate files need writing; the config file is
// readable only by root and the Xray group, like the REALITY private key
// in it. The ALPN is left to the transport (Hysteria sets h3).
func (TLS) StreamSecurity(n node.Node) (string, string, any, error) {
	t := n.TLS
	if t == nil {
		return "", "", nil, errors.New("tls settings missing")
	}
	return "tls", "tlsSettings", tlsSettings{Certificates: []tlsCert{{
		Certificate: pemLines(t.Certificate),
		Key:         pemLines(t.Key),
	}}}, nil
}

// pemLines splits PEM text into lines as Xray's inline certificate
// format expects, without the empty last line.
func pemLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// ShareParams gives the SNI and the certificate hash as pinSHA256, the
// Hysteria URI name (DESIGN section 9.10).
func (TLS) ShareParams(n node.Node) ([]node.Param, error) {
	t := n.TLS
	if t == nil {
		return nil, errors.New("tls settings missing")
	}
	fp, err := CertFingerprints(t.Certificate)
	if err != nil {
		return nil, err
	}
	return []node.Param{
		{Key: "sni", Value: t.ServerName},
		{Key: "pinSHA256", Value: fp.CertHex()},
	}, nil
}
