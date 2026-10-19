package node

import (
	"errors"
	"slices"
	"testing"
)

// fake implements every component interface with empty settings.
type fake struct{ name string }

func (f fake) Name() string                                     { return f.name }
func (f fake) Label() string                                    { return f.name }
func (f fake) InboundSettings(Node) (any, error)                { return nil, nil }
func (f fake) Network() string                                  { return f.name }
func (f fake) StreamSettings(Node) (string, any, error)         { return "", nil, nil }
func (f fake) StreamSecurity(Node) (string, string, any, error) { return f.name, "", nil, nil }
func (f fake) ShareParams(Node) ([]Param, error)                { return nil, nil }
func (f fake) ShareLink(Node, Client, string, int, []Param) (string, error) {
	return "", nil
}

// testRegistry has vless/trojan/shadowsocks/hysteria, tcp/xhttp/hysteria, reality/tls/none and a small
// compatibility table, independent of the built-in components.
func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.AddProtocol(fake{"vless"}))
	must(r.AddProtocol(fake{"trojan"}))
	must(r.AddProtocol(fake{"shadowsocks"}))
	must(r.AddProtocol(fake{"hysteria"}))
	must(r.AddTransport(fake{"hysteria"}))
	must(r.AddTransport(fake{"tcp"}))
	must(r.AddTransport(fake{"xhttp"}))
	must(r.AddSecurity(fake{"reality"}))
	must(r.AddSecurity(fake{"tls"}))
	must(r.AddSecurity(fake{"none"}))
	must(r.AddCompat(Compat{Protocol: "vless", Transport: "tcp", Security: "reality", Flows: []string{FlowVision, ""}}))
	must(r.AddCompat(Compat{Protocol: "vless", Transport: "xhttp", Security: "reality", Flows: []string{""}}))
	must(r.AddCompat(Compat{Protocol: "trojan", Transport: "tcp", Security: "tls", Flows: []string{""}}))
	must(r.AddCompat(Compat{Protocol: "vless", Transport: "tcp", Security: "none", Flows: []string{FlowVision, ""}}))
	must(r.AddCompat(Compat{Protocol: "trojan", Transport: "tcp", Security: "none", Flows: []string{""}}))
	must(r.AddCompat(Compat{Protocol: "shadowsocks", Transport: "tcp", Security: "none", Flows: []string{""}}))
	must(r.AddCompat(Compat{Protocol: "hysteria", Transport: "hysteria", Security: "tls", Flows: []string{""}}))
	return r
}

func TestCheckRejectsIllegalCombos(t *testing.T) {
	r := testRegistry(t)
	legal := []Combo{
		{"vless", "tcp", "reality", FlowVision},
		{"vless", "tcp", "reality", ""},
		{"vless", "xhttp", "reality", ""},
		{"trojan", "tcp", "tls", ""},
	}
	for _, c := range legal {
		if err := r.Check(c); err != nil {
			t.Errorf("Check(%+v) = %v, want legal", c, err)
		}
	}
	illegal := map[string]Combo{
		"vision over xhttp":   {"vless", "xhttp", "reality", FlowVision},
		"vision with trojan":  {"trojan", "tcp", "tls", FlowVision},
		"not in table":        {"vless", "xhttp", "none", ""},
		"trojan with reality": {"trojan", "tcp", "reality", ""},
		"unknown flow":        {"vless", "tcp", "reality", "xtls-rprx-direct"},
	}
	for name, c := range illegal {
		if err := r.Check(c); !errors.Is(err, ErrIllegalCombo) {
			t.Errorf("%s: Check(%+v) = %v, want ErrIllegalCombo", name, c, err)
		}
	}
	for name, c := range map[string]Combo{
		"unknown protocol":  {"vmess", "tcp", "reality", ""},
		"unknown transport": {"vless", "grpc", "reality", ""},
		"unknown security":  {"vless", "tcp", "xtls", ""},
	} {
		if err := r.Check(c); err == nil {
			t.Errorf("%s: Check(%+v) accepted", name, c)
		}
	}
}

func TestRegistrationErrors(t *testing.T) {
	r := testRegistry(t)
	if err := r.AddProtocol(fake{"vless"}); err == nil {
		t.Error("duplicate protocol accepted")
	}
	if err := r.AddTransport(fake{""}); err == nil {
		t.Error("empty name accepted")
	}
	if err := r.AddCompat(Compat{Protocol: "vmess", Transport: "tcp", Security: "tls"}); err == nil {
		t.Error("compat with unknown component accepted")
	}
	if err := r.AddPreset(Preset{ID: "bad", Combo: Combo{"vless", "xhttp", "reality", FlowVision}}); err == nil {
		t.Error("preset with illegal combo accepted")
	}
	p := Preset{ID: "p", Combo: Combo{"vless", "tcp", "reality", ""}}
	if err := r.AddPreset(p); err != nil {
		t.Fatal(err)
	}
	if err := r.AddPreset(p); err == nil {
		t.Error("duplicate preset ID accepted")
	}
	if err := r.AddPreset(Preset{Combo: p.Combo}); err == nil {
		t.Error("preset without ID accepted")
	}
}

func TestPresetsComeFromRegistry(t *testing.T) {
	r := testRegistry(t)
	for _, p := range []Preset{
		{ID: "trojan-tls", Combo: Combo{"trojan", "tcp", "tls", ""}},
		{ID: "vless-xhttp", Combo: Combo{"vless", "xhttp", "reality", ""}},
		{ID: "vless-vision", Combo: Combo{"vless", "tcp", "reality", FlowVision}},
	} {
		if err := r.AddPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	for _, p := range r.Presets() {
		ids = append(ids, p.ID)
	}
	// Registration order (D-060: no recommended preset).
	if want := []string{"trojan-tls", "vless-xhttp", "vless-vision"}; !slices.Equal(ids, want) {
		t.Errorf("Presets() = %v, want %v", ids, want)
	}
	if p, ok := r.Preset("vless-xhttp"); !ok || p.Combo.Transport != "xhttp" {
		t.Errorf("Preset lookup = %+v, %v", p, ok)
	}
	if got := r.PresetFlow("vless", "tcp", "reality"); got != FlowVision {
		t.Errorf("PresetFlow(vless, tcp, reality) = %q", got)
	}
	if got := r.PresetFlow("vless", "ws", "tls"); got != "" {
		t.Errorf("PresetFlow without preset = %q", got)
	}
	if _, ok := r.Preset("missing"); ok {
		t.Error("missing preset found")
	}
	if len(NewRegistry().Presets()) != 0 {
		t.Error("empty registry has presets")
	}
}

func TestCheckNode(t *testing.T) {
	r := testRegistry(t)
	n := Node{ID: 1, Protocol: "vless", Transport: "tcp", Security: "reality", Reality: &Reality{},
		Clients: []Client{{ID: 2, Flow: FlowVision}, {ID: 3, Flow: ""}}}
	if err := r.CheckNode(n); err != nil {
		t.Errorf("valid node rejected: %v", err)
	}
	bad := n
	bad.Transport = "xhttp" // Vision client not allowed over xhttp
	if err := r.CheckNode(bad); !errors.Is(err, ErrIllegalCombo) {
		t.Errorf("err = %v, want ErrIllegalCombo", err)
	}
	noSettings := n
	noSettings.Reality = nil
	if err := r.CheckNode(noSettings); err == nil {
		t.Error("reality node without settings accepted")
	}
	noClients := n
	noClients.Clients = nil
	if err := r.CheckNode(noClients); err == nil {
		t.Error("node without clients accepted")
	}
}

func TestCheckNodeVLESSEncryption(t *testing.T) {
	r := testRegistry(t)
	enc := Node{ID: 1, Protocol: "vless", Transport: "tcp", Security: "none",
		VLESSEncryption: &VLESSEncryption{PrivateKey: "p", PublicKey: "q"},
		Clients:         []Client{{ID: 2, Flow: FlowVision}}}
	if err := r.CheckNode(enc); err != nil {
		t.Errorf("valid node rejected: %v", err)
	}
	plain := enc
	plain.VLESSEncryption = nil
	if err := r.CheckNode(plain); err == nil {
		t.Error("vless + none without encryption accepted")
	}
	trojan := enc
	trojan.Protocol = "trojan"
	trojan.Clients = []Client{{ID: 2}}
	if err := r.CheckNode(trojan); err == nil {
		t.Error("vless encryption on trojan accepted")
	}
}

func TestCheckNodeTLS(t *testing.T) {
	r := testRegistry(t)
	hy := Node{ID: 1, Protocol: "hysteria", Transport: "hysteria", Security: "tls",
		TLS: &TLS{ServerName: "s", Certificate: "c", Key: "k"}, Clients: []Client{{ID: 2, Key: "p"}}}
	if err := r.CheckNode(hy); err != nil {
		t.Errorf("valid node rejected: %v", err)
	}
	noTLS := hy
	noTLS.TLS = nil
	if err := r.CheckNode(noTLS); err == nil {
		t.Error("tls node without settings accepted")
	}
	onTCP := hy
	onTCP.Transport = "tcp"
	if err := r.CheckNode(onTCP); !errors.Is(err, ErrIllegalCombo) {
		t.Errorf("hysteria over tcp: err = %v, want ErrIllegalCombo", err)
	}
	reality := Node{ID: 1, Protocol: "vless", Transport: "tcp", Security: "reality", Reality: &Reality{},
		TLS: hy.TLS, Clients: []Client{{ID: 2}}}
	if err := r.CheckNode(reality); err == nil {
		t.Error("tls settings on a reality node accepted")
	}
}

func TestCheckNodeShadowsocks(t *testing.T) {
	r := testRegistry(t)
	ss := Node{ID: 1, Protocol: "shadowsocks", Transport: "tcp", Security: "none",
		Shadowsocks: &Shadowsocks{Method: "m", ServerKey: "k"},
		Clients:     []Client{{ID: 2, Key: "u"}}}
	if err := r.CheckNode(ss); err != nil {
		t.Errorf("valid node rejected: %v", err)
	}
	noSettings := ss
	noSettings.Shadowsocks = nil
	if err := r.CheckNode(noSettings); err == nil {
		t.Error("shadowsocks node without settings accepted")
	}
	vision := ss
	vision.Clients = []Client{{ID: 2, Key: "u", Flow: FlowVision}}
	if err := r.CheckNode(vision); !errors.Is(err, ErrIllegalCombo) {
		t.Errorf("vision on shadowsocks: err = %v, want ErrIllegalCombo", err)
	}
	vless := Node{ID: 1, Protocol: "vless", Transport: "tcp", Security: "none",
		VLESSEncryption: &VLESSEncryption{PrivateKey: "p", PublicKey: "q"},
		Shadowsocks:     &Shadowsocks{Method: "m", ServerKey: "k"},
		Clients:         []Client{{ID: 2, Flow: FlowVision}}}
	if err := r.CheckNode(vless); err == nil {
		t.Error("shadowsocks settings on vless accepted")
	}
}
