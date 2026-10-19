package protocol

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
)

const testHyPassword = "0123456789abcdef0123456789abcdef"

func TestHysteriaPassword(t *testing.T) {
	pw, err := NewHysteriaPassword(rand.Reader)
	if err != nil || !ValidHysteriaPassword(pw) || len(pw) != 32 {
		t.Fatalf("%q %v", pw, err)
	}
	for _, bad := range []string{"", "0123", testHyPassword + "00", "0123456789ABCDEF0123456789ABCDEF", "g123456789abcdef0123456789abcdef"} {
		if ValidHysteriaPassword(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func hyNode() node.Node {
	return node.Node{ID: 3, Port: 45678, Protocol: "hysteria", Transport: "hysteria", Security: "tls",
		Clients: []node.Client{
			{ID: 4, Remark: "me", Key: testHyPassword, Enabled: true},
			{ID: 5, Remark: "off", Key: "ffffffffffffffffffffffffffffffff"},
		}}
}

func TestHysteriaInboundSettings(t *testing.T) {
	v, err := Hysteria{}.InboundSettings(hyNode())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	want := `{"version":2,"clients":[{"auth":"` + testHyPassword + `","email":"me"}]}`
	if !bytes.Equal(b, []byte(want)) {
		t.Errorf("settings = %s, want %s", b, want)
	}
	n := hyNode()
	n.Clients[0].Key = "bad"
	if _, err := (Hysteria{}).InboundSettings(n); err == nil {
		t.Error("bad password accepted")
	}
}

func TestHysteriaShareLink(t *testing.T) {
	n := hyNode()
	params := []node.Param{{Key: "sni", Value: "www.bing.com"}, {Key: "pinSHA256", Value: "86c5"}}
	c := n.Clients[0]
	c.Remark = "my hy2"
	got, err := Hysteria{}.ShareLink(n, c, "2001:db8::1", 45678, params)
	want := "hysteria2://" + testHyPassword + "@[2001:db8::1]:45678/?sni=www.bing.com&pinSHA256=86c5#my%20hy2"
	if err != nil || got != want {
		t.Errorf("link = %s, %v\nwant %s", got, err, want)
	}
}
