package xray

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/builtin"
)

var update = flag.Bool("update", false, "rewrite golden files")

// testPrivateKey is a real key pair from "xray x25519" (v26.3.27).
const testPrivateKey = "wMRf1V1r21-0nC_s75OI24VkdDnoVs9f2Xoe8FiTj2Q"

func realityNode(id int64, port int, target string) node.Node {
	host, _, _ := strings.Cut(target, ":")
	return node.Node{
		ID: id, Remark: "node", Enabled: true, Port: port,
		Protocol: "vless", Transport: "tcp", Security: "reality",
		Reality: &node.Reality{
			Target: target, ServerNames: []string{host},
			PrivateKey: testPrivateKey, PublicKey: "v3JM76S_RKzF9DY2uwKCN6wyAoz6MOPAuUfjFc6Ks0M",
			ShortIDs: []string{"a1b2c3d4"}, Fingerprint: "chrome",
			FallbackLimit: node.FallbackLimit{AfterBytes: 10485760, BytesPerSec: 1048576, BurstBytesPerSec: 5242880},
		},
	}
}

func goldenNodes() []node.Node {
	la := realityNode(1, 443, "www.example-a.com:443")
	la.Remark = "LA-Reality"
	la.Clients = []node.Client{
		{ID: 2, Remark: "me", UUID: "5783a3e7-e373-51cd-8642-c83782b807c5", Flow: node.FlowVision, Enabled: true},
		{ID: 3, Remark: "phone", UUID: "0e9e4b3c-7a4e-4e8a-9d1e-2c3f6b7a8d90", Flow: node.FlowVision, Enabled: false},
		{ID: 4, Remark: "tablet", UUID: "c2a4f0de-1b2c-4d3e-8f90-a1b2c3d4e5f6", Flow: node.FlowVision, Enabled: true},
	}

	disabled := realityNode(5, 8443, "www.example-b.com:443")
	disabled.Remark = "off"
	disabled.Enabled = false
	disabled.Clients = []node.Client{{ID: 6, Remark: "x", UUID: "11111111-2222-4333-8444-555555555555", Flow: node.FlowVision, Enabled: true}}

	noClients := realityNode(7, 9443, "www.example-c.com:443")
	noClients.Remark = "no-clients"
	noClients.Clients = []node.Client{{ID: 8, Remark: "y", UUID: "66666666-7777-4888-8999-aaaaaaaaaaaa", Flow: node.FlowVision, Enabled: false}}

	noLimit := realityNode(9, 2053, "www.example-d.com:443")
	noLimit.Remark = "no-limit"
	noLimit.Reality.ShortIDs = []string{"", "0123456789abcdef"}
	noLimit.Reality.FallbackLimit = node.FallbackLimit{}
	noLimit.Clients = []node.Client{{ID: 10, Remark: "plain", UUID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff", Flow: "", Enabled: true}}

	return []node.Node{la, disabled, noClients, noLimit}
}

// vlessEncNode uses the key pair of a real "xray vlessenc" output (D-107).
func vlessEncNode() node.Node {
	return node.Node{
		ID: 11, Remark: "Enc-23456", Enabled: true, Port: 23456,
		Protocol: "vless", Transport: "tcp", Security: "none",
		VLESSEncryption: &node.VLESSEncryption{
			PrivateKey: "6Eaa3cii4SQoHqeVHDrJ1XTamvh74GFemgtE1-ONWk4",
			PublicKey:  "4xx3hMnM-YmlYM_RE17rfPlYKg7DAOcuJVKRpGi9PF4",
		},
		Clients: []node.Client{
			{ID: 12, Remark: "Enc-23456", UUID: "5783a3e7-e373-51cd-8642-c83782b807c5", Flow: node.FlowVision, Enabled: true},
			{ID: 13, Remark: "enc-off", UUID: "0e9e4b3c-7a4e-4e8a-9d1e-2c3f6b7a8d90", Flow: node.FlowVision, Enabled: false},
		},
	}
}

// ss2022Node uses keys from "openssl rand -base64 16" (DESIGN section 9.8).
func ss2022Node() node.Node {
	return node.Node{
		ID: 14, Remark: "SS-34567", Enabled: true, Port: 34567,
		Protocol: "shadowsocks", Transport: "tcp", Security: "none",
		Shadowsocks: &node.Shadowsocks{Method: "2022-blake3-aes-128-gcm", ServerKey: "Q1vZgt0nPdtNEyxt0LUGsw=="},
		Clients: []node.Client{
			{ID: 15, Remark: "SS-34567", Key: "gB+4Ioqq1ywQlfhxV0QbJg==", Enabled: true},
			{ID: 16, Remark: "ss-off", Key: "HDf4Gc/OKTXkvWiqkHOd4g==", Enabled: false},
		},
	}
}

// hysteriaNode uses the openssl certificate of the security package's
// test data (DESIGN section 9.10).
func hysteriaNode(t *testing.T) node.Node {
	t.Helper()
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "node", "security", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return node.Node{
		ID: 17, Remark: "Hy2-45678", Enabled: true, Port: 45678,
		Protocol: "hysteria", Transport: "hysteria", Security: "tls",
		TLS: &node.TLS{ServerName: "www.bing.com", Certificate: read("openssl-cert.pem"), Key: read("openssl-key.pem")},
		Clients: []node.Client{
			{ID: 18, Remark: "Hy2-45678", Key: "0123456789abcdef0123456789abcdef", Enabled: true},
			{ID: 19, Remark: "hy-off", Key: "ffffffffffffffffffffffffffffffff", Enabled: false},
		},
	}
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s mismatch\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestGenerateConfigGolden(t *testing.T) {
	reg, err := builtin.Registry()
	if err != nil {
		t.Fatal(err)
	}
	for name, nodes := range map[string][]node.Node{
		"xray-empty.golden.json": nil,
		"xray-nodes.golden.json": goldenNodes(),
		// A VLESS Encryption node next to the Reality nodes, which stay
		// as in xray-nodes.golden.json.
		"xray-vlessenc.golden.json": append(goldenNodes(), vlessEncNode()),
		// A Shadowsocks 2022 node next to the other nodes, which stay as
		// in xray-vlessenc.golden.json.
		"xray-ss2022.golden.json": append(goldenNodes(), vlessEncNode(), ss2022Node()),
		// A Hysteria2 node after all the others, which stay as in
		// xray-ss2022.golden.json.
		"xray-hysteria2.golden.json": append(goldenNodes(), vlessEncNode(), ss2022Node(), hysteriaNode(t)),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := GenerateConfig(reg, nodes, "")
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(got) {
				t.Fatal("invalid JSON")
			}
			checkGolden(t, name, got)
		})
	}
}

// TestGenerateConfigPrinciples checks DESIGN section 9.6 independently of
// the golden files.
func TestGenerateConfigPrinciples(t *testing.T) {
	reg, _ := builtin.Registry()
	data, err := GenerateConfig(reg, goldenNodes(), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, banned := range []string{"geoip", "geosite", `"stats"`, `"api"`, `"policy"`, "fingerprint", "publicKey"} {
		if strings.Contains(s, banned) {
			t.Errorf("config contains %s", banned)
		}
	}
	var cfg struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Settings struct {
				Clients []struct {
					Email string `json:"email"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
		Outbounds []struct {
			Protocol string `json:"protocol"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 2 || cfg.Inbounds[0].Tag != "node-1" || cfg.Inbounds[1].Tag != "node-9" {
		t.Errorf("inbounds = %+v, want node-1 and node-9 (disabled node and node without enabled clients skipped)", cfg.Inbounds)
	}
	if got := len(cfg.Inbounds[0].Settings.Clients); got != 2 {
		t.Errorf("node-1 has %d clients, want 2 (disabled client skipped)", got)
	}
	if len(cfg.Outbounds) != 2 || cfg.Outbounds[0].Protocol != "freedom" || cfg.Outbounds[1].Protocol != "blackhole" {
		t.Errorf("outbounds = %+v", cfg.Outbounds)
	}
}

func TestGenerateConfigRejectsInvalidNode(t *testing.T) {
	reg, _ := builtin.Registry()
	n := realityNode(1, 443, "www.example.com:443")
	n.Clients = []node.Client{{ID: 2, Remark: "me", UUID: "x", Flow: "xtls-rprx-direct", Enabled: true}}
	if _, err := GenerateConfig(reg, []node.Node{n}, ""); err == nil {
		t.Error("illegal flow accepted")
	}
	n.Clients[0].Flow = node.FlowVision
	n.Reality = nil
	if _, err := GenerateConfig(reg, []node.Node{n}, ""); err == nil {
		t.Error("reality node without settings accepted")
	}
}
