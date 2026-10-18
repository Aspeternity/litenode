package app

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
)

// dialerFunc fails the connections to the addresses it returns an error
// for.
type dialerFunc func(addr string) error

func (d dialerFunc) DialContext(_ context.Context, _, addr string) (net.Conn, error) {
	return nil, d(addr)
}

func runForward(t *testing.T, f *systemtest.Fake, d dialerFunc, input ...string) result {
	t.Helper()
	return runEnv(t, Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
		ForwardResolver: resolveTo("203.0.113.20"), ForwardDialer: d}, strings.Join(input, "\n")+"\n")
}

func loadForwards(t *testing.T, f *systemtest.Fake) []forward.Forward {
	t.Helper()
	st, err := state.NewStore(f).Load()
	if err != nil {
		t.Fatal(err)
	}
	return st.Forwards
}

func wantOutput(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n%s", w, out)
		}
	}
}

func TestForwardMenuAddToggleDelete(t *testing.T) {
	f := supported()
	// main: 4 forwards; add: port, tcp+udp, target, default port, remark;
	// toggle 1; delete 1, yes; back; quit.
	r := runForward(t, f, nil, "4", "1", "30001", "1", "203.0.113.5", "", "LA",
		"3", "1", "5", "1", "y", "0", "0")
	wantOutput(t, r.stdout,
		"═════════════ 主菜单 › 端口转发 ══════════════",
		text.ForwardNone,
		"◆ 1/5 本机端口",
		"◆ 2/5 协议",
		"◆ 4/5 目标端口 [30001]:",
		"✓ 转发规则已添加。",
		" ID  本机端口  协议     目标                        状态    备注",
		" 1   30001     tcp+udp  203.0.113.5:30001  仅 IPv4  ● 启用  LA",
		" 2. 修改规则",
		" 3. 启用 / 禁用规则",
		" 4. 测试目标连通性",
		" 5. 删除规则",
		"✓ 转发规则已禁用。",
		"○ 禁用",
		"确定删除转发规则 1（30001 → 203.0.113.5:30001）？",
		"✓ 转发规则已删除。",
	)
	if len(loadForwards(t, f)) != 0 {
		t.Error("forward not deleted")
	}
}

func TestForwardMenuMainStatus(t *testing.T) {
	f := domainState(t)
	r := runForward(t, f, nil, "0")
	wantOutput(t, r.stdout, "转发规则: 1")
}

func TestForwardMenuDomainAndRange(t *testing.T) {
	f := supported()
	// Target range of another length is asked again.
	r := runForward(t, f, nil, "4", "1", "30000-30010", "2", "la.example.com", "40000", "40000-40010", "", "0", "0")
	wantOutput(t, r.stdout, text.ErrTargetPortLength, "✓ 转发规则已添加。",
		"30000-30010  tcp   la.example.com:40000-40010  仅 IPv4  ● 启用")
	fs := loadForwards(t, f)
	if len(fs) != 1 || fs[0].ResolvedIPv4 != "203.0.113.20" {
		t.Fatalf("%+v", fs)
	}
	if conf := string(f.Files[nft.ConfigPath]); !strings.Contains(conf, "203.0.113.20") {
		t.Errorf("rules:\n%s", conf)
	}
}

func TestForwardMenuPortConflict(t *testing.T) {
	f := supported()
	store := state.NewStore(f)
	st := state.New()
	st.Nodes = []node.Node{{ID: 1, Remark: "n", Enabled: true, Port: 443}}
	st.Forwards = []forward.Forward{{ID: 2, Enabled: false, Protocol: forward.UDP, ListenPort: "30000-30005",
		TargetHost: "203.0.113.5", TargetPort: "30000-30005"}}
	st.NextID = 3
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	// A node's TCP port, then a disabled forward's UDP port (D-037), then
	// cancel at the port step.
	r := runForward(t, f, nil, "4", "1", "443", "2", "30003", "1", "0", "0", "0")
	wantOutput(t, r.stdout, text.PortTaken(443, "tcp", "node:1"), text.PortTaken(30003, "udp", "forward:2"), "└─ 已取消")
	if len(loadForwards(t, f)) != 1 {
		t.Error("conflicting forward added")
	}
	// Enabling checks again; its own ports do not conflict with itself.
	r = runForward(t, f, nil, "4", "3", "1", "0", "0")
	wantOutput(t, r.stdout, "✓ 转发规则已启用。")
}

func TestForwardMenuModify(t *testing.T) {
	f := domainState(t)
	// Modify 1: keep port, tcp, new target, keep port and remark.
	r := runForward(t, f, nil, "4", "2", "1", "", "2", "198.51.100.7", "", "", "0", "0")
	wantOutput(t, r.stdout, "◆ 1/5 本机端口（单个端口或 起始-结束，0 返回） [30001]:", "2. TCP             (当前)", "✓ 转发规则已修改。")
	if fs := loadForwards(t, f); fs[0].TargetHost != "198.51.100.7" || fs[0].ID != 1 || !fs[0].Enabled {
		t.Errorf("%+v", fs)
	}
	// Nothing changed.
	r = runForward(t, f, nil, "4", "2", "1", "", "2", "", "", "", "0", "0")
	wantOutput(t, r.stdout, "└─ "+text.ForwardNoChange)
}

func TestForwardMenuProbe(t *testing.T) {
	f := supported()
	st := state.New()
	st.Forwards = []forward.Forward{
		{ID: 1, Enabled: true, Protocol: forward.TCPUDP, ListenPort: "30001", TargetHost: "la.example.com",
			TargetPort: "443", ResolvedIPv4: "203.0.113.20", ResolvedIPv6: "2001:db8::20"},
		{ID: 2, Enabled: true, Protocol: forward.UDP, ListenPort: "30002", TargetHost: "203.0.113.5", TargetPort: "443"},
	}
	st.NextID = 3
	if err := state.NewStore(f).Save(st); err != nil {
		t.Fatal(err)
	}
	d := dialerFunc(func(addr string) error {
		if addr == "[2001:db8::20]:443" {
			return errors.New("i/o timeout")
		}
		return nil
	})
	r := runForward(t, f, d, "4", "4", "1", "4", "2", "0", "0")
	wantOutput(t, r.stdout, text.ProbeTCPOnly, "203.0.113.20:443  连通，延迟",
		"[2001:db8::20]:443  无法连接：i/o timeout", text.ProbeSomeFails, "└─ "+text.ProbeUDPOnly)
}
