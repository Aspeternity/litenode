package security

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Keys are encoded like "xray x25519": unpadded base64url of 32 bytes,
// 43 characters (D-047).
var keyEncoding = base64.RawURLEncoding

// NewX25519 generates a REALITY key pair and returns it encoded like the
// PrivateKey and "Password (PublicKey)" lines of "xray x25519". The
// private key is clamped (RFC 7748) before encoding, as Xray does, so
// "xray x25519 -i <private>" prints the same private key back.
func NewX25519(rand io.Reader) (private, public string, err error) {
	var raw [32]byte
	if _, err := io.ReadFull(rand, raw[:]); err != nil {
		return "", "", fmt.Errorf("generate x25519 key: %w", err)
	}
	clamp(&raw)
	k, err := ecdh.X25519().NewPrivateKey(raw[:])
	if err != nil {
		return "", "", fmt.Errorf("generate x25519 key: %w", err)
	}
	return keyEncoding.EncodeToString(k.Bytes()), keyEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

// clamp applies the X25519 scalar clamping. It does not change the public
// key (X25519 clamps internally anyway), only the stored form.
func clamp(k *[32]byte) {
	k[0] &= 248
	k[31] &= 127
	k[31] |= 64
}

// PublicKey derives the public key from an encoded private key.
func PublicKey(private string) (string, error) {
	raw, err := keyEncoding.DecodeString(private)
	if err != nil || len(raw) != 32 {
		return "", errors.New("invalid x25519 private key")
	}
	k, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", fmt.Errorf("invalid x25519 private key: %w", err)
	}
	return keyEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

// NewShortID returns a REALITY shortId of 8 hex digits (4 bytes).
func NewShortID(rand io.Reader) (string, error) {
	var b [4]byte
	if _, err := io.ReadFull(rand, b[:]); err != nil {
		return "", fmt.Errorf("generate short id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
