package builtin

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/protocol"
	"github.com/Aspeternity/litenode/internal/node/security"
)

func TestEveryPresetCanBuild(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Presets() {
		if p.Build == nil {
			t.Errorf("preset %s has no Build", p.ID)
		}
	}
}

func TestBuildVLESSRealityVision(t *testing.T) {
	r, _ := Registry()
	p, _ := r.Preset(PresetVLESSRealityVision)
	n, err := p.Build(node.BuildInput{
		NodeID: 7, ClientID: 8, Port: 443, Remark: "LA-Reality",
		Target: "www.example.com:443", Rand: rand.Reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckNode(n); err != nil {
		t.Fatalf("built node is not valid: %v", err)
	}
	re := n.Reality
	if n.ID != 7 || n.Port != 443 || !n.Enabled || n.Remark != "LA-Reality" {
		t.Errorf("node = %+v", n)
	}
	if re.Target != "www.example.com:443" || len(re.ServerNames) != 1 || re.ServerNames[0] != "www.example.com" {
		t.Errorf("target / server names = %q %q", re.Target, re.ServerNames)
	}
	if pub, err := security.PublicKey(re.PrivateKey); err != nil || pub != re.PublicKey {
		t.Errorf("key pair mismatch (%v)", err)
	}
	if len(re.ShortIDs) != 1 || len(re.ShortIDs[0]) != 8 {
		t.Errorf("short ids = %q", re.ShortIDs)
	}
	if re.Fingerprint != "chrome" || re.FallbackLimit != node.DefaultFallbackLimit {
		t.Errorf("defaults = %q %+v", re.Fingerprint, re.FallbackLimit)
	}
	c := n.Clients[0]
	if len(n.Clients) != 1 || c.ID != 8 || c.Remark != "LA-Reality" || c.Flow != node.FlowVision || !c.Enabled || !node.ValidUUID(c.UUID) {
		t.Errorf("client = %+v", c)
	}

	for _, bad := range []string{"www.example.com", ":443", ""} {
		if _, err := p.Build(node.BuildInput{Target: bad, Rand: rand.Reader}); err == nil {
			t.Errorf("target %q accepted", bad)
		}
	}
}

func TestBuildVLESSEncVision(t *testing.T) {
	r, _ := Registry()
	p, _ := r.Preset(PresetVLESSEncVision)
	if p.Label != "VLESS + TCP + Encryption (Vision)" || r.PresetFlow("vless", "tcp", "none") != node.FlowVision {
		t.Errorf("preset = %+v", p)
	}
	n, err := p.Build(node.BuildInput{NodeID: 7, ClientID: 8, Port: 23456, Remark: "Enc-23456", Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckNode(n); err != nil {
		t.Fatalf("built node is not valid: %v", err)
	}
	if n.ID != 7 || n.Port != 23456 || !n.Enabled || n.Remark != "Enc-23456" || n.Security != "none" || n.Reality != nil {
		t.Errorf("node = %+v", n)
	}
	e := n.VLESSEncryption
	if e == nil {
		t.Fatal("vless encryption settings missing")
	}
	if pub, err := security.PublicKey(e.PrivateKey); err != nil || pub != e.PublicKey {
		t.Errorf("key pair mismatch (%v)", err)
	}
	c := n.Clients[0]
	if len(n.Clients) != 1 || c.ID != 8 || c.Remark != "Enc-23456" || c.Flow != node.FlowVision || !c.Enabled || !node.ValidUUID(c.UUID) {
		t.Errorf("client = %+v", c)
	}
}

func TestBuildSS2022(t *testing.T) {
	r, _ := Registry()
	p, _ := r.Preset(PresetSS2022)
	if p.Label != "Shadowsocks 2022" || r.PresetFlow("shadowsocks", "tcp", "none") != "" {
		t.Errorf("preset = %+v", p)
	}
	n, err := p.Build(node.BuildInput{NodeID: 7, ClientID: 8, Port: 34567, Remark: "SS-34567", Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckNode(n); err != nil {
		t.Fatalf("built node is not valid: %v", err)
	}
	if n.ID != 7 || n.Port != 34567 || !n.Enabled || n.Remark != "SS-34567" || n.Protocol != "shadowsocks" ||
		n.Security != "none" || n.Reality != nil || n.VLESSEncryption != nil {
		t.Errorf("node = %+v", n)
	}
	s := n.Shadowsocks
	if s == nil || s.Method != "2022-blake3-aes-128-gcm" || !protocol.ValidSS2022Key(s.ServerKey) {
		t.Fatalf("shadowsocks = %+v", s)
	}
	c := n.Clients[0]
	if len(n.Clients) != 1 || c.ID != 8 || c.Remark != "SS-34567" || c.Flow != "" || !c.Enabled ||
		c.UUID != "" || !protocol.ValidSS2022Key(c.Key) || c.Key == s.ServerKey {
		t.Errorf("client = %+v", c)
	}
}

func TestBuildHysteria2(t *testing.T) {
	r, _ := Registry()
	if err := r.Check(node.Combo{Protocol: "hysteria", Transport: "hysteria", Security: "tls"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	n, err := buildHysteria2(node.BuildInput{NodeID: 7, ClientID: 8, Port: 45678, Remark: "Hy2-45678", Rand: rand.Reader, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckNode(n); err != nil {
		t.Fatalf("built node is not valid: %v", err)
	}
	if n.ID != 7 || n.Port != 45678 || !n.Enabled || n.Remark != "Hy2-45678" || n.Protocol != "hysteria" ||
		n.Transport != "hysteria" || n.Security != "tls" || n.Reality != nil || n.Shadowsocks != nil {
		t.Errorf("node = %+v", n)
	}
	if err := security.CheckTLS(n.TLS); err != nil || n.TLS.ServerName != "www.bing.com" {
		t.Fatalf("tls = %+v: %v", n.TLS, err)
	}
	cert, _ := security.ParseCert(n.TLS.Certificate)
	if !cert.NotAfter.Equal(now.Add(security.CertValidity)) {
		t.Errorf("not after = %v", cert.NotAfter)
	}
	c := n.Clients[0]
	if len(n.Clients) != 1 || c.ID != 8 || c.Remark != "Hy2-45678" || c.Flow != "" || !c.Enabled ||
		c.UUID != "" || !protocol.ValidHysteriaPassword(c.Key) {
		t.Errorf("client = %+v", c)
	}
	if n.UsesTCP() || !n.UsesUDP() || n.PortProto() != "udp" {
		t.Errorf("ports: tcp %v udp %v proto %s", n.UsesTCP(), n.UsesUDP(), n.PortProto())
	}
}
