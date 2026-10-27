package warp

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/egress"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestNewKeys(t *testing.T) {
	k, err := NewKeys(bytes.NewReader(bytes.Repeat([]byte{7}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := base64.StdEncoding.DecodeString(k.Private)
	pk, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil || base64.StdEncoding.EncodeToString(pk.PublicKey().Bytes()) != k.Public {
		t.Errorf("keys do not match: %+v %v", k, err)
	}
}

func TestRegisterBody(t *testing.T) {
	var m map[string]string
	if err := json.Unmarshal(RegisterBody("PUB=", time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC)), &m); err != nil {
		t.Fatal(err)
	}
	if m["key"] != "PUB=" || m["tos"] != "2026-10-27T01:02:03Z" || m["locale"] != "en_US" {
		t.Errorf("body = %v", m)
	}
}

// regResponse is shaped like a real registration response (trimmed).
const regResponse = `{"id":"dev-1","type":"a","key":"PUB=","token":"tok-1","account":{"id":"acc","account_type":"free"},
"config":{"client_id":"AbCd","peers":[{"public_key":"bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
"endpoint":{"v4":"162.159.192.9:0","v6":"[2606:4700:d0::a29f:c009]:0","host":"engage.cloudflareclient.com:2408","ports":[2408,500,1701,4500,8854]}}],
"interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700:110:8a1e::1"}},"services":{"http_proxy":"172.16.0.1:2480"}}}`

func TestParseAccount(t *testing.T) {
	a, err := ParseAccount([]byte(regResponse))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "dev-1" || a.Token != "tok-1" || a.Addr4.String() != "172.16.0.2" || a.Addr6.String() != "2606:4700:110:8a1e::1" ||
		a.PeerKey != "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=" || len(a.Endpoints) != 2 || a.Endpoints[0].String() != "2606:4700:d0::a29f:c009" || len(a.Ports) != 5 {
		t.Errorf("account = %+v", a)
	}
	for _, bad := range []string{`{"id":"x"}`, `<html>`, `{"id":"x","token":"y","config":{"peers":[{"public_key":"k"}]}}`} {
		if _, err := ParseAccount([]byte(bad)); err == nil {
			t.Errorf("ParseAccount(%s) should fail", bad)
		}
	}
}

func TestCandidates(t *testing.T) {
	a, _ := ParseAccount([]byte(regResponse))
	got := Candidates(a.Endpoints, a.Ports, true, netip.AddrPort{})
	// 3 IPv6 addresses x 5 ports, the offered address first.
	if len(got) != 15 || got[0].String() != "[2606:4700:d0::a29f:c009]:2408" || got[1].String() != "[2606:4700:d0::a29f:c009]:1701" ||
		got[5].String() != "[2606:4700:d0::a29f:c001]:2408" {
		t.Errorf("candidates = %v", got)
	}
	cur := netip.MustParseAddrPort("[2606:4700:d0::a29f:c001]:1701")
	got = Candidates(nil, nil, true, cur)
	if len(got) != 8 || got[len(got)-1] != cur || got[0].String() != "[2606:4700:d0::a29f:c001]:2408" {
		t.Errorf("reselect = %v", got)
	}
	for _, ap := range Candidates(a.Endpoints, nil, false, netip.AddrPort{}) {
		if !ap.Addr().Is4() {
			t.Errorf("IPv4 endpoints contain %s", ap)
		}
	}
}

func TestConfRoundTrip(t *testing.T) {
	c := Conf{AccountID: "dev-1", Token: "tok-1", PrivateKey: "PRIV=", Address: netip.MustParseAddr("172.16.0.2"),
		PeerKey: "PEER=", Endpoint: netip.MustParseAddrPort("[2606:4700:d0::a29f:c001]:1701")}
	out := string(c.Render())
	for _, want := range []string{"Address = 172.16.0.2/32\n", "AllowedIPs = 0.0.0.0/0\n", "Endpoint = [2606:4700:d0::a29f:c001]:1701\n", "MTU = 1280\n", "# litenode-warp-family: ipv4\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DNS") || strings.Contains(out, "::/0") {
		t.Errorf("unexpected lines:\n%s", out)
	}
	back, err := ParseConf([]byte(out))
	if err != nil || back != c {
		t.Errorf("ParseConf = %+v, %v", back, err)
	}
	c6 := c
	c6.IPv6, c6.Address, c6.Endpoint = true, netip.MustParseAddr("2606:4700:110:8a1e::1"), netip.MustParseAddrPort("162.159.192.1:2408")
	out = string(c6.Render())
	if !strings.Contains(out, "Address = 2606:4700:110:8a1e::1/128\n") || !strings.Contains(out, "AllowedIPs = ::/0\n") {
		t.Errorf("ipv6:\n%s", out)
	}
	if back, err := ParseConf([]byte(out)); err != nil || back != c6 {
		t.Errorf("ParseConf ipv6 = %+v, %v", back, err)
	}
	if _, err := ParseConf([]byte("[Interface]\nPrivateKey = x\n")); err == nil {
		t.Error("foreign file accepted")
	}
}

func TestParseHandshake(t *testing.T) {
	if got := ParseHandshake("bmXOC+F1Fx=\t1793096641\n"); got.Unix() != 1793096641 {
		t.Errorf("got %v", got)
	}
	if !ParseHandshake("bmXOC+F1Fx=\t0\n").IsZero() || !ParseHandshake("").IsZero() {
		t.Error("no handshake should be zero")
	}
}

// api fakes the registration API.
type api struct {
	reqs []string
	fail bool
}

func (a *api) RoundTrip(r *http.Request) (*http.Response, error) {
	a.reqs = append(a.reqs, r.Method+" "+r.URL.String()+" "+r.Header.Get("Authorization"))
	body, code := regResponse, 200
	if a.fail {
		body, code = `{"success":false}`, 429
	}
	if r.Method == http.MethodDelete {
		body = ""
	}
	return &http.Response{StatusCode: code, Status: http.StatusText(code), Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

// fixture: handshakes always happen, but only endpoints on port 1701
// carry traffic.
func fixture(t *testing.T) (*systemtest.Fake, *Service, *api) {
	t.Helper()
	f := systemtest.New()
	store := state.NewStore(f)
	if err := store.Save(state.New()); err != nil {
		t.Fatal(err)
	}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "PEER=\t1793096641\n"}}, "wg", "show", Interface, "latest-handshakes")
	a := &api{}
	s := &Service{Runner: f, Changes: change.NewManager(f, store), Client: &http.Client{Transport: a},
		Rand: bytes.NewReader(bytes.Repeat([]byte{7}, 64)), Sleep: func(time.Duration) {},
		Trace: func(_ context.Context, v6 bool) (egress.Trace, error) {
			if v6 || !strings.Contains(string(f.Files[ConfPath]), ":1701\n") {
				return egress.Trace{}, errors.New("timeout")
			}
			return egress.Trace{IP: netip.MustParseAddr("104.28.215.68"), WARP: true}, nil
		}}
	return f, s, a
}

func TestEnableTriesEndpoints(t *testing.T) {
	f, s, a := fixture(t)
	var tried []string
	if err := s.Enable(t.Context(), false, func(ep netip.AddrPort) { tried = append(tried, ep.String()) }); err != nil {
		t.Fatal(err)
	}
	if want := []string{"[2606:4700:d0::a29f:c009]:2408", "[2606:4700:d0::a29f:c009]:1701"}; strings.Join(tried, " ") != strings.Join(want, " ") {
		t.Errorf("tried %v, want %v", tried, want)
	}
	c, err := s.Conf()
	if err != nil || c.Endpoint.String() != "[2606:4700:d0::a29f:c009]:1701" || c.AccountID != "dev-1" || c.IPv6 || c.Address.String() != "172.16.0.2" {
		t.Errorf("conf = %+v, %v", c, err)
	}
	execs := strings.Join(f.Execs(), "\n")
	for _, want := range []string{"modprobe wireguard", "systemctl enable " + Unit, "systemctl restart " + Unit} {
		if !strings.Contains(execs, want) {
			t.Errorf("missing %q in\n%s", want, execs)
		}
	}
	if len(a.reqs) != 1 || !strings.HasPrefix(a.reqs[0], "POST "+APIURL) {
		t.Errorf("api requests = %v", a.reqs)
	}
	st, err := s.Status(t.Context())
	if err != nil || !st.Enabled || st.Handshake.Unix() != 1793096641 {
		t.Errorf("status = %+v, %v", st, err)
	}

	// Disable removes the file, stops the service, deletes the device.
	if err := s.Disable(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[ConfPath]; ok {
		t.Error("conf left behind")
	}
	if !strings.Contains(strings.Join(f.Execs(), "\n"), "systemctl disable --now "+Unit) {
		t.Error("service not disabled")
	}
	if last := a.reqs[len(a.reqs)-1]; last != "DELETE "+APIURL+"/dev-1 Bearer tok-1" {
		t.Errorf("delete request = %q", last)
	}
}

func TestEnableNothingWorks(t *testing.T) {
	f, s, a := fixture(t)
	s.Trace = func(context.Context, bool) (egress.Trace, error) { return egress.Trace{}, errors.New("timeout") }
	n := 0
	err := s.Enable(t.Context(), false, func(netip.AddrPort) { n++ })
	if !errors.Is(err, ErrNoEndpoint) || n != 15 {
		t.Errorf("err = %v after %d attempts", err, n)
	}
	if _, ok := f.Files[ConfPath]; ok {
		t.Error("conf left behind")
	}
	// Each failed attempt takes the interface down with the stand-in
	// file: the real one is already gone (wg-quick would refuse).
	if n := strings.Count(strings.Join(f.Execs(), "\n"), "wg-quick down "+downConf); n != 15 {
		t.Errorf("wg-quick down ran %d times", n)
	}
	if _, ok := f.Files[downConf]; ok {
		t.Error("stand-in file left behind")
	}
	if last := a.reqs[len(a.reqs)-1]; !strings.HasPrefix(last, "DELETE ") {
		t.Errorf("device not deleted: %v", a.reqs)
	}
}

func TestEnableFailures(t *testing.T) {
	f, s, a := fixture(t)
	a.fail = true
	if err := s.Enable(t.Context(), false, nil); err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("rate limited: %v", err)
	}
	f.Respond(systemtest.Response{Err: errors.New("exit status 1")}, "modprobe", "wireguard")
	if err := s.Enable(t.Context(), false, nil); !errors.Is(err, ErrNoKernel) {
		t.Errorf("no kernel: %v", err)
	}
	// wg missing: wireguard-tools is installed first.
	f, s, _ = fixture(t)
	f.Respond(systemtest.Response{Err: errors.New("not found")}, "wg", "--version")
	s.Enable(t.Context(), false, nil)
	if !strings.Contains(strings.Join(f.Execs(), "\n"), "apt-get install -y --no-install-recommends wireguard-tools") {
		t.Errorf("wireguard-tools not installed: %v", f.Execs())
	}
}

func TestReselectKeepsWorkingEndpoint(t *testing.T) {
	f, s, _ := fixture(t)
	if err := s.Enable(t.Context(), false, nil); err != nil {
		t.Fatal(err)
	}
	var tried []string
	if err := s.Reselect(t.Context(), func(ep netip.AddrPort) { tried = append(tried, ep.String()) }); err != nil {
		t.Fatal(err)
	}
	// The built-in c001 on 1701 works before the current endpoint.
	if c, _ := s.Conf(); c.Endpoint.String() != "[2606:4700:d0::a29f:c001]:1701" || tried[0] != "[2606:4700:d0::a29f:c001]:2408" {
		t.Errorf("endpoint = %s, tried %v", c.Endpoint, tried)
	}
	_ = f
}

// clock is a fake time that Sleep advances.
type clock struct{ t time.Time }

func (c *clock) now() time.Time        { return c.t }
func (c *clock) sleep(d time.Duration) { c.t = c.t.Add(d) }
func (c *clock) unix(ago time.Duration) string {
	return "PEER=\t" + strconv.FormatInt(c.t.Add(-ago).Unix(), 10) + "\n"
}

func TestWatch(t *testing.T) {
	f, s, _ := fixture(t)
	if err := s.Enable(t.Context(), false, nil); err != nil {
		t.Fatal(err)
	}
	execs := strings.Join(f.Execs(), "\n")
	if !strings.Contains(execs, "systemctl enable --now "+CheckTimer) || !strings.Contains(string(f.Files[unitDir+"/"+CheckService]), "ExecStart=/usr/local/bin/litenode internal warp-check") {
		t.Errorf("check timer not installed:\n%s", execs)
	}
	c := &clock{t: time.Unix(1793100000, 0)}
	s.Now, s.Sleep = c.now, c.sleep
	hs := func(ago time.Duration) {
		f.Respond(systemtest.Response{Result: system.Result{Stdout: c.unix(ago)}}, "wg", "show", Interface, "latest-handshakes")
	}
	// Fresh handshake: nothing to do.
	hs(time.Minute)
	if ep, err := s.Watch(t.Context()); err != nil || ep.IsValid() {
		t.Errorf("fresh: %v %v", ep, err)
	}
	// Stale: the first other endpoint that handshakes is saved.
	hs(10 * time.Minute)
	setC001 := system.CommandLine("wg", "set", Interface, "peer", "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=", "endpoint", "[2606:4700:d0::a29f:c001]:2408")
	f.OnCommand(setC001, func() { hs(0) })
	ep, err := s.Watch(t.Context())
	if err != nil || ep.String() != "[2606:4700:d0::a29f:c001]:2408" {
		t.Fatalf("stale: %v %v", ep, err)
	}
	if cf, _ := s.Conf(); cf.Endpoint != ep {
		t.Errorf("saved endpoint = %s", cf.Endpoint)
	}
	// Nothing answers: back to the saved endpoint.
	hs(10 * time.Minute)
	f.OnCommand(setC001, func() {})
	if _, err := s.Watch(t.Context()); !errors.Is(err, ErrAllStale) {
		t.Errorf("all stale: %v", err)
	}
	if last := f.Execs()[len(f.Execs())-1]; !strings.HasSuffix(last, "endpoint '[2606:4700:d0::a29f:c001]:2408'") {
		t.Errorf("last = %q", last)
	}
	// Disabling removes the timer.
	if err := s.Disable(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[unitDir+"/"+CheckTimer]; ok {
		t.Error("timer left behind")
	}
}
