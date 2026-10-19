//go:build vps

// Loads and removes the probe table on the test VPS (M10-01) and checks
// that the rest of the ruleset is untouched. While it waits, send traffic
// to the port from another machine to see the counts.
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o nodecheck-vps.test ./internal/nodecheck
//	LN_PROBE_PORT=45678 LN_PROBE_WAIT=20 ./nodecheck-vps.test -test.v -test.run TestVPS   (as root)
package nodecheck

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/system"
)

var counterRe = regexp.MustCompile(`packets \d+ bytes \d+`)

func TestVPS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	port, _ := strconv.Atoi(os.Getenv("LN_PROBE_PORT"))
	if port == 0 {
		port = 45678
	}
	wait, _ := strconv.Atoi(os.Getenv("LN_PROBE_WAIT"))
	ruleset := func() string {
		t.Helper()
		res, err := r.Query(ctx, "nft", "list", "ruleset")
		if err != nil {
			t.Fatal(err)
		}
		return counterRe.ReplaceAllString(res.Stdout, "")
	}
	before := ruleset()
	ps := []nft.ProbePort{{Proto: "tcp", Port: port}, {Proto: "udp", Port: port}}
	if err := Start(ctx, r, ps); err != nil {
		t.Fatal(err)
	}
	defer Stop(ctx, r)
	// Loading again replaces the table.
	if err := Start(ctx, r, ps); err != nil {
		t.Fatal(err)
	}
	during := ruleset()
	if !strings.HasPrefix(during, before) || !strings.Contains(during, "table inet litenode-probe") {
		t.Errorf("ruleset while probing does not keep the old rules:\n%s", during)
	}
	tr := NewTracker([]string{"tcp", "udp"}, time.Now())
	for i := 0; i <= wait; i++ {
		c, err := Read(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		s := tr.Update(c, time.Now())
		if i == wait || s.Done {
			for _, p := range Final(s) {
				t.Logf("%s arrived=%d passed=%d status=%d sources=%v", p.Name, p.Count.Arrived, p.Count.Passed, p.Status, p.Count.Sources)
			}
			break
		}
		time.Sleep(time.Second)
	}
	if err := Stop(ctx, r); err != nil {
		t.Fatal(err)
	}
	if after := ruleset(); after != before {
		t.Errorf("ruleset changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	// A leftover table is removed; without one nothing happens.
	if err := Start(ctx, r, ps); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := RemoveLeftover(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if after := ruleset(); after != before {
		t.Error("RemoveLeftover left the table")
	}
}
