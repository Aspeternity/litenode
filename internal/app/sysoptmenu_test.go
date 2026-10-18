package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
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
