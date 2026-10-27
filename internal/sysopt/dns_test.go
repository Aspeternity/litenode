package sysopt

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestParseServers(t *testing.T) {
	got, err := ParseServers(" 1.1.1.1, 2606:4700:4700::1111，1.1.1.1 ")
	if err != nil || !slices.Equal(got, []string{"1.1.1.1", "2606:4700:4700::1111"}) {
		t.Errorf("ParseServers = %q, %v", got, err)
	}
	for _, in := range []string{"", "dns.google", "1.1.1.1 2.2.2.2 3.3.3.3 4.4.4.4", "fe80::1%eth0"} {
		if _, err := ParseServers(in); !errors.Is(err, ErrDNSServers) {
			t.Errorf("ParseServers(%q): %v", in, err)
		}
	}
}

func TestResolvConf(t *testing.T) {
	old := "# from DHCP\nnameserver 10.0.0.1\nsearch example.com\noptions edns0 trust-ad\n"
	want := dnsHeader + "\nnameserver 1.1.1.1\nnameserver 1.0.0.1\nsearch example.com\noptions edns0 trust-ad\n"
	if got := string(ResolvConf([]byte(old), []string{"1.1.1.1", "1.0.0.1"})); got != want {
		t.Errorf("ResolvConf:\n%s", got)
	}
	if got := string(ResolvedConfig([]string{"8.8.8.8"})); got != dnsHeader+"\n[Resolve]\nDNS=8.8.8.8\nDomains=~.\n" {
		t.Errorf("ResolvedConfig:\n%s", got)
	}
}

const stub = "# This is /run/systemd/resolve/stub-resolv.conf managed by man:systemd-resolved(8).\nnameserver 127.0.0.53\noptions edns0 trust-ad\n"

func resolvedActive(f *systemtest.Fake) {
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "active\n"}}, "systemctl", "is-active", ResolvedUnit)
}

func TestSetDNSResolved(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.Files[ResolvConfPath] = []byte(stub)
	resolvedActive(f)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Global:\nLink 2 (eth0): 10.0.0.1 10.0.0.2\n"}}, "resolvectl", "dns")
	if d, _ := s.DNSStatus(ctx); !d.Resolved || d.Managed || !slices.Equal(d.Servers, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Errorf("before: %+v", d)
	}
	if err := s.SetDNS(ctx, []string{"1.1.1.1", "1.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if string(f.Files[ResolvConfPath]) != stub {
		t.Error("resolv.conf changed")
	}
	if d, _ := s.DNSStatus(ctx); !d.Managed || !slices.Equal(d.Servers, []string{"1.1.1.1", "1.0.0.1"}) {
		t.Errorf("after: %+v", d)
	}
	if !slices.Contains(f.Execs(), "systemctl restart "+ResolvedUnit) {
		t.Errorf("execs = %q", f.Execs())
	}
	if err := s.ResetDNS(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[ResolvedDropIn]; ok {
		t.Error("drop-in kept")
	}
}

func TestSetDNSResolvConf(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	// systemd-resolved runs but resolv.conf does not use it.
	resolvedActive(f)
	f.Files[ResolvConfPath] = []byte("nameserver 10.0.0.1\n")
	if err := s.SetDNS(ctx, []string{"8.8.8.8"}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.DNSStatus(ctx)
	if d.Resolved || !d.Managed || !slices.Equal(d.Servers, []string{"8.8.8.8"}) {
		t.Errorf("status = %+v", d)
	}
	if _, ok := f.Files[ResolvedDropIn]; ok {
		t.Error("drop-in written")
	}
}

func TestPresetsFor(t *testing.T) {
	cases := []struct {
		v4, v6 bool
		want   []string
	}{
		{true, false, []string{"1.1.1.1", "1.0.0.1"}},
		{false, false, []string{"1.1.1.1", "1.0.0.1"}},
		{false, true, []string{"2606:4700:4700::1111", "2606:4700:4700::1001"}},
		{true, true, []string{"1.1.1.1", "1.0.0.1", "2606:4700:4700::1111"}},
	}
	for _, c := range cases {
		ps := PresetsFor(c.v4, c.v6)
		if len(ps) != 3 || ps[0].Name != "Cloudflare" || !slices.Equal(ps[0].Servers, c.want) {
			t.Errorf("PresetsFor(%v, %v) = %v", c.v4, c.v6, ps)
		}
		for _, p := range ps {
			if _, err := ParseServers(strings.Join(p.Servers, " ")); err != nil {
				t.Errorf("%s: %v", p.Name, err)
			}
		}
	}
}

func TestSlowFirstServer(t *testing.T) {
	cases := []struct {
		name     string
		d        DNS
		ipv4Exit bool
		want     string
	}{
		{"ipv4 first", DNS{Servers: []string{"1.1.1.1", "2001:db8::53"}}, false, "1.1.1.1"},
		{"ipv6 first", DNS{Servers: []string{"2001:db8::53", "1.1.1.1"}}, false, ""},
		{"ipv4 exit", DNS{Servers: []string{"1.1.1.1"}}, true, ""},
		// systemd-resolved moves on to a server that answers.
		{"resolved", DNS{Resolved: true, Servers: []string{"1.1.1.1"}}, false, ""},
		{"none", DNS{}, false, ""},
	}
	for _, tc := range cases {
		got, ok := SlowFirstServer(tc.d, tc.ipv4Exit)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%s: got %q %v, want %q", tc.name, got, ok, tc.want)
		}
	}
}
