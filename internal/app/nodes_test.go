package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/node/security"
	"github.com/Aspeternity/litenode/internal/reality"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
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
		"═ 主菜单 › 节点管理 ═", // page heading (D-060)
		"┌─ 添加节点\n│\n◆ 节点类型\n│   1. VLESS + TCP + Reality (Vision)\n│   2. VLESS + TCP + Encryption (Vision)\n│   3. Shadowsocks 2022\n│   4. Hysteria2\n│   0. 取消\n", // flow (D-069)
		"◆ 1/3 端口 [443]: ", "◆ 2/3 目标网站 (SNI)\n", "◆ 3/3 备注 [Reality-443]: ", "└─ ✓ 节点已添加。\n",
		"检测到公网地址：203.0.113.10",
		"www.example.com     3 ms  TLS1.3 ✓  H2 ✓  X25519 ✓  证书 ✓",
		"节点已添加。",
		"vless://", "@203.0.113.10:443?", "sni=www.example.com", "#Reality-443",
		"▀", // QR code
		"连不上时，请检查云厂商的安全组或防火墙是否放行了 TCP 端口 443。",
		// DESIGN section 9.1 node list after adding.
		" ID  备注         协议               端口  客户端  状态",
		" 1   Reality-443  VLESS-TCP-Reality  443   1       ● 启用",
		" 1. 添加节点", " 2. 修改节点", " 3. 启用 / 禁用节点", " 4. 检测节点连通性", " 5. 删除节点",
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
	if !strings.Contains(r.stdout, "（暂无节点）") || strings.Contains(r.stdout, "主机地址") || strings.Contains(r.stdout, "删除节点") {
		t.Errorf("empty node menu:\n%s", r.stdout)
	}
}

func TestDeleteNodeFlow(t *testing.T) {
	f := nodeSystem()
	add := strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n") + "\n"
	runNodes(t, f, add)
	// nodes > delete (5) > node 1 > confirm y
	r := runNodes(t, f, "1\n5\n1\ny\n0\n0\n")
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
	// main: 2 clients > add (1, only one node: no choice) default remark;
	// then delete the original client (4, 1, y); then the last one (4, 1,
	// y), which keeps the node (D-060); then add a client again (1,
	// default remark).
	input := strings.Join([]string{"2", "1", "", "4", "1", "y", "4", "1", "y", "1", "", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	for _, want := range []string{
		"═ 主菜单 › 客户端管理 ═",
		" ID  节点         备注           状态",
		" 2   Reality-443  Reality-443-2  ● 启用",
		" 1. 添加客户端", " 2. 修改客户端\n", " 3. 启用 / 禁用客户端", " 4. 删除客户端",
		"请输入客户端序号（0 返回）: ",
		"客户端已添加。", "#Reality-443-2",
		"删除客户端 Reality-443？ [y/N]", "客户端已删除。",
		"Reality-443-2 是该节点最后一个客户端，删除后节点暂时不可用，直到添加新客户端。是否继续？ [y/N]",
		// Without clients the menu offers only "add" (D-060).
		" （暂无客户端）\n 节点 Reality-443 没有客户端，当前不可用；添加客户端后即可使用。\n──────────────────────────────────────────────\n 1. 添加客户端\n──────────────────────────────────────────────\n 0. 返回",
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

// TestModifyNodeDone: changes that print the new links close the flow
// as successful, not as canceled.
func TestModifyNodeDone(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	// 1 nodes, 2 modify, node 1: 1 port (y, 8443); 5 share port (2053, y).
	r := runNodes(t, f, "1\n2\n1\n1\ny\n8443\n5\n2053\ny\n0\n0\n0\n")
	for _, want := range []string{"└─ ✓ " + text.PortSet(8443), "└─ ✓ " + text.SharePortSet, text.NewLinksTitle} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, text.FlowCanceled) {
		t.Errorf("flow shown as canceled:\n%s", r.stdout)
	}
}

func TestXrayUpdateBack(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	// 5 Xray, 6 update: 0 goes back.
	r := runNodes(t, f, "5\n6\n0\n0\n0\n")
	if !strings.Contains(r.stdout, "0 返回）") || !strings.Contains(r.stdout, "└─ "+text.FlowCanceled) || strings.Contains(r.stdout, text.XrayUpdating) {
		t.Errorf("update with 0:\n%s", r.stdout)
	}
}

func TestXrayMenu(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	r := runNodes(t, f, "5\n2\n0\n0\n")
	for _, want := range []string{
		" 1. 启动", " 2. 停止", " 3. 重启", " 4. 查看日志", " 5. 节点出站 IP", " 6. 更新 Xray", " 7. 卸载 Xray",
		" 7. 卸载 Xray\n", "节点出站: 默认（由系统决定）", "Xray 已停止。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if st, _ := state.NewStore(f).Load(); !st.Xray.Stopped {
		t.Error("stop not recorded")
	}
	// Node outbound IP: prefer IPv6 is applied; only IPv6 is refused
	// without an IPv6 default route.
	r = runNodes(t, f, "5\n5\n3\n5\n5\n0\n0\n")
	for _, want := range []string{"节点出站 IP 已设置为优先 IPv6。", "节点出站: 优先 IPv6", "优先 IPv6 （当前）", "本机没有可用的 IPv6 公网地址或默认路由"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("outbound IP: output missing %q:\n%s", want, r.stdout)
		}
	}
	if st, _ := state.NewStore(f).Load(); st.Xray.OutboundIP != xray.OutboundPreferIPv6 {
		t.Errorf("outbound IP = %q", st.Xray.OutboundIP)
	}
	// Main menu warns while nodes exist but Xray was stopped by hand.
	r = runNodes(t, f, "0\n")
	if !strings.Contains(r.stdout, "⚠ Xray 已手动停止") {
		t.Errorf("no manual-stop warning:\n%s", r.stdout)
	}
	// Not installed: only the note.
	g := supported()
	r = runNodes(t, g, "5\n0\n0\n")
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
	for _, want := range []string{"扫描附近网站    （推荐，约 1 分钟）", "正在扫描 203.0.113.0/24 的 443 端口", "正在扫描 253/253", "◇ 附近的网站", "│   1. near.example.net", "节点已添加。", "sni=near.example.net"} {
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

func TestClientAddChoosesNode(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "8443", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	// main: 2 clients > add > node 2 > default remark; disable client 3.
	r := runNodes(t, f, strings.Join([]string{"2", "1", "2", "", "3", "3", "0", "0"}, "\n")+"\n")
	for _, want := range []string{
		"选择节点", "1. Reality-443 (VLESS-TCP-Reality :443)", "2. Reality-8443 (VLESS-TCP-Reality :8443)",
		"#Reality-8443-2", "客户端已禁用。",
		" 3   Reality-8443  Reality-8443-2  ○ 禁用",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	st, _ := state.NewStore(f).Load()
	if len(st.Nodes) != 2 || len(st.Nodes[1].Clients) != 2 || st.Nodes[1].Clients[1].Enabled {
		t.Errorf("state = %+v", st.Nodes)
	}
	// Viewing the disabled client's link warns.
	r = runNodes(t, f, "3\n1\n3\n0\n0\n")
	if !strings.Contains(r.stdout, "该客户端或其节点已禁用") || !strings.Contains(r.stdout, "#Reality-8443-2") {
		t.Errorf("disabled link:\n%s", r.stdout)
	}
}

func TestXrayMenuStateAndLogs(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	show := []string{"show", xray.UnitName, "-p", "ActiveState", "--value"}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "active\n"}}, "systemctl", show...)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "log line\n"}}, "journalctl", "-u", xray.UnitName, "-n", "80", "--no-pager", "-o", "short-iso")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "live line\n"}}, "journalctl", "-u", xray.UnitName, "-f", "-n", "20", "--no-pager", "-o", "short-iso")
	before := len(f.Execs())
	// start while running; logs > recent, bad count, 80 lines; live; back.
	r := runNodes(t, f, "5\n1\n4\n1\n2000\n80\n2\n0\n0\n0\n")
	for _, want := range []string{
		"Xray 已在运行，没有重启。",
		"══ 主菜单 › Xray 服务 › 查看日志 ══", " 1. 最近日志", " 2. 实时日志",
		"显示行数（最多 1000，0 返回） [50]: ", "请输入 0 到 1000 之间的数字。",
		"最近 80 行日志：", "log line",
		"实时日志（按 Ctrl+C 回到菜单）：", "live line",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	if e := f.Execs()[before:]; len(e) != 0 {
		t.Errorf("start while running changed the system: %v", e)
	}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "inactive\n"}}, "systemctl", show...)
	r = runNodes(t, f, "5\n2\n0\n0\n")
	if !strings.Contains(r.stdout, "Xray 已经是停止状态。") {
		t.Errorf("stop while stopped:\n%s", r.stdout)
	}
	if st, _ := state.NewStore(f).Load(); st.Xray.Stopped {
		t.Error("stop while stopped recorded a manual stop")
	}
}

func TestXrayProgramMissing(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "8443", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	f.Respond(systemtest.Response{Err: errors.New("no such file")}, xray.BinPath, "version")
	// Delete node 1 while node 2 needs Xray; then the Xray menu.
	r := runNodes(t, f, "1\n5\n1\ny\n0\n5\n0\n0\n")
	for _, want := range []string{
		"Xray 程序不存在，无法应用需要运行的节点。请先在\"Xray 服务\"中选择\"安装 Xray\"。",
		"Xray 程序不存在（可能被手动删除），节点当前无法运行。", " 1. 安装 Xray",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	if st, _ := state.NewStore(f).Load(); len(st.Nodes) != 2 {
		t.Errorf("nodes = %d", len(st.Nodes))
	}
}

func TestDescribeXrayDownload(t *testing.T) {
	got := describe(fmt.Errorf("update: %w", &xray.DownloadError{Err: errors.New("404")}))
	if !strings.Contains(got, text.NoGitHubURL) || !strings.Contains(got, "404") {
		t.Errorf("describe = %q", got)
	}
}

// TestAddVLESSEncNodeFlow: a VLESS Encryption node asks no target, picks
// a random port and an "Enc-<port>" remark (DESIGN section 9.7); its
// modify menu has no target or fallback limit.
func TestAddVLESSEncNodeFlow(t *testing.T) {
	f := nodeSystem()
	// 1 nodes, 1 add, preset 2, port and remark default; 2 modify, node
	// 1, 2 regenerate keys (y); back out.
	input := strings.Join([]string{"1", "1", "2", "", "", "2", "1", "2", "y", "0", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{
		text.VLESSEncHint, "◆ 1/2 端口 [", "◆ 2/2 备注 [Enc-", "└─ ✓ 节点已添加。\n",
		"?encryption=mlkem768x25519plus.native.0rtt.", "&flow=xtls-rprx-vision&security=none&type=tcp#Enc-",
		"VLESS-TCP-Encryption",
		" 1. 端口", " 2. 重新生成密钥", " 3. 分享端口", " 4. 备注",
		"└─ ✓ " + text.KeysRegenerated, text.NewLinksTitle, text.VLESSEncClientHint,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	for _, unwanted := range []string{"目标网站", "回落限速", text.Non443Hint, "[443]"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("output has %q", unwanted)
		}
	}
	st, err := state.NewStore(f).Load()
	if err != nil || len(st.Nodes) != 1 {
		t.Fatalf("state = %+v, %v", st, err)
	}
	n := st.Nodes[0]
	if n.VLESSEncryption == nil || n.Reality != nil || n.Port < 10000 || n.Port > 60000 || n.Remark != "Enc-"+strconv.Itoa(n.Port) {
		t.Errorf("node = %+v", n)
	}
	if !strings.Contains(r.stdout, n.VLESSEncryption.PublicKey) {
		t.Error("links do not use the regenerated public key")
	}
	if cfg := string(f.Files[xray.ConfigPath]); !strings.Contains(cfg, `"decryption": "mlkem768x25519plus.native.600s.`+n.VLESSEncryption.PrivateKey+`"`) {
		t.Errorf("xray config:\n%s", cfg)
	}
}

// TestAddVLESSEncNodeOldXray: an installed Xray below the minimum version
// stops the flow before any question.
func TestAddVLESSEncNodeOldXray(t *testing.T) {
	f := supported()
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Xray 25.8.3 (Xray, Penetrates Everything.)\n"}}, xray.BinPath, "version")
	r := runNodes(t, f, "1\n1\n2\n0\n0\n")
	if !strings.Contains(r.stdout, text.XrayTooOldFor(text.FeatureVLESSEnc, "v25.8.3", "v25.9.5")) || strings.Contains(r.stdout, "端口 [") {
		t.Errorf("old xray:\n%s", r.stdout)
	}
	if st, _ := state.NewStore(f).Load(); len(st.Nodes) != 0 {
		t.Error("node added")
	}
}

// TestXrayUpdateWarnsVLESSEnc: updating to a version without VLESS
// Encryption warns when such a node exists.
func TestXrayUpdateWarnsVLESSEnc(t *testing.T) {
	f := nodeSystem()
	env := func() Env {
		return Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
			TargetChecker: fakeChecker{}, AddressDetector: fakeDetector{}, XrayFetcher: releaseFetcher{}}
	}
	runEnv(t, env(), strings.Join([]string{"1", "1", "2", "", "", "0", "0"}, "\n")+"\n")
	// 5 Xray, 6 update to v25.8.3 (the download fails).
	r := runEnv(t, env(), "5\n6\nv25.8.3\n0\n0\n")
	if !strings.Contains(r.stdout, text.XrayOldVersionFor(text.FeatureVLESSEnc, "v25.8.3", "v25.9.5")) {
		t.Errorf("no VLESS Encryption warning:\n%s", r.stdout)
	}
	r = runEnv(t, env(), "5\n6\nv26.3.27\n0\n0\n")
	if strings.Contains(r.stdout, "不支持 VLESS Encryption") {
		t.Errorf("warning for a supported version:\n%s", r.stdout)
	}
}

// TestAddSS2022NodeFlow covers DESIGN section 9.8: hint, random port free
// for TCP and UDP, "SS-<port>" remark, ss:// link, modify items and the
// client key reset.
func TestAddSS2022NodeFlow(t *testing.T) {
	f := nodeSystem()
	// 1 nodes, 1 add, preset 3, port and remark default; 2 modify, node
	// 1, 2 regenerate keys (y); back out.
	input := strings.Join([]string{"1", "1", "3", "", "", "2", "1", "2", "y", "0", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{
		text.SS2022Hint, "◆ 1/2 端口 [", "◆ 2/2 备注 [SS-", "└─ ✓ 节点已添加。\n",
		"ss://2022-blake3-aes-128-gcm:", "TCP 和 UDP 端口", "Shadowsocks-2022",
		" 1. 端口", " 2. 重新生成密钥", " 3. 分享端口", " 4. 备注",
		"└─ ✓ " + text.KeysRegenerated, text.NewLinksTitle,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	for _, unwanted := range []string{"目标网站", "回落限速", text.Non443Hint, "[443]", text.VLESSEncClientHint, "security=", "flow="} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("output has %q", unwanted)
		}
	}
	st, err := state.NewStore(f).Load()
	if err != nil || len(st.Nodes) != 1 {
		t.Fatalf("state = %+v, %v", st, err)
	}
	n := st.Nodes[0]
	if n.Shadowsocks == nil || n.Port < 10000 || n.Port > 60000 || n.Remark != "SS-"+strconv.Itoa(n.Port) {
		t.Errorf("node = %+v", n)
	}
	if cfg := string(f.Files[xray.ConfigPath]); !strings.Contains(cfg, `"password": "`+n.Shadowsocks.ServerKey+`"`) ||
		!strings.Contains(cfg, `"network": "tcp,udp"`) {
		t.Errorf("xray config:\n%s", cfg)
	}

	// 2 clients: 1 add (default remark); 2 modify, client 2, 2 reset key
	// (y).
	r = runNodes(t, f, strings.Join([]string{"2", "1", "", "2", "2", "2", "y", "0", "0"}, "\n")+"\n")
	for _, want := range []string{"客户端已添加。", " 2. " + text.ClientResetKey, text.ClientKeyReset} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(r.stdout, "UUID") {
		t.Error("UUID shown for a shadowsocks client")
	}
	st, _ = state.NewStore(f).Load()
	if cl := st.Nodes[0].Clients; len(cl) != 2 || cl[1].Key == "" || cl[1].UUID != "" {
		t.Errorf("clients = %+v", cl)
	}
}

// TestAddHysteria2NodeFlow covers DESIGN section 9.10: hint, random port
// free for UDP, "Hy2-<port>" remark, hysteria2:// link with the pinned
// certificate, UDP-only security group hint, certificate regeneration and
// the client password reset.
func TestAddHysteria2NodeFlow(t *testing.T) {
	f := nodeSystem()
	// 1 nodes, 1 add, preset 4, port and remark default; 2 modify, node
	// 1, 2 regenerate certificate (y); back out.
	input := strings.Join([]string{"1", "1", "4", "", "", "2", "1", "2", "y", "0", "0", "0"}, "\n") + "\n"
	r := runNodes(t, f, input)
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	st, err := state.NewStore(f).Load()
	if err != nil || len(st.Nodes) != 1 {
		t.Fatalf("state = %+v, %v", st, err)
	}
	n := st.Nodes[0]
	if n.TLS == nil || n.Port < 10000 || n.Port > 60000 || n.Remark != "Hy2-"+strconv.Itoa(n.Port) {
		t.Fatalf("node = %+v", n)
	}
	fp, err := security.CertFingerprints(n.TLS.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		text.Hy2Hint, "◆ 1/2 端口 [", "◆ 2/2 备注 [Hy2-", "└─ ✓ 节点已添加。\n",
		"hysteria2://", "/?sni=www.bing.com&pinSHA256=", "放行了 UDP 端口", "Hysteria2",
		" 1. 端口", " 2. " + text.ModRegenCert, " 3. 分享端口", " 4. 备注",
		text.ConfirmRegenCert, "└─ ✓ " + text.CertRegenerated, text.NewLinksTitle,
		// The links after regenerating pin the new certificate, and the
		// new fingerprints follow them.
		"pinSHA256=" + fp.CertHex(), text.NewFingerprintsTitle,
		"  证书 SHA-256：" + fp.CertHex() + "\n", "  公钥 SHA-256：" + fp.PublicKeyBase64() + "\n",
		// The relay's outbound settings below the first link.
		"线路机出站参数", "  端口：" + strconv.Itoa(n.Port) + "\n", "  密码：" + n.Clients[0].Key + "\n",
		"  SNI：www.bing.com\n", "certificate_public_key_sha256", "skip-cert-verify",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	for _, unwanted := range []string{"目标网站", "回落限速", text.Non443Hint, "[443]", "TCP 和 UDP", "放行了 TCP"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("output has %q", unwanted)
		}
	}
	if cfg := string(f.Files[xray.ConfigPath]); !strings.Contains(cfg, `"auth": "`+n.Clients[0].Key+`"`) ||
		!strings.Contains(cfg, `"network": "hysteria"`) || !strings.Contains(cfg, "-----BEGIN PRIVATE KEY-----") {
		t.Errorf("xray config:\n%s", cfg)
	}

	// 2 clients: 1 add (default remark); 2 modify, client 2, 2 reset
	// password (y).
	r = runNodes(t, f, strings.Join([]string{"2", "1", "", "2", "2", "2", "y", "0", "0"}, "\n")+"\n")
	for _, want := range []string{"客户端已添加。", " 2. " + text.ClientResetPass, text.ClientPassReset} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	st, _ = state.NewStore(f).Load()
	if cl := st.Nodes[0].Clients; len(cl) != 2 || len(cl[1].Key) != 32 || cl[1].UUID != "" || cl[1].Key == cl[0].Key {
		t.Errorf("clients = %+v", cl)
	}
}

// TestAddHysteria2NodeOldXray: an Xray without the Hysteria2 inbound
// stops the flow before any question.
func TestAddHysteria2NodeOldXray(t *testing.T) {
	f := supported()
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Xray 26.2.6 (Xray, Penetrates Everything.)\n"}}, xray.BinPath, "version")
	r := runNodes(t, f, "1\n1\n4\n0\n0\n")
	if !strings.Contains(r.stdout, text.XrayTooOldFor(text.FeatureHysteria, "v26.2.6", "v26.3.27")) || strings.Contains(r.stdout, "端口 [") {
		t.Errorf("old xray:\n%s", r.stdout)
	}
	if st, _ := state.NewStore(f).Load(); len(st.Nodes) != 0 {
		t.Error("node added")
	}
}
