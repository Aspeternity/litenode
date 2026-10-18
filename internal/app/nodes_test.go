package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/reality"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/xray"
)

type fakeChecker struct{}

func (fakeChecker) Check(_ context.Context, target string) reality.Result {
	host, port, _ := reality.SplitTarget(target)
	return reality.Result{Target: host + ":" + port, Host: host, TLS13: true, H2: true, X25519: true, CertOK: true,
		Latency: 3 * time.Millisecond, HomepageSize: 1000}
}

func (c fakeChecker) CheckAll(ctx context.Context, targets []string, _ int) []reality.Result {
	var out []reality.Result
	for _, t := range targets {
		out = append(out, c.Check(ctx, t))
	}
	return out
}

type fakeDetector struct{}

func (fakeDetector) Detect(context.Context) share.Detected {
	return share.Detected{IPv4: "203.0.113.10", Source: share.SourceTrace}
}
func (fakeDetector) OnInterface(string) bool { return true }

// nodeSystem is a supported Debian 12 with Xray installed and ufw off.
func nodeSystem() *systemtest.Fake {
	f := supported()
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Xray 26.3.27 (Xray, Penetrates Everything.)\n"}}, xray.BinPath, "version")
	return f
}

func runNodes(t *testing.T, f *systemtest.Fake, input string) result {
	t.Helper()
	return runEnv(t, Env{
		Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
		TargetChecker: fakeChecker{}, AddressDetector: fakeDetector{},
	}, input)
}

func TestAddNodeFlow(t *testing.T) {
	f := nodeSystem()
	// main: 1 nodes; node menu: 1 add; preset 1; port default; target:
	// 2 manual, domain; remark default; then back out.
	input := strings.Join([]string{"1", "1", "1", "", "2", "www.example.com", "", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{
		"VLESS + TCP + Reality (Vision)", "★推荐", // preset menu
		"检测到公网地址：203.0.113.10",
		"www.example.com     3 ms  TLS1.3 ✓  H2 ✓  X25519 ✓  证书 ✓",
		"节点已添加。",
		"vless://", "@203.0.113.10:443?", "sni=www.example.com", "#Reality-443",
		"▀", // QR code
		"连不上时，请检查云厂商的安全组或防火墙是否放行了 TCP 端口 443。",
		// DESIGN section 9.1 node list after adding.
		" 分享地址: 203.0.113.10",
		" ID  备注         协议               端口  客户端  状态",
		" 1   Reality-443  VLESS-TCP-Reality  443   1       ● 启用",
		" 1. 添加节点", " 2. 修改节点", " 3. 启用 / 禁用节点", " 4. 客户端管理", " 5. 分享地址        (修改 / 重新检测)", " 6. 删除节点",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	st, err := state.NewStore(f).Load()
	if err != nil || len(st.Nodes) != 1 || st.Server.ShareAddress != "203.0.113.10" {
		t.Fatalf("state = %+v, %v", st, err)
	}
}

func TestNodeMenuEmptyHidesItems(t *testing.T) {
	r := runNodes(t, nodeSystem(), "1\n0\n0\n")
	if !strings.Contains(r.stdout, "（暂无节点）") || !strings.Contains(r.stdout, " 2. 分享地址") || strings.Contains(r.stdout, "删除节点") {
		t.Errorf("empty node menu:\n%s", r.stdout)
	}
}

func TestDeleteNodeFlow(t *testing.T) {
	f := nodeSystem()
	add := strings.Join([]string{"1", "1", "1", "", "2", "www.example.com", "", "0", "0"}, "\n") + "\n"
	runNodes(t, f, add)
	// nodes > delete (6) > node 1 > confirm y
	r := runNodes(t, f, "1\n6\n1\ny\n0\n0\n")
	if !strings.Contains(r.stdout, "删除节点 Reality-443 及其 1 个客户端？ [y/N]") || !strings.Contains(r.stdout, "节点已删除。") {
		t.Errorf("delete flow:\n%s", r.stdout)
	}
	if st, _ := state.NewStore(f).Load(); len(st.Nodes) != 0 {
		t.Error("node not deleted")
	}
}

func TestClientMenuFlow(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "2", "www.example.com", "", "0", "0"}, "\n")+"\n")
	// nodes > clients (4) > node 1 > add (1) default remark; then delete the
	// original client (5, 1, y); then delete the last one (5, 1, y) which
	// deletes the node and leaves the client menu.
	input := strings.Join([]string{"1", "4", "1", "1", "", "5", "1", "y", "5", "1", "y", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	for _, want := range []string{
		" 节点 1: Reality-443 (VLESS-TCP-Reality :443)",
		" 1. 添加客户端", " 2. 修改客户端      (备注 / UUID)", " 3. 启用 / 禁用客户端", " 4. 分享链接 / 二维码", " 5. 删除客户端",
		"客户端已添加。", "#Reality-443-2",
		"删除客户端 Reality-443？ [y/N]", "客户端已删除。",
		"节点至少需要一个客户端，是否同时删除节点？ [y/N]", "节点已删除。",
		"（暂无节点）",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestXrayMenu(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "2", "www.example.com", "", "0", "0"}, "\n")+"\n")
	r := runNodes(t, f, "3\n2\n0\n0\n")
	for _, want := range []string{
		" 1. 启动", " 2. 停止", " 3. 重启", " 4. 查看日志", " 5. 更新 Xray       (可选指定版本)", " 6. 卸载 Xray       (同时删除所有节点和客户端)",
		"Xray 已停止。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if st, _ := state.NewStore(f).Load(); !st.Xray.Stopped {
		t.Error("stop not recorded")
	}
	// Main menu warns while nodes exist but Xray was stopped by hand.
	r = runNodes(t, f, "0\n")
	if !strings.Contains(r.stdout, "⚠ Xray 已手动停止") {
		t.Errorf("no manual-stop warning:\n%s", r.stdout)
	}
	// Not installed: only the note.
	g := supported()
	r = runNodes(t, g, "3\n0\n0\n")
	if !strings.Contains(r.stdout, "Xray 未安装，首次添加节点时会自动安装。") || strings.Contains(r.stdout, " 1. 启动") {
		t.Errorf("not-installed menu:\n%s", r.stdout)
	}
}
