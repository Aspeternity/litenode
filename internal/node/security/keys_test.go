package security

import (
	"bytes"
	"crypto/rand"
	"regexp"
	"testing"
)

// Output of "xray x25519" from Xray v26.3.27 on the test VPS (D-047):
//
//	PrivateKey: wMRf1V1r21-0nC_s75OI24VkdDnoVs9f2Xoe8FiTj2Q
//	Password (PublicKey): v3JM76S_RKzF9DY2uwKCN6wyAoz6MOPAuUfjFc6Ks0M
const (
	xrayPrivate = "wMRf1V1r21-0nC_s75OI24VkdDnoVs9f2Xoe8FiTj2Q"
	xrayPublic  = "v3JM76S_RKzF9DY2uwKCN6wyAoz6MOPAuUfjFc6Ks0M"
)

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func TestPublicKeyMatchesXray(t *testing.T) {
	got, err := PublicKey(xrayPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if got != xrayPublic {
		t.Errorf("PublicKey = %q, want %q (from xray x25519)", got, xrayPublic)
	}
}

// From the test VPS: "xray x25519 -i" with an unclamped private key prints
// the clamped key and the same public key.
const (
	unclampedPrivate = "k4T42DOV9O6LGkv1TtHhTpkhupV7WE8zPWcla0bfHiA"
	clampedPrivate   = "kIT42DOV9O6LGkv1TtHhTpkhupV7WE8zPWcla0bfHmA"
	clampedPublic    = "PmLl1TieVQvRiaf6-OviNssJKgTGd_-seYxXbq4YuBQ"
)

func TestClampMatchesXray(t *testing.T) {
	raw, _ := keyEncoding.DecodeString(unclampedPrivate)
	var k [32]byte
	copy(k[:], raw)
	clamp(&k)
	if got := keyEncoding.EncodeToString(k[:]); got != clampedPrivate {
		t.Errorf("clamped = %q, want %q (from xray x25519 -i)", got, clampedPrivate)
	}
	for _, priv := range []string{unclampedPrivate, clampedPrivate} {
		if pub, _ := PublicKey(priv); pub != clampedPublic {
			t.Errorf("PublicKey(%q) = %q, want %q", priv, pub, clampedPublic)
		}
	}
}

func TestNewX25519(t *testing.T) {
	for range 20 {
		priv, pub, err := NewX25519(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if !keyRE.MatchString(priv) || !keyRE.MatchString(pub) {
			t.Fatalf("keys not 43-char base64url: %q %q", priv, pub)
		}
		if derived, _ := PublicKey(priv); derived != pub {
			t.Fatalf("public key does not match private key")
		}
		raw, _ := keyEncoding.DecodeString(priv)
		if raw[0]&7 != 0 || raw[31]&128 != 0 || raw[31]&64 == 0 {
			t.Fatalf("private key %q is not clamped", priv)
		}
	}
}

func TestPublicKeyRejectsBadInput(t *testing.T) {
	for _, bad := range []string{"", "short", xrayPrivate + "A", "wMRf1V1r21+0nC/s75OI24VkdDnoVs9f2Xoe8FiTj2Q", xrayPrivate[:42] + "="} {
		if _, err := PublicKey(bad); err == nil {
			t.Errorf("PublicKey(%q) accepted", bad)
		}
	}
}

func TestNewShortID(t *testing.T) {
	sid, err := NewShortID(bytes.NewReader([]byte{0xa1, 0xb2, 0xc3, 0xd4}))
	if err != nil || sid != "a1b2c3d4" {
		t.Errorf("NewShortID = (%q, %v)", sid, err)
	}
	if _, err := NewShortID(bytes.NewReader([]byte{1})); err == nil {
		t.Error("short read accepted")
	}
}
