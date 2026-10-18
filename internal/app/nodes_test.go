package app

import (
	"context"
	"net/netip"
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
	input := strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{
		"═ 节点管理 ═", // page heading (D-060)
		"┌─ 添加节点\n│\n◆ 1/4 节点类型\n│   1. VLESS + TCP + Reality (Vision)\n│   0. 取消\n", // flow (D-069)
		"◆ 2/4 端口 [443]: ", "◆ 3/4 目标网站 (SNI)\n", "◆ 4/4 备注 [Reality-443]: ", "└─ ✓ 节点已添加。\n",
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
		" 1. 添加节点", " 2. 修改节点", " 3. 启用 / 禁用节点", " 4. 客户端管理", " 5. 分享地址        (添加 / 设为默认 / 重新检测)", " 6. 删除节点",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(r.stdout, "推荐\n") || strings.Contains(r.stdout, "★") {
		t.Error("preset menu still recommends a node type")
	}
	st, err := state.NewStore(f).Load()
	if err != nil || len(st.Nodes) != 1 || st.Server.DefaultShareAddress() != "203.0.113.10" {
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
	add := strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n") + "\n"
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
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	// nodes > clients (4) > node 1 > add (1) default remark; then delete the
	// original client (5, 1, y); then the last one (5, 1, y), which keeps
	// the node (D-060); then add a client again (1, default remark).
	input := strings.Join([]string{"1", "4", "1", "1", "", "5", "1", "y", "5", "1", "y", "1", "", "0", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	for _, want := range []string{
		"═ 客户端管理 ═",
		" 节点 1: Reality-443 (VLESS-TCP-Reality :443)",
		" 1. 添加客户端", " 2. 修改客户端      (备注 / UUID)", " 3. 启用 / 禁用客户端", " 4. 分享链接 / 二维码", " 5. 删除客户端",
		"请输入客户端序号 (0 返回): ",
		"客户端已添加。", "#Reality-443-2",
		"删除客户端 Reality-443？ [y/N]", "客户端已删除。",
		"Reality-443-2 是该节点最后一个客户端，删除后节点暂时不可用，直到添加新客户端。是否继续？ [y/N]",
		// The empty node's client menu offers only "add".
		" （该节点没有客户端，当前不可用；添加客户端后即可使用）\n──────────────────────────────────────────────\n 1. 添加客户端\n──────────────────────────────────────────────\n 0. 返回",
		"flow=xtls-rprx-vision",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(r.stdout, "节点已删除。") {
		t.Error("deleting the last client deleted the node")
	}
	st, _ := state.NewStore(f).Load()
	if len(st.Nodes) != 1 || len(st.Nodes[0].Clients) != 1 || st.Nodes[0].Clients[0].Flow != "xtls-rprx-vision" {
		t.Errorf("state = %+v", st.Nodes)
	}
}

func TestChooseTargetReturnsToMenu(t *testing.T) {
	f := nodeSystem()
	// Add: preset 1, port default; target menu: common list (2) then back
	// (0); manual (3) then empty line (back); common list again, pick 1;
	// default remark.
	input := strings.Join([]string{"1", "1", "1", "", "2", "0", "3", "", "2", "1", "", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	if got := strings.Count(r.stdout, "目标网站 (SNI)\n│   1. 扫描附近网站"); got != 3 {
		t.Errorf("target menu shown %d times, want 3:\n%s", got, r.stdout)
	}
	for _, want := range []string{
		"│   ◇ 常用目标网站\n", "│   1.  www.microsoft.com ", "│   14. www.xbox.com ", "\n│   0. 返回\n",
		"节点已添加。", "sni=www.microsoft.com",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// 0 at the target menu cancels adding the node.
	r = runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "0", "0", "0"}, "\n")+"\n")
	if st, _ := state.NewStore(f).Load(); len(st.Nodes) != 1 {
		t.Errorf("cancel added a node: %d nodes", len(st.Nodes))
	}
}

func TestXrayMenu(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
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

type fakeScanner struct{ calls int }

func (f *fakeScanner) Scan(_ context.Context, center netip.Addr, progress func(int, int)) ([]reality.Result, error) {
	f.calls++
	progress(253, 253)
	return []reality.Result{{Target: "near.example.net:443", Host: "near.example.net", TLS13: true, H2: true, X25519: true, CertOK: true, Latency: 2 * time.Millisecond}}, nil
}

func TestScanTargetFlow(t *testing.T) {
	f := nodeSystem()
	sc := &fakeScanner{}
	env := func() Env {
		return Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
			TargetChecker: fakeChecker{}, AddressDetector: fakeDetector{}, TargetScanner: sc}
	}
	// First use: the warning is asked; answering no returns to the target
	// menu, where 0 cancels adding.
	r := runEnv(t, env(), strings.Join([]string{"1", "1", "1", "", "1", "n", "0", "0", "0"}, "\n")+"\n")
	if !strings.Contains(r.stdout, "部分 VPS 商家禁止端口扫描") || sc.calls != 0 ||
		strings.Count(r.stdout, "目标网站 (SNI)\n│   1. 扫描附近网站") != 2 || strings.Contains(r.stdout, "常用目标网站") {
		t.Fatalf("declined warning: calls=%d\n%s", sc.calls, r.stdout)
	}
	if st, _ := state.NewStore(f).Load(); st.Server.ScanWarned {
		t.Error("declined warning remembered")
	}
	// Accept, scan, pick result 1, default remark.
	r = runEnv(t, env(), strings.Join([]string{"1", "1", "1", "", "1", "y", "1", "", "0", "0"}, "\n")+"\n")
	for _, want := range []string{"扫描附近网站    (推荐，约 1 分钟)", "正在扫描 203.0.113.0/24 的 443 端口", "正在扫描 253/253", "◇ 附近的网站", "│   1. near.example.net", "节点已添加。", "sni=near.example.net"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	st, _ := state.NewStore(f).Load()
	if !st.Server.ScanWarned || len(st.Nodes) != 1 {
		t.Errorf("state = %+v", st)
	}
	// Second scan: no warning again.
	r = runEnv(t, env(), strings.Join([]string{"1", "1", "1", "", "1", "1", "Two", "0", "0"}, "\n")+"\n")
	if strings.Contains(r.stdout, "部分 VPS 商家禁止端口扫描") || sc.calls != 2 {
		t.Errorf("warning repeated or scan not run: calls=%d", sc.calls)
	}
}
