package node

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestJSONMatchesDesign checks the field names against the node example
// in DESIGN section 5.
func TestJSONMatchesDesign(t *testing.T) {
	n := Node{
		ID: 1, Remark: "LA-Reality", Enabled: true, Port: 443,
		Protocol: "vless", Transport: "tcp", Security: "reality",
		Reality: &Reality{
			Target: "www.example.com:443", ServerNames: []string{"www.example.com"},
			PrivateKey: "...", PublicKey: "...", ShortIDs: []string{"a1b2c3d4"}, Fingerprint: "chrome",
			FallbackLimit: FallbackLimit{AfterBytes: 10485760, BytesPerSec: 1048576, BurstBytesPerSec: 5242880},
		},
		Clients: []Client{{ID: 1, Remark: "me", UUID: "...", Flow: FlowVision, Enabled: true}},
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"remark":"LA-Reality","enabled":true,"port":443,"protocol":"vless","transport":"tcp","security":"reality",` +
		`"reality":{"target":"www.example.com:443","server_names":["www.example.com"],"private_key":"...","public_key":"...",` +
		`"short_ids":["a1b2c3d4"],"fingerprint":"chrome","fallback_limit":{"after_bytes":10485760,"bytes_per_sec":1048576,"burst_bytes_per_sec":5242880}},` +
		`"clients":[{"id":1,"remark":"me","uuid":"...","flow":"xtls-rprx-vision","enabled":true}]}`
	if string(data) != want {
		t.Errorf("got\n%s\nwant\n%s", data, want)
	}
	n.Reality = nil
	data, _ = json.Marshal(n)
	if strings.Contains(string(data), "reality\":") {
		t.Errorf("absent settings should be omitted: %s", data)
	}
}
