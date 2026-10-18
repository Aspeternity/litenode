package forwardmgr

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
)

const (
	unitPath  = DefaultUnitDir + "/" + ResolveService
	timerPath = DefaultUnitDir + "/" + ResolveTimer
)

func TestResolveChanged(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	activeUFW(f)
	got, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "la.example.com", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	// ufw has the rules of the old addresses.
	f.Files[ufw.UserRules] = []byte("### tuple ### route:allow tcp 443 203.0.113.20 any 0.0.0.0/0 in comment=6c6974656e6f64653a666f72776172643a34\n")
	f.Files[ufw.User6Rules] = []byte("### tuple ### route:allow tcp 443 2001:db8::20 any ::/0 in comment=6c6974656e6f64653a666f72776172643a34\n")
	// The A record changes; the AAAA record stays.
	s.Resolver = fakeResolver{
		"ip4 la.example.com": {net.ParseIP("203.0.113.30")},
		"ip6 la.example.com": {net.ParseIP("2001:db8::20")},
	}
	n := len(f.Execs())
	res, err := s.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 1 || res.Changed[0].ID != got.ID || len(res.Failed) != 0 {
		t.Errorf("Resolve = %+v", res)
	}
	st, _ := s.Store.Load()
	if fw := st.Forwards[0]; fw.ResolvedIPv4 != "203.0.113.30" || fw.ResolvedIPv6 != "2001:db8::20" {
		t.Errorf("state: %+v", fw)
	}
	if conf := string(f.Files[nft.ConfigPath]); !strings.Contains(conf, "dnat ip to 203.0.113.30:443") || strings.Contains(conf, "203.0.113.20") {
		t.Errorf("rules:\n%s", conf)
	}
	execs := f.Execs()[n:]
	for _, want := range []string{
		"ufw route allow proto tcp to 203.0.113.30 port 443 comment litenode:forward:4",
		"ufw route delete allow proto tcp to 203.0.113.20 port 443",
	} {
		if !slices.Contains(execs, want) {
			t.Errorf("missing %q in %v", want, execs)
		}
	}
	if slices.ContainsFunc(execs, func(e string) bool { return strings.Contains(e, "2001:db8::20") }) {
		t.Errorf("unchanged IPv6 rule touched: %v", execs)
	}
}

func TestResolveUnchanged(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	if _, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "la.example.com", TargetPort: "443"}); err != nil {
		t.Fatal(err)
	}
	before := len(f.Mutations())
	res, err := s.Resolve(ctx)
	if err != nil || len(res.Changed) != 0 {
		t.Fatalf("Resolve = %+v, %v", res, err)
	}
	if m := f.Mutations(); len(m) != before {
		t.Errorf("changes without a new address: %+v", m[before:])
	}
}

// A failed lookup keeps the rules as they are.
func TestResolveLookupFails(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	if _, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "la.example.com", TargetPort: "443"}); err != nil {
		t.Fatal(err)
	}
	before := len(f.Mutations())
	s.Resolver = fakeResolver{} // the domain is gone
	res, err := s.Resolve(ctx)
	if err != nil || len(res.Changed) != 0 || len(res.Failed) != 1 {
		t.Fatalf("Resolve = %+v, %v", res, err)
	}
	if m := f.Mutations(); len(m) != before {
		t.Errorf("changes after a failed lookup: %+v", m[before:])
	}
}

// The timer exists exactly while a forward has a domain target.
func TestResolveTimer(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	ip, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "203.0.113.10", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[timerPath]; ok {
		t.Error("timer without a domain target")
	}
	dom, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30002", TargetHost: "la.example.com", TargetPort: "443"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(f.Files[unitPath]), "ExecStart=/usr/local/bin/litenode internal resolve") || f.Files[timerPath] == nil {
		t.Errorf("units: %q %q", f.Files[unitPath], f.Files[timerPath])
	}
	if !slices.Contains(f.Execs(), "systemctl enable --now "+ResolveTimer) {
		t.Errorf("timer not started: %v", f.Execs())
	}
	// A disabled domain rule still keeps the timer.
	if err := s.SetEnabled(ctx, dom.ID, false); err != nil {
		t.Fatal(err)
	}
	if f.Files[timerPath] == nil {
		t.Error("timer removed while a domain target exists")
	}
	if err := s.Delete(ctx, dom.ID); err != nil {
		t.Fatal(err)
	}
	if f.Files[timerPath] != nil || f.Files[unitPath] != nil || !slices.Contains(f.Execs(), "systemctl disable --now "+ResolveTimer) {
		t.Errorf("timer kept after the last domain target: %v", f.Execs())
	}
	if err := s.Delete(ctx, ip.ID); err != nil {
		t.Fatal(err)
	}
}

// A timer failure after the change is reported as such: the rules stay.
func TestTimerError(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}, Err: errors.New("exit status 1")}, "systemctl", "enable", "--now", ResolveTimer)
	_, err := s.Add(ctx, Input{Protocol: forward.TCP, ListenPort: "30001", TargetHost: "la.example.com", TargetPort: "443"})
	var te *TimerError
	if !errors.As(err, &te) {
		t.Fatalf("Add = %v", err)
	}
	if st, _ := s.Store.Load(); len(st.Forwards) != 1 {
		t.Error("rule rolled back after a timer failure")
	}
}
