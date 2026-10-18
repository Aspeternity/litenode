package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

const ccPath = "/proc/sys/net/ipv4/tcp_congestion_control"

func runSysopt(t *testing.T, f *systemtest.Fake, input ...string) result {
	t.Helper()
	return runEnv(t, Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock")}, strings.Join(input, "\n")+"\n")
}

func TestSysoptMenuBBR(t *testing.T) {
	f := supported()
	f.Files["/proc/sys/net/ipv4/tcp_available_congestion_control"] = []byte("reno cubic bbr\n")
	f.Files[ccPath] = []byte("cubic\n")
	f.OnCommand("sysctl -p "+sysctl.ConfigPath, func() { f.Files[ccPath] = []byte("bbr\n") })
	// main: 6 system optimization; 1 BBR, yes; 1 BBR again, yes (off); back; quit.
	r := runSysopt(t, f, "6", "1", "y", "1", "y", "0", "0")
	wantOutput(t, r.stdout, "BBR: ✗", text.BBREnabledDone, "BBR: ✓", text.BBRDisabledDone)
	if _, ok := f.Files[sysctl.ConfigPath]; ok {
		t.Error("sysctl file kept after turning BBR off")
	}
}

func TestSysoptMenuHidesUnavailable(t *testing.T) {
	f := supported()
	f.Respond(systemtest.Response{Err: errors.New("not found")}, "modinfo", "-n", "tcp_bbr")
	r := runSysopt(t, f, "6", "0", "0")
	if strings.Contains(r.stdout, text.SysoptBBR) {
		t.Errorf("BBR shown without kernel support\n%s", r.stdout)
	}
	wantOutput(t, r.stdout, "部分功能在当前环境")
}

func TestSysoptMenuSwap(t *testing.T) {
	f := supported()
	const header = "Filename Type Size Used Priority\n"
	f.Files["/proc/meminfo"] = []byte("MemTotal: 996720 kB\nMemAvailable: 600000 kB\n")
	f.Files["/proc/swaps"] = []byte(header)
	f.Files["/etc/fstab"] = []byte("UUID=x / ext4 defaults 0 1\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Filesystem 1M-blocks Used Available Capacity Mounted on\n/dev/vda1 6900 2500 4300 37% /\n"}}, "df", "-P", "-B1M", "/")
	f.OnCommand("swapon /litenode.swap", func() { f.Files["/proc/swaps"] = []byte(header + "/litenode.swap file 1048572 0 -2\n") })
	f.OnCommand("swapoff /litenode.swap", func() { f.Files["/proc/swaps"] = []byte(header) })
	// main: 6; 2 swap, default size; 2 swap, 2 delete, yes; back; quit.
	r := runSysopt(t, f, "6", "2", "", "2", "2", "y", "0", "0")
	wantOutput(t, r.stdout, "swap: 无", "内存 973M，建议创建 1G swap。", "最多可创建 4044M", text.SwapCreated("1G"), "swap: 1G", text.SwapDeleted)
	if strings.Contains(string(f.Files["/etc/fstab"]), "litenode.swap") {
		t.Errorf("fstab:\n%s", f.Files["/etc/fstab"])
	}
}

func TestSysoptMenuPreference(t *testing.T) {
	f := supported()
	f.Files["/etc/gai.conf"] = []byte("# getaddrinfo\n")
	f.Files["/proc/swaps"] = []byte("Filename Type Size Used Priority\n")
	f.Respond(systemtest.Response{Err: errors.New("not found")}, "modinfo", "-n", "tcp_bbr")
	// main: 6; 2 (BBR hidden), IPv4; 2, IPv6; back; quit.
	r := runSysopt(t, f, "6", "2", "1", "2", "2", "0", "0")
	wantOutput(t, r.stdout, "优先: IPv6", text.PreferIPv4Done, "优先: IPv4", text.PreferIPv6Done)
	if string(f.Files["/etc/gai.conf"]) != "# getaddrinfo\n" {
		t.Errorf("gai.conf:\n%s", f.Files["/etc/gai.conf"])
	}
}

func TestSysoptMenuDNS(t *testing.T) {
	f := supported()
	f.Files["/etc/resolv.conf"] = []byte("nameserver 10.0.0.1\nsearch example.com\n")
	f.Respond(systemtest.Response{Err: errors.New("not found")}, "modinfo", "-n", "tcp_bbr")
	probe := func(_ context.Context, server string) error {
		if server == "9.9.9.9" {
			return errors.New("timeout")
		}
		return nil
	}
	in := "6\n2\n4\n1.1.1.1 9.9.9.9\nn\n2\n1\n0\n0\n"
	// main: 6; 2 DNS (BBR, swap hidden), custom with a failing server, no;
	// 2 DNS, Cloudflare; back; quit.
	r := runEnv(t, Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"), DNSProber: probe}, in)
	wantOutput(t, r.stdout, "当前 DNS：10.0.0.1（系统配置，/etc/resolv.conf）", "9.9.9.9 无法解析域名：timeout",
		text.DNSResolvConfNote, text.DNSSet("1.1.1.1 1.0.0.1"))
	if got := string(f.Files["/etc/resolv.conf"]); !strings.Contains(got, "nameserver 1.1.1.1\nnameserver 1.0.0.1\nsearch example.com\n") {
		t.Errorf("resolv.conf:\n%s", got)
	}
}

func TestSysoptMenuClock(t *testing.T) {
	f := supported()
	f.Respond(systemtest.Response{Err: errors.New("not found")}, "modinfo", "-n", "tcp_bbr")
	show := []string{"show", "-p", "Timezone", "-p", "NTP", "-p", "NTPSynchronized", "-p", "CanNTP"}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Timezone=UTC\nNTP=no\nNTPSynchronized=no\nCanNTP=yes\n"}}, "timedatectl", show...)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Asia/Shanghai\nUTC\n"}}, "timedatectl", "list-timezones")
	f.OnCommand("timedatectl set-ntp true", func() {
		f.Respond(systemtest.Response{Result: system.Result{Stdout: "Timezone=UTC\nNTP=yes\nNTPSynchronized=yes\nCanNTP=yes\n"}}, "timedatectl", show...)
	})
	// main: 6; 3 clock (BBR, swap hidden), 1 zone, 2 Shanghai; 3, 2 NTP; back; quit.
	r := runSysopt(t, f, "6", "3", "1", "2", "3", "2", "0", "0")
	wantOutput(t, r.stdout, "时区: UTC (同步 ✗)", "UTC (当前)", text.TimezoneSet("Asia/Shanghai"), text.NTPSyncedDone, "时区: UTC (同步 ✓)")
	if !slices.Contains(f.Execs(), "timedatectl set-timezone Asia/Shanghai") {
		t.Errorf("execs = %q", f.Execs())
	}
}

// sysoptFake is a system where every optimization is available: BBR on,
// no swap, IPv4 preferred, UTC synchronized.
func sysoptFake() *systemtest.Fake {
	f := supported()
	f.Files["/proc/sys/net/ipv4/tcp_available_congestion_control"] = []byte("reno cubic bbr\n")
	f.Files[ccPath] = []byte("bbr\n")
	f.Files["/proc/swaps"] = []byte("Filename Type Size Used Priority\n")
	f.Files["/etc/gai.conf"] = []byte("precedence ::ffff:0:0/96  100\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Timezone=UTC\nNTP=yes\nNTPSynchronized=yes\nCanNTP=yes\n"}},
		"timedatectl", "show", "-p", "Timezone", "-p", "NTP", "-p", "NTPSynchronized", "-p", "CanNTP")
	return f
}

func sysoptMenu(t *testing.T, f *systemtest.Fake, virt system.Virt) string {
	t.Helper()
	store := state.NewStore(f)
	if err := store.Save(state.New()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u := &sysoptUI{ctx: ctx, s: ui.NewSession(strings.NewReader(""), io.Discard, nil, ui.Colors{}),
		info: system.Info{Virt: virt}, caps: system.ProbeAll(ctx, f),
		svc: &sysopt.Service{Runner: f, Store: store, Changes: change.NewManager(f, store)}}
	return u.menu().Render()
}

// TestRenderSysoptMenu compares against the DESIGN section 15 example.
func TestRenderSysoptMenu(t *testing.T) {
	want, err := os.ReadFile("testdata/sysopt_menu.golden")
	if err != nil {
		t.Fatal(err)
	}
	f := sysoptFake()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")
	if got := sysoptMenu(t, f, system.Virt{Type: "kvm"}); got != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSysoptMenuContainer(t *testing.T) {
	f := sysoptFake()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "lxc\n"}}, "systemd-detect-virt")
	f.Respond(systemtest.Response{Err: errors.New("not found")}, "modinfo", "-n", "tcp_bbr")
	f.Files["/proc/sys/net/ipv4/tcp_available_congestion_control"] = []byte("reno cubic\n")
	got := sysoptMenu(t, f, system.Virt{Type: "lxc", Container: true})
	for _, want := range []string{"部分功能在当前环境（LXC）中不可用，已隐藏。", " 优先: IPv4   时区: UTC\n", " 1. IPv4 / IPv6 优先级", " 3. 时区与时间同步"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	for _, hidden := range []string{"BBR", "swap", "同步 "} {
		if strings.Contains(got, hidden) {
			t.Errorf("%q shown in a container:\n%s", hidden, got)
		}
	}
}
