package share

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/builtin"
)

func testNode() node.Node {
	return node.Node{
		ID: 1, Remark: "LA-Reality", Enabled: true, Port: 443,
		Protocol: "vless", Transport: "tcp", Security: "reality",
		Reality: &node.Reality{
			Target: "www.example.com:443", ServerNames: []string{"www.example.com"},
			PrivateKey: "wMRf1V1r21-0nC_s75OI24VkdDnoVs9f2Xoe8FiTj2Q", PublicKey: "v3JM76S_RKzF9DY2uwKCN6wyAoz6MOPAuUfjFc6Ks0M",
			ShortIDs: []string{"a1b2c3d4"}, Fingerprint: "chrome",
		},
		Clients: []node.Client{{ID: 2, Remark: "me", UUID: "5783a3e7-e373-51cd-8642-c83782b807c5", Flow: node.FlowVision, Enabled: true}},
	}
}

func TestLinkMatchesDesign(t *testing.T) {
	reg, _ := builtin.Registry()
	n := testNode()
	got, err := Link(reg, n, n.Clients[0], "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	// DESIGN section 9.5.
	want := "vless://5783a3e7-e373-51cd-8642-c83782b807c5@203.0.113.10:443?encryption=none&flow=xtls-rprx-vision" +
		"&security=reality&sni=www.example.com&fp=chrome&pbk=v3JM76S_RKzF9DY2uwKCN6wyAoz6MOPAuUfjFc6Ks0M&sid=a1b2c3d4&type=tcp#me"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// encNode uses the key pair of a real "xray vlessenc" output (D-107).
func encNode() node.Node {
	return node.Node{
		ID: 3, Remark: "Enc-23456", Enabled: true, Port: 23456,
		Protocol: "vless", Transport: "tcp", Security: "none",
		VLESSEncryption: &node.VLESSEncryption{
			PrivateKey: "6Eaa3cii4SQoHqeVHDrJ1XTamvh74GFemgtE1-ONWk4",
			PublicKey:  "4xx3hMnM-YmlYM_RE17rfPlYKg7DAOcuJVKRpGi9PF4",
		},
		Clients: []node.Client{{ID: 4, Remark: "me", UUID: "5783a3e7-e373-51cd-8642-c83782b807c5", Flow: node.FlowVision, Enabled: true}},
	}
}

func TestVLESSEncLinkMatchesDesign(t *testing.T) {
	reg, _ := builtin.Registry()
	n := encNode()
	got, err := Link(reg, n, n.Clients[0], "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	// DESIGN section 9.7.
	want := "vless://5783a3e7-e373-51cd-8642-c83782b807c5@203.0.113.10:23456" +
		"?encryption=mlkem768x25519plus.native.0rtt.4xx3hMnM-YmlYM_RE17rfPlYKg7DAOcuJVKRpGi9PF4" +
		"&flow=xtls-rprx-vision&security=none&type=tcp#me"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	n.SharePort = 30001
	if got, _ := LinkFor(reg, n, n.Clients[0], "2001:db8::1", false); !strings.HasPrefix(got, "vless://5783a3e7-e373-51cd-8642-c83782b807c5@[2001:db8::1]:30001?encryption=mlkem768x25519plus.") || !strings.HasSuffix(got, "#me-IPv6") {
		t.Errorf("IPv6 / share port link = %s", got)
	}
	n.Clients[0].Flow = ""
	if got, _ := Link(reg, n, n.Clients[0], "203.0.113.10"); !strings.Contains(got, "PF4&security=none&type=tcp#") {
		t.Errorf("no-flow link = %s", got)
	}
	if qr, err := QR(want, false); err != nil || len([]rune(strings.SplitN(qr, "\n", 2)[0])) > 65 {
		t.Errorf("QR too wide or failed: %v", err)
	}
}

func TestLinkVariants(t *testing.T) {
	reg, _ := builtin.Registry()
	tests := []struct {
		name   string
		mod    func(*node.Node)
		addr   string
		checks []string
	}{
		{"IPv6 in brackets", nil, "2001:db8::1", []string{"@[2001:db8::1]:443?"}},
		{"domain", nil, "la.example.com", []string{"@la.example.com:443?"}},
		{"share port for NAT", func(n *node.Node) { n.SharePort = 30001 }, "203.0.113.10", []string{"@203.0.113.10:30001?"}},
		{"no flow", func(n *node.Node) { n.Clients[0].Flow = "" }, "203.0.113.10", []string{"?encryption=none&security=reality"}},
		{"special characters in remark", func(n *node.Node) { n.Clients[0].Remark = "洛杉矶 #1 a&b=c+d%" }, "203.0.113.10",
			[]string{"#%E6%B4%9B%E6%9D%89%E7%9F%B6%20%231%20a%26b%3Dc%2Bd%25"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode()
			if tt.mod != nil {
				tt.mod(&n)
			}
			got, err := Link(reg, n, n.Clients[0], tt.addr)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range tt.checks {
				if !strings.Contains(got, c) {
					t.Errorf("%s does not contain %s", got, c)
				}
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("link does not parse: %v", err)
			}
			if u.Fragment != n.Clients[0].Remark {
				t.Errorf("fragment decodes to %q, want %q", u.Fragment, n.Clients[0].Remark)
			}
		})
	}
	n := testNode()
	if _, err := Link(reg, n, n.Clients[0], ""); err == nil {
		t.Error("empty address accepted")
	}
}

func TestValidAddress(t *testing.T) {
	for s, want := range map[string]bool{
		"203.0.113.10": true, "2001:db8::1": true, "la.example.com": true,
		"": false, "localhost": false, "a b.com": false, "-x.com": false, "http://x.com": false,
	} {
		if ValidAddress(s) != want {
			t.Errorf("ValidAddress(%q) = %v", s, !want)
		}
	}
}

func TestQRRendering(t *testing.T) {
	text := "vless://5783a3e7-e373-51cd-8642-c83782b807c5@203.0.113.10:443?encryption=none#me"
	m, err := Matrix(text)
	if err != nil {
		t.Fatal(err)
	}
	n := len(m)
	for _, color := range []bool{true, false} {
		out, err := QR(text, color)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if len(lines) != (n+1)/2 {
			t.Fatalf("color=%v: %d lines for %d modules", color, len(lines), n)
		}
		// Decode the half blocks back into modules and compare.
		for row, line := range lines {
			line = strings.TrimPrefix(strings.TrimSuffix(line, "\x1b[0m"), "\x1b[30;47m")
			cells := []rune(line)
			if len(cells) != n {
				t.Fatalf("line %d has %d cells, want %d", row, len(cells), n)
			}
			for x, c := range cells {
				top := c == '█' || c == '▀'
				bottom := c == '█' || c == '▄'
				if !color {
					top, bottom = !top, !bottom
				}
				y := 2 * row
				if top != m[y][x] || (y+1 < n && bottom != m[y+1][x]) {
					t.Fatalf("color=%v: module mismatch at %d,%d", color, x, y)
				}
			}
		}
	}
	// The quiet zone is light.
	for x := range n {
		if m[0][x] || m[n-1][x] {
			t.Fatal("quiet zone not light")
		}
	}
	// A typical link fits an 80-column terminal.
	reg, _ := builtin.Registry()
	tn := testNode()
	link, _ := Link(reg, tn, tn.Clients[0], "2001:db8:1234:5678:9abc:def0:1234:5678")
	if lm, _ := Matrix(link); len(lm) > 80 {
		t.Errorf("QR for a long link is %d columns", len(lm))
	}
}

func TestParseTrace(t *testing.T) {
	body := []byte("fl=123\nh=1.1.1.1\nip=203.0.113.10\nts=1\nvisit_scheme=https\n")
	if ip, ok := ParseTrace(body); !ok || ip.String() != "203.0.113.10" {
		t.Errorf("ParseTrace = %v, %v", ip, ok)
	}
	if ip, ok := ParseTrace([]byte("ip=2001:db8::1\n")); !ok || !ip.Is6() {
		t.Errorf("IPv6: %v %v", ip, ok)
	}
	if _, ok := ParseTrace([]byte("<html>")); ok {
		t.Error("garbage parsed")
	}
}

type fakeAddr string

func (a fakeAddr) Network() string { return "ip+net" }
func (a fakeAddr) String() string  { return string(a) }

func addrs(s ...string) func() ([]net.Addr, error) {
	return func() ([]net.Addr, error) {
		var out []net.Addr
		for _, a := range s {
			out = append(out, fakeAddr(a))
		}
		return out, nil
	}
}

func TestDetect(t *testing.T) {
	ctx := context.Background()
	trace := map[string]string{TraceURLv4: "ip=203.0.113.10\n", TraceURLv6: "ip=2001:db8:abcd::1\n"}
	get := func(fail ...string) func(context.Context, string) ([]byte, error) {
		return func(_ context.Context, u string) ([]byte, error) {
			for _, f := range fail {
				if f == u {
					return nil, errors.New("unreachable")
				}
			}
			return []byte(trace[u]), nil
		}
	}

	// Both families answer: IPv4 first.
	d := &Detector{Get: get(), InterfaceAddrs: addrs("10.0.0.5/24")}
	res := d.Detect(ctx)
	if res.Source != SourceTrace || res.IPv4 != "203.0.113.10" || res.IPv6 != "2001:db8:abcd::1" {
		t.Errorf("both: %+v", res)
	}
	if c := res.Candidates(); len(c) != 2 || c[0] != "203.0.113.10" {
		t.Errorf("candidates = %v", c)
	}
	// The cloud case: the interface only has a private address, so the
	// trace result is not on an interface (possible NAT, D-043).
	if d.OnInterface("203.0.113.10") {
		t.Error("public IP reported on interface")
	}

	// No network: fall back to interfaces, skipping non-public ones.
	d = &Detector{Get: get(TraceURLv4, TraceURLv6), InterfaceAddrs: addrs(
		"127.0.0.1/8", "10.0.0.5/24", "100.64.1.2/10", "fe80::1/64", "fd00::1/64", "198.51.200.7/24", "2a01:4f8::1/64")}
	res = d.Detect(ctx)
	if res.Source != SourceInterface || res.IPv4 != "198.51.200.7" || res.IPv6 != "2a01:4f8::1" {
		t.Errorf("fallback: %+v", res)
	}
	if !d.OnInterface("198.51.200.7") {
		t.Error("interface address not found")
	}

	// Nothing at all: the user must enter an address.
	d = &Detector{Get: get(TraceURLv4, TraceURLv6), InterfaceAddrs: addrs("10.0.0.5/24")}
	if res := d.Detect(ctx); res.Source != SourceNone || len(res.Candidates()) != 0 {
		t.Errorf("none: %+v", res)
	}
}

func TestIsPublic(t *testing.T) {
	for s, want := range map[string]bool{
		"8.8.8.8": true, "2a01:4f8::1": true,
		"10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false, "100.64.0.1": false,
		"127.0.0.1": false, "169.254.1.1": false, "fd00::1": false, "fe80::1": false, "::1": false, "2001:db8::1": false,
	} {
		if got := IsPublic(netip.MustParseAddr(s)); got != want {
			t.Errorf("IsPublic(%s) = %v", s, got)
		}
	}
}
