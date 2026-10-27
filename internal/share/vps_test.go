//go:build vps

// Test VPS checks for M1-06 (needs zbar-tools for zbarimg):
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o share-vps.test ./internal/share
//	./share-vps.test -test.v -test.run TestVPS
package share

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/egress"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/builtin"
)

// TestVPSQRDecodes renders links as QR codes, decodes them with zbarimg
// and compares.
func TestVPSQRDecodes(t *testing.T) {
	if _, err := exec.LookPath("zbarimg"); err != nil {
		t.Skip("zbarimg not installed")
	}
	reg, _ := builtin.Registry()
	n := testNode()
	for _, mod := range []func(*node.Node){
		nil,
		func(n *node.Node) { n.Clients[0].Remark = "洛杉矶 #1 a&b=c" },
		func(n *node.Node) { n.SharePort = 30001 },
	} {
		nn := testNode()
		nn.Clients = append([]node.Client(nil), n.Clients...)
		if mod != nil {
			mod(&nn)
		}
		for _, addr := range []string{"198.51.100.252", "2001:db8:1234:5678:9abc:def0:1234:5678"} {
			link, err := Link(reg, nn, nn.Clients[0], addr)
			if err != nil {
				t.Fatal(err)
			}
			m, err := Matrix(link)
			if err != nil {
				t.Fatal(err)
			}
			const scale = 6
			img := image.NewGray(image.Rect(0, 0, len(m)*scale, len(m)*scale))
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					c := color.Gray{Y: 255}
					if m[y/scale][x/scale] {
						c.Y = 0
					}
					img.SetGray(x, y, c)
				}
			}
			var buf bytes.Buffer
			png.Encode(&buf, img)
			path := filepath.Join(t.TempDir(), "qr.png")
			os.WriteFile(path, buf.Bytes(), 0o600)
			out, err := exec.Command("zbarimg", "--quiet", "--raw", path).Output()
			if err != nil {
				t.Fatalf("zbarimg failed for %s: %v", link, err)
			}
			if got := strings.TrimRight(string(out), "\n"); got != link {
				t.Fatalf("decoded\n%s\nwant\n%s", got, link)
			}
			t.Logf("ok   %d modules: %s", len(m), link)
		}
	}
}

func TestVPSDetect(t *testing.T) {
	d := NewDetector(egress.New().Status)
	res := d.Detect(context.Background())
	t.Logf("detected IPv4=%q IPv6=%q source=%d", res.IPv4, res.IPv6, res.Source)
	if res.Source != SourceTrace || res.IPv4 == "" {
		t.Fatalf("want an IPv4 address from Cloudflare trace: %+v", res)
	}
	t.Logf("IPv4 on interface: %v", d.OnInterface(res.IPv4))
}
