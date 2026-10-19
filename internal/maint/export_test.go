package maint

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/node/security"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/state"
)

func realityNode(id int64, port int, remark string, clientID int64) node.Node {
	return node.Node{
		ID: id, Remark: remark, Enabled: true, Port: port,
		Protocol: "vless", Transport: "tcp", Security: "reality",
		Reality: &node.Reality{
			Target: "www.example.com:443", ServerNames: []string{"www.example.com"},
			PrivateKey: "wMRf1V1r21-0nC_s75OI24VkdDnoVs9f2Xoe8FiTj2Q", PublicKey: "v3JM76S_RKzF9DY2uwKCN6wyAoz6MOPAuUfjFc6Ks0M",
			ShortIDs: []string{"a1b2c3d4"}, Fingerprint: "chrome",
		},
		Clients: []node.Client{{ID: clientID, Remark: remark, UUID: "5783a3e7-e373-51cd-8642-c83782b807c5", Flow: node.FlowVision, Enabled: true}},
	}
}

// fullState has every section set, including the machine's own settings.
func fullState() *state.State {
	yes := true
	st := state.New()
	st.NextID = 20
	st.Server.ShareAddresses = []string{"203.0.113.10"}
	st.Xray = state.Xray{Version: "v26.10.17", Stopped: true, OutboundIP: "ipv6_only"}
	st.SSH = state.SSH{Port: 22222, PasswordAuth: &yes, PermitRootLogin: "no"}
	st.Sysopt.BBR = true
	st.Nodes = []node.Node{realityNode(1, 443, "LA", 2)}
	st.Forwards = []forward.Forward{{ID: 3, Enabled: true, Protocol: "tcp", ListenPort: "30001", TargetHost: "198.51.100.7", TargetPort: "443"}}
	st.Firewall = state.Firewall{
		Enabled: true, BlockPing: true,
		Rules:       []state.FirewallRule{{ID: 4, Port: "8080", Protocol: "tcp", Sources: []string{"198.51.100.0/24"}}},
		AutoSources: map[string][]string{"ssh": {"192.0.2.1"}, "node:1": {"192.0.2.2"}},
	}
	return st
}

func TestExportLeavesOutMachineSettings(t *testing.T) {
	e := Build(fullState(), "v0.1.0", time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC))
	data, err := Encode(e)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, banned := range []string{"ssh", "22222", "password", "permit_root", "sysopt", "bbr", "share_address", "203.0.113.10", "outbound_ip", "stopped", "v26.10.17", `"enabled": true,
    "block`} {
		if strings.Contains(s, banned) {
			t.Errorf("export contains %q:\n%s", banned, s)
		}
	}
	for _, want := range []string{`"format": "litenode-export"`, `"schema_version": 1`, `"litenode_version": "v0.1.0"`, `"exported_at": "2026-10-19T08:00:00Z"`, `"private_key"`, `"node:1"`, `"block_ping": true`, `"8080"`} {
		if !strings.Contains(s, want) {
			t.Errorf("export missing %q", want)
		}
	}
	reg, _ := builtin.Registry()
	back, err := Decode(data, reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := Summarize(back); got != (Summary{Nodes: 1, Clients: 1, Forwards: 1, Rules: 1}) {
		t.Errorf("summary = %+v", got)
	}
}

func TestDecodeMigratesOldVersion(t *testing.T) {
	e := Build(fullState(), "v0.0.9", time.Now())
	data, _ := Encode(e)
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	// A version 0 file named the forward list "port_forwards".
	doc["schema_version"] = 0
	doc["port_forwards"] = doc["forwards"]
	delete(doc, "forwards")
	old, _ := json.Marshal(doc)
	table := map[int]state.Migration{0: func(d map[string]any) error {
		d["forwards"] = d["port_forwards"]
		delete(d, "port_forwards")
		return nil
	}}
	reg, _ := builtin.Registry()
	got, err := Decode(old, reg, table)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Forwards) != 1 || got.SchemaVersion != state.SchemaVersion {
		t.Errorf("not migrated: %+v", got)
	}
	// Without the migration the version 0 file cannot be read.
	if _, err := Decode(old, reg, map[int]state.Migration{}); err == nil {
		t.Error("missing migration accepted")
	}
	doc["schema_version"] = state.SchemaVersion + 1
	newer, _ := json.Marshal(doc)
	var tooNew *state.TooNewError
	if _, err := Decode(newer, reg, nil); !errors.As(err, &tooNew) {
		t.Errorf("newer file: err = %v", err)
	}
}

func TestDecodeRejects(t *testing.T) {
	reg, _ := builtin.Registry()
	if _, err := Decode([]byte(`{"schema_version":1,"nodes":[]}`), reg, nil); !errors.Is(err, ErrNotExport) {
		t.Errorf("state file: err = %v", err)
	}
	if _, err := Decode([]byte("not json"), reg, nil); !errors.Is(err, ErrNotExport) {
		t.Errorf("garbage: err = %v", err)
	}
	for name, edit := range map[string]func(e *Export){
		"duplicate id": func(e *Export) { e.Forwards[0].ID = 1 },
		"same port":    func(e *Export) { e.Forwards[0].ListenPort = "443" },
		"bad node":     func(e *Export) { e.Nodes[0].Reality.PrivateKey = "x" },
		"bad vless enc keys": func(e *Export) {
			e.Nodes[0].Security, e.Nodes[0].Reality = "none", nil
			e.Nodes[0].VLESSEncryption = &node.VLESSEncryption{PrivateKey: "wMRf1V1r21-0nC_s75OI24VkdDnoVs9f2Xoe8FiTj2Q", PublicKey: "4xx3hMnM-YmlYM_RE17rfPlYKg7DAOcuJVKRpGi9PF4"}
		},
		"none without enc":   func(e *Export) { e.Nodes[0].Security, e.Nodes[0].Reality = "none", nil },
		"bad port":           func(e *Export) { e.Nodes[0].Port = 70000 },
		"bad uuid":           func(e *Export) { e.Nodes[0].Clients[0].UUID = "u" },
		"bad forward":        func(e *Export) { e.Forwards[0].Protocol = "sctp" },
		"bad rule":           func(e *Export) { e.Firewall.Rules[0].Port = "1-x" },
		"bad source":         func(e *Export) { e.Firewall.Rules[0].Sources = []string{"nope"} },
		"ssh source limit":   func(e *Export) { e.Firewall.AutoSources = map[string][]string{"ssh": {"192.0.2.1"}} },
		"duplicate remark":   func(e *Export) { n := realityNode(9, 8443, "la", 10); e.Nodes = append(e.Nodes, n) },
		"remark with spaces": func(e *Export) { e.Nodes[0].Remark = " LA" },
	} {
		e := Build(fullState(), "v", time.Now())
		edit(&e)
		data, _ := Encode(e)
		if _, err := Decode(data, reg, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestReplaceKeepsMachineSettings(t *testing.T) {
	src := fullState()
	src.Nodes[0].ID, src.Nodes[0].Clients[0].ID = 40, 41
	src.Firewall.AutoSources = map[string][]string{"node:40": {"192.0.2.9"}}
	e := Build(src, "v", time.Now())

	local := state.New()
	local.NextID = 7
	local.SSH.Port = 2222
	local.Server.ShareAddresses = []string{"192.0.2.50"}
	local.Xray = state.Xray{Version: "v26.10.17", OutboundIP: "prefer_ipv4"}
	local.Sysopt.BBR = true
	local.Firewall = state.Firewall{Enabled: false, AutoSources: map[string][]string{"ssh": {"192.0.2.77"}, "node:5": {"192.0.2.5"}}}
	local.Nodes = []node.Node{realityNode(5, 8443, "old", 6)}

	got := Replace(local, e)
	if len(got.Nodes) != 1 || got.Nodes[0].ID != 40 || len(got.Forwards) != 1 || len(got.Firewall.Rules) != 1 || !got.Firewall.BlockPing {
		t.Errorf("content not replaced: %+v", got)
	}
	if got.SSH.Port != 2222 || got.Server.ShareAddresses[0] != "192.0.2.50" || got.Xray.OutboundIP != "prefer_ipv4" || got.Xray.Version != "v26.10.17" || !got.Sysopt.BBR || got.Firewall.Enabled {
		t.Errorf("machine settings changed: %+v", got)
	}
	if src := got.Firewall.AutoSources; len(src) != 2 || src["ssh"][0] != "192.0.2.77" || src["node:40"][0] != "192.0.2.9" {
		t.Errorf("auto sources = %v", src)
	}
	if got.NextID != 42 {
		t.Errorf("next_id = %d, want 42", got.NextID)
	}
	if local.Nodes[0].ID != 5 {
		t.Error("Replace changed its input")
	}
}

func TestConflicts(t *testing.T) {
	e := Build(fullState(), "v", time.Now())
	current := state.New()
	current.Nodes = []node.Node{realityNode(8, 443, "x", 9)}
	listening := map[ports.Key]bool{{Proto: ports.TCP, Port: 443}: true, {Proto: ports.TCP, Port: 30001}: true, {Proto: ports.TCP, Port: 22}: true}
	got := Conflicts(e, current, listening)
	// 443 is the current node's Xray, which the import replaces.
	if len(got) != 1 || got[0].Port != 30001 || got[0].Owner != "forward:3" {
		t.Errorf("conflicts = %+v", got)
	}
}

func ssNode(id int64, port int, remark string, clientID int64) node.Node {
	return node.Node{
		ID: id, Remark: remark, Enabled: true, Port: port,
		Protocol: "shadowsocks", Transport: "tcp", Security: "none",
		Shadowsocks: &node.Shadowsocks{Method: "2022-blake3-aes-128-gcm", ServerKey: "Q1vZgt0nPdtNEyxt0LUGsw=="},
		Clients:     []node.Client{{ID: clientID, Remark: remark, Key: "gB+4Ioqq1ywQlfhxV0QbJg==", Enabled: false}},
	}
}

func TestDecodeShadowsocks(t *testing.T) {
	reg, _ := builtin.Registry()
	build := func(edit func(n *node.Node)) []byte {
		st := fullState()
		n := ssNode(9, 34567, "SS-34567", 10)
		edit(&n)
		st.Nodes = append(st.Nodes, n)
		data, _ := Encode(Build(st, "v", time.Now()))
		return data
	}
	e, err := Decode(build(func(*node.Node) {}), reg, nil)
	if err != nil {
		t.Fatalf("valid shadowsocks node rejected: %v", err)
	}
	if got := e.Nodes[1]; got.Shadowsocks == nil || got.Clients[0].Key != "gB+4Ioqq1ywQlfhxV0QbJg==" || got.Clients[0].UUID != "" {
		t.Errorf("node = %+v", got)
	}
	// Disabled clients are checked too.
	for name, edit := range map[string]func(n *node.Node){
		"bad server key":   func(n *node.Node) { n.Shadowsocks.ServerKey = "Q1vZgt0nPdtNEyxt0LUGsw" },
		"aes-256 key":      func(n *node.Node) { n.Shadowsocks.ServerKey = "ZBrUwP5/c8q05B1ZOtjGqparGNN2NCtw0kWvIG96e6o=" },
		"other method":     func(n *node.Node) { n.Shadowsocks.Method = "aes-128-gcm" },
		"bad user key":     func(n *node.Node) { n.Clients[0].Key = "x" },
		"uuid instead":     func(n *node.Node) { n.Clients[0].Key, n.Clients[0].UUID = "", "5783a3e7-e373-51cd-8642-c83782b807c5" },
		"missing settings": func(n *node.Node) { n.Shadowsocks = nil },
	} {
		if _, err := Decode(build(edit), reg, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestDecodeHysteria(t *testing.T) {
	reg, _ := builtin.Registry()
	tl, err := security.NewTLS(rand.Reader, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	other, _ := security.NewTLS(rand.Reader, time.Now())
	build := func(edit func(n *node.Node)) []byte {
		st := fullState()
		n := node.Node{
			ID: 9, Remark: "Hy2-45678", Enabled: true, Port: 45678,
			Protocol: "hysteria", Transport: "hysteria", Security: "tls", TLS: &node.TLS{},
			Clients: []node.Client{{ID: 10, Remark: "Hy2-45678", Key: "0123456789abcdef0123456789abcdef"}},
		}
		*n.TLS = *tl
		edit(&n)
		st.Nodes = append(st.Nodes, n)
		data, _ := Encode(Build(st, "v", time.Now()))
		return data
	}
	e, err := Decode(build(func(*node.Node) {}), reg, nil)
	if err != nil {
		t.Fatalf("valid hysteria node rejected: %v", err)
	}
	if got := e.Nodes[1]; got.TLS == nil || got.TLS.Certificate != tl.Certificate || got.TLS.Key != tl.Key {
		t.Errorf("node = %+v", got)
	}
	for name, edit := range map[string]func(n *node.Node){
		"bad password":  func(n *node.Node) { n.Clients[0].Key = "x" },
		"uuid instead":  func(n *node.Node) { n.Clients[0].Key, n.Clients[0].UUID = "", "5783a3e7-e373-51cd-8642-c83782b807c5" },
		"key mismatch":  func(n *node.Node) { n.TLS.Key = other.Key },
		"bad cert":      func(n *node.Node) { n.TLS.Certificate = "x" },
		"no tls":        func(n *node.Node) { n.TLS = nil },
		"no servername": func(n *node.Node) { n.TLS.ServerName = "" },
	} {
		if _, err := Decode(build(edit), reg, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}
