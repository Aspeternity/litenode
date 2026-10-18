//go:build vps

// CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o forward-vps.test ./internal/forwardmgr
//
// Steps of the M4-01 check, run as root on the test VPS with LN_FWD set:
//
//	LN_FWD=golden LN_GOLDEN=<dir with *.nft>   nft -c every golden file
//	LN_FWD=link LN_ADDR=<ip> LN_PORT=<port>     print the first node's link via addr:port
//	LN_FWD=add LN_SPEC='tcp+udp 30001 1.2.3.4 443;...'
//	LN_FWD=delete LN_PORTS='30001 30002'
//	LN_FWD=fw-on | fw-off                       LiteNode firewall, confirmed at once
package forwardmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

func TestVPSForward(t *testing.T) {
	mode := os.Getenv("LN_FWD")
	if mode == "" || os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS with LN_FWD set")
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	store := state.NewStore(r)
	m := change.NewManager(r, store)
	s := &Service{Runner: r, Store: store, Changes: m, Executable: "/usr/local/bin/litenode"}
	switch mode {
	case "golden":
		paths, _ := filepath.Glob(filepath.Join(os.Getenv("LN_GOLDEN"), "*.nft"))
		for _, p := range paths {
			if out, err := exec.Command(nft.NftPath, "-c", "-f", p).CombinedOutput(); err != nil {
				t.Errorf("%s: %v\n%s", p, err, out)
			}
		}
		fmt.Printf("checked %d files\n", len(paths))
	case "link":
		reg, err := builtin.Registry()
		if err != nil {
			t.Fatal(err)
		}
		st, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		n := st.Nodes[0]
		n.SharePort, _ = strconv.Atoi(os.Getenv("LN_PORT"))
		link, err := share.Link(reg, n, n.Clients[0], os.Getenv("LN_ADDR"))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(link)
	case "add":
		for _, spec := range strings.Split(os.Getenv("LN_SPEC"), ";") {
			f := strings.Fields(spec)
			got, err := s.Add(ctx, Input{Protocol: f[0], ListenPort: f[1], TargetHost: f[2], TargetPort: f[3]})
			if err != nil {
				t.Fatalf("%s: %v", spec, err)
			}
			fmt.Printf("added forward:%d %s\n", got.ID, spec)
		}
	case "delete":
		ports := strings.Fields(os.Getenv("LN_PORTS"))
		st, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range st.Forwards {
			if slices.Contains(ports, f.ListenPort) {
				if err := s.Delete(ctx, f.ID); err != nil {
					t.Fatal(err)
				}
				fmt.Printf("deleted %s\n", forward.Owner(f.ID))
			}
		}
	case "fw-on", "fw-off":
		fw := &firewall.Service{Store: store, Confirmer: change.NewConfirmer(m, "/usr/local/bin/litenode")}
		op := fw.Enable
		if mode == "fw-off" {
			op = fw.Disable
		}
		if err := op(ctx); err != nil {
			t.Fatal(err)
		}
		if err := fw.Confirmer.Disarm(ctx); err != nil {
			t.Fatal(err)
		}
		fmt.Println(mode, "done")
	default:
		t.Fatalf("unknown LN_FWD %q", mode)
	}
}
