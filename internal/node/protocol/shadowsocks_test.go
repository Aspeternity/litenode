package protocol

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
)

// Keys from "openssl rand -base64 16", the generator the Xray docs suggest
// for 2022-blake3-aes-128-gcm.
const (
	ssServerKey = "Q1vZgt0nPdtNEyxt0LUGsw=="
	ssUserKey   = "gB+4Ioqq1ywQlfhxV0QbJg=="
	ssOffKey    = "HDf4Gc/OKTXkvWiqkHOd4g=="
)

func ssNode() node.Node {
	return node.Node{ID: 1, Protocol: "shadowsocks", Transport: "tcp", Security: "none",
		Shadowsocks: &node.Shadowsocks{Method: SS2022Method, ServerKey: ssServerKey},
		Clients: []node.Client{
			{ID: 2, Remark: "me", Key: ssUserKey, Enabled: true},
			{ID: 3, Remark: "off", Key: ssOffKey, Enabled: false},
		}}
}

func TestSS2022Keys(t *testing.T) {
	for range 20 {
		k, err := NewSS2022Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		b, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(b) != 16 || len(k) != 24 || !ValidSS2022Key(k) {
			t.Fatalf("key %q: %d bytes, %v", k, len(b), err)
		}
	}
	if k, _ := NewSS2022Key(bytes.NewReader(make([]byte, 16))); k != "AAAAAAAAAAAAAAAAAAAAAA==" {
		t.Errorf("zero key = %q", k)
	}
	if _, err := NewSS2022Key(bytes.NewReader(make([]byte, 15))); err == nil {
		t.Error("short random source accepted")
	}
	for _, k := range []string{ssServerKey, ssUserKey, ssOffKey} {
		if !ValidSS2022Key(k) {
			t.Errorf("openssl key %q rejected", k)
		}
	}
	for _, bad := range []string{
		"",
		"ZBrUwP5/c8q05B1ZOtjGqparGNN2NCtw0kWvIG96e6o=", // 32 bytes (aes-256)
		"Q1vZgt0nPdtNEyxt0LUGsw",                       // no padding
		"HDf4Gc_OKTXkvWiqkHOd4g==",                     // URL-safe alphabet
		"Q1vZgt0nPdtNEyxt0LUGs w==",
		"5783a3e7-e373-51cd-8642-c83782b807c5", // a UUID
	} {
		if ValidSS2022Key(bad) {
			t.Errorf("key %q accepted", bad)
		}
	}
}

func TestSSInboundSettings(t *testing.T) {
	got, err := Shadowsocks{}.InboundSettings(ssNode())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(got)
	want := `{"method":"2022-blake3-aes-128-gcm","password":"Q1vZgt0nPdtNEyxt0LUGsw==",` +
		`"clients":[{"password":"gB+4Ioqq1ywQlfhxV0QbJg==","email":"me"}],"network":"tcp,udp"}`
	if string(data) != want {
		t.Errorf("settings = %s\nwant %s", data, want)
	}

	for name, mutate := range map[string]func(n *node.Node){
		"no settings":    func(n *node.Node) { n.Shadowsocks = nil },
		"other method":   func(n *node.Node) { n.Shadowsocks.Method = "2022-blake3-aes-256-gcm" },
		"bad server key": func(n *node.Node) { n.Shadowsocks.ServerKey = "short" },
		"bad user key":   func(n *node.Node) { n.Clients[0].Key = "" },
	} {
		n := ssNode()
		s := *n.Shadowsocks
		n.Shadowsocks = &s
		n.Clients = append([]node.Client(nil), n.Clients...)
		mutate(&n)
		if _, err := (Shadowsocks{}).InboundSettings(n); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
