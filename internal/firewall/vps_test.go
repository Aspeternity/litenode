//go:build vps

// CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o firewall-vps.test ./internal/firewall
package firewall

import (
	"context"
	"os"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
)

// TestVPS adds an Oracle-style REJECT rule for an unused port and checks
// that it is detected, then removes it.
func TestVPS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	if f := DetectForeignFilter(ctx, r); f.Found() {
		t.Fatalf("clean system reported filtering: %v", f.Where)
	}
	rule := []string{"INPUT", "-p", "tcp", "--dport", "9", "-j", "REJECT", "--reject-with", "icmp-host-prohibited"}
	if _, err := r.Exec(ctx, "iptables", append([]string{"-A"}, rule...)...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Exec(ctx, "iptables", append([]string{"-D"}, rule...)...) })
	f := DetectForeignFilter(ctx, r)
	t.Logf("detected: %v", f.Where)
	if !f.Found() {
		t.Error("REJECT rule not detected")
	}
}
