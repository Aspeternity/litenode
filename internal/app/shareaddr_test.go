package app

import (
	"context"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/egress"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

type dualDetector struct{ none bool }

func (d dualDetector) Detect(context.Context) share.Detected {
	if d.none {
		return share.Detected{}
	}
	return share.Detected{IPv4: "203.0.113.10", IPv6: "2001:db8::10", Source: share.SourceTrace}
}
func (dualDetector) OnInterface(string) bool { return true }

type fakeDNS map[string]string

func (f fakeDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if ip, ok := f[host]; ok {
		return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func runAddr(t *testing.T, f *systemtest.Fake, d dualDetector, input ...string) result {
	t.Helper()
	return runEnv(t, Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
		TargetChecker: fakeChecker{}, AddressDetector: d,
		Resolver: fakeDNS{"here.example.com": "203.0.113.10", "cdn.example.com": "104.16.1.1"}},
		strings.Join(input, "\n")+"\n")
}

func TestAddNodeDoesNotAskAddress(t *testing.T) {
	f := nodeSystem()
	// nodes > add > preset 1 > port default > target manual > remark default.
	r := runAddr(t, f, dualDetector{}, "1", "1", "1", "", "3", "www.example.com", "", "0", "0")
	for _, want := range []string{
		"检测到公网地址：203.0.113.10、2001:db8::10（默认 203.0.113.10）",
		"@203.0.113.10:443?", "#Reality-443\n",
		"其他主机地址（如 IPv6、域名）的链接：主菜单 → 分享节点。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(r.stdout, "选择主机地址") {
		t.Error("add node asked for the share address")
	}
	// The IPv6 link, chosen in the share menu (one client: no choice), has
	// a suffixed remark.
	r = runAddr(t, f, dualDetector{}, "3", "1", "2", "0", "0")
	for _, want := range []string{
		"═ 主菜单 › 分享节点 ═", " 主机地址: 203.0.113.10（另有 1 个）", " 1   Reality-443  Reality-443  ● 启用",
		" 1. 导出单个链接", " 2. 导出全部链接", " 3. 配置主机地址\n",
		"选择链接使用的地址", "@[2001:db8::10]:443?", "#Reality-443-IPv6", "▀",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("share menu missing %q", want)
		}
	}
	if strings.Contains(r.stdout, "请输入客户端序号") {
		t.Error("asked for the only client")
	}
	// All links: text only, for the default address.
	r = runAddr(t, f, dualDetector{}, "3", "2", "1", "0", "0")
	if !strings.Contains(r.stdout, "全部链接：\nReality-443 / Reality-443:\nvless://") || !strings.Contains(r.stdout, "@203.0.113.10:443?") || strings.Contains(r.stdout, "▀") {
		t.Errorf("all links:\n%s", r.stdout)
	}
}

func TestAddNodeNothingDetectedBackCancels(t *testing.T) {
	// No address detected and an empty answer: back to the node list,
	// not on to the port (M1 acceptance, 2026-10-18).
	r := runAddr(t, nodeSystem(), dualDetector{none: true}, "1", "1", "1", "", "0", "0")
	if !strings.Contains(r.stdout, "未能检测到公网地址，请手动输入。") || strings.Contains(r.stdout, "端口 [") {
		t.Errorf("output:\n%s", r.stdout)
	}
}

func TestShareAddressPage(t *testing.T) {
	f := nodeSystem()
	st := state.New()
	st.Server.ShareAddresses = []string{"203.0.113.10", "2001:db8::10"}
	state.NewStore(f).Save(st)
	// share > host address page (1 without clients): add a domain pointing
	// here; add one behind a CDN (warned, declined); set the domain as
	// default; delete the IPv6 address.
	r := runAddr(t, f, dualDetector{}, "3", "1",
		"1", "here.example.com",
		"1", "cdn.example.com", "n",
		"2", "3",
		"4", "3",
		"0", "0", "0")
	for _, want := range []string{
		"══════ 主菜单 › 分享节点 › 配置主机地址 ══════",
		"  [1] 203.0.113.10  IPv4  默认",
		"  [2] 2001:db8::10  IPv6",
		"地址已添加。",
		"cdn.example.com 解析到 104.16.1.1，不是本机的地址",
		"默认主机地址已修改",
		"  [1] here.example.com  域名  默认",
		"地址已删除。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	got, _ := state.NewStore(f).Load()
	if want := []string{"here.example.com", "203.0.113.10"}; strings.Join(got.Server.ShareAddresses, ",") != strings.Join(want, ",") {
		t.Errorf("addresses = %v", got.Server.ShareAddresses)
	}
}

// v6Detector finds only an IPv6 address, as on an IPv6-only host with
// WARP for IPv4.
type v6Detector struct{}

func (v6Detector) Detect(context.Context) share.Detected {
	return share.Detected{IPv6: "2001:db8::10", Source: share.SourceTrace}
}
func (v6Detector) OnInterface(string) bool { return true }

// TestWARPShareAddressReplaced: a WARP exit saved as the default address
// by an older version is replaced when a node is added (D-132).
func TestWARPShareAddressReplaced(t *testing.T) {
	f := nodeSystem()
	st := state.New()
	st.Server.ShareAddresses = []string{"104.28.215.68", "here.example.com"}
	state.NewStore(f).Save(st)
	eg := fakeEgress{IPv4: egress.Family{OK: true, WARP: true, IP: netip.MustParseAddr("104.28.215.68")},
		IPv6: egress.Family{OK: true, IP: netip.MustParseAddr("2001:db8::10")}}
	r := runEnv(t, Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
		TargetChecker: fakeChecker{}, AddressDetector: v6Detector{}, Egress: eg},
		strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	if !strings.Contains(r.stdout, "@[2001:db8::10]:443?") {
		t.Errorf("link not on IPv6:\n%s", r.stdout)
	}
	got, _ := state.NewStore(f).Load()
	if want := "2001:db8::10,here.example.com"; strings.Join(got.Server.ShareAddresses, ",") != want {
		t.Errorf("addresses = %v, want %s", got.Server.ShareAddresses, want)
	}
}
