package protocol

import (
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
)

// From "xray vlessenc" (v26.10.17) on the test VPS, X25519 authentication.
const (
	xrayDecryption = "mlkem768x25519plus.native.600s.6Eaa3cii4SQoHqeVHDrJ1XTamvh74GFemgtE1-ONWk4"
	xrayEncryption = "mlkem768x25519plus.native.0rtt.4xx3hMnM-YmlYM_RE17rfPlYKg7DAOcuJVKRpGi9PF4"
)

func encNode() node.Node {
	return node.Node{ID: 1, Protocol: "vless", Transport: "tcp", Security: "none",
		VLESSEncryption: &node.VLESSEncryption{
			PrivateKey: "6Eaa3cii4SQoHqeVHDrJ1XTamvh74GFemgtE1-ONWk4",
			PublicKey:  "4xx3hMnM-YmlYM_RE17rfPlYKg7DAOcuJVKRpGi9PF4",
		},
		Clients: []node.Client{
			{ID: 2, Remark: "me", UUID: "5783a3e7-e373-51cd-8642-c83782b807c5", Flow: node.FlowVision, Enabled: true},
			{ID: 3, Remark: "off", UUID: "0e9e4b3c-7a4e-4e8a-9d1e-2c3f6b7a8d90", Flow: node.FlowVision, Enabled: false},
		}}
}

func TestVLESSEncStringsMatchXray(t *testing.T) {
	n := encNode()
	if dec, err := Decryption(n); err != nil || dec != xrayDecryption {
		t.Errorf("Decryption = (%q, %v), want %q", dec, err, xrayDecryption)
	}
	if enc, err := Encryption(n); err != nil || enc != xrayEncryption {
		t.Errorf("Encryption = (%q, %v), want %q", enc, err, xrayEncryption)
	}
	n.VLESSEncryption = nil
	dec, _ := Decryption(n)
	enc, _ := Encryption(n)
	if dec != "none" || enc != "none" {
		t.Errorf("without encryption = %q %q, want none", dec, enc)
	}
	if _, err := Decryption(node.Node{VLESSEncryption: &node.VLESSEncryption{}}); err == nil {
		t.Error("empty private key accepted")
	}
	if _, err := Encryption(node.Node{VLESSEncryption: &node.VLESSEncryption{}}); err == nil {
		t.Error("empty public key accepted")
	}
}

func TestInboundSettingsVLESSEnc(t *testing.T) {
	got, err := VLESS{}.InboundSettings(encNode())
	if err != nil {
		t.Fatal(err)
	}
	s := got.(vlessSettings)
	if s.Decryption != xrayDecryption || len(s.Clients) != 1 || s.Clients[0].Flow != node.FlowVision {
		t.Errorf("settings = %+v", s)
	}
}
