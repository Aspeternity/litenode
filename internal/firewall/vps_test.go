//go:build vps

// CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o firewall-vps.test ./internal/firewall
package firewall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
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

// TestVPSNftGolden checks the generator's golden scripts with the real
// nft (M3-01).
func TestVPSNftGolden(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	for _, name := range []string{"empty", "firewall", "no-ping"} {
		path := filepath.Join(os.Getenv("LN_GOLDEN"), name+".nft")
		if out, err := exec.Command(nft.NftPath, "-c", "-f", path).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", name, err, out)
		}
	}
}

// TestVPSNftApply loads (LN_NFT=on) or removes (LN_NFT=off) the table for
// the machine's real state with the firewall on, through the change flow
// (M3-01). Reachability is checked from the development machine.
func TestVPSNftApply(t *testing.T) {
	mode := os.Getenv("LN_NFT")
	if mode != "on" && mode != "off" {
		t.Skip("set LN_NFT=on|off")
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	store := state.NewStore(r)
	st, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := nft.Config{}
	if mode == "on" {
		e, err := ssh.ReadEffective(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		st.Firewall.Enabled = true
		c = Config(st, SSHPorts(st, e.Ports))
	}
	file, err := nft.ConfigFile(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := change.NewManager(r, store).Apply(ctx, change.Change{Parts: []change.Part{{Kind: nft.Kind(), Files: []change.File{file}}}}); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(nft.NftPath, "list", "table", "inet", "litenode").CombinedOutput()
	t.Logf("%s", out)
}
