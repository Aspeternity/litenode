package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/sysctl"
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
