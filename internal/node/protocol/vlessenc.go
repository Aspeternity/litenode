package protocol

import (
	"errors"

	"github.com/Aspeternity/litenode/internal/node"
)

// VLESS Encryption parameters (D-107), the same as the defaults printed by
// "xray vlessenc": native appearance, 600 s server 0-RTT tickets and 0-RTT
// on the client, X25519 authentication.
const (
	vlessEncPrefix    = "mlkem768x25519plus.native."
	vlessEncServerTTL = "600s"
	vlessEncClientRTT = "0rtt"
)

// Decryption is the server's "decryption" value: "none" without VLESS
// Encryption.
func Decryption(n node.Node) (string, error) {
	e := n.VLESSEncryption
	if e == nil {
		return "none", nil
	}
	if e.PrivateKey == "" {
		return "", errors.New("vless encryption private key missing")
	}
	return vlessEncPrefix + vlessEncServerTTL + "." + e.PrivateKey, nil
}

// Encryption is the client's "encryption" value, as put in share links:
// "none" without VLESS Encryption.
func Encryption(n node.Node) (string, error) {
	e := n.VLESSEncryption
	if e == nil {
		return "none", nil
	}
	if e.PublicKey == "" {
		return "", errors.New("vless encryption public key missing")
	}
	return vlessEncPrefix + vlessEncClientRTT + "." + e.PublicKey, nil
}
