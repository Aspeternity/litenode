package builtin

import (
	"crypto/rand"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
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
