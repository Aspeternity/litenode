package app

import (
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/xray"
)

// checkSystem has one REALITY node and answers the probe table listing
// with probe (nft -j output).
func checkSystem(t *testing.T, probe string) *systemtest.Fake {
	t.Helper()
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: probe}}, "nft", "-j", "list", "table", "inet", "litenode-probe")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "ActiveState=active\n"}}, "systemctl", "show", xray.UnitName,
		"-p", "ActiveState", "-p", "ActiveEnterTimestampMonotonic", "-p", "MemoryCurrent")
	return f
}

func probeJSON(arrived, passed, src string) string {
	return `{"nftables": [{"counter": {"name": "arrive_tcp", "table": "litenode-probe", "packets": ` + arrived + `}},
{"counter": {"name": "pass_tcp", "table": "litenode-probe", "packets": ` + passed + `}},
{"set": {"name": "src_tcp4", "table": "litenode-probe", "elem": [` + src + `]}}]}`
}

func TestNodeCheckOpen(t *testing.T) {
	f := checkSystem(t, probeJSON("12", "12", `"192.0.2.7"`))
	// nodes > check (4) > node 1 > Enter (piped: ends at once)
	r := runNodes(t, f, "1\n4\n1\n\n0\n0\n")
	for _, want := range []string{
		"请现在用客户端连接这个节点",
		"│   检测结论：\n│     TCP：连通（到达 12 个包，通过 12 个）\n│       本机这一侧是通的。",
		"│       来源：192.0.2.7\n", "└─ 检测结束。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, r.stdout)
		}
	}
	var loads []string
	for _, c := range f.Calls() {
		if c.Command == "nft -f -" {
			loads = append(loads, string(c.Data))
		}
	}
	if len(loads) != 2 || !strings.Contains(loads[0], "tcp dport 443 counter") || strings.Contains(loads[0], "udp") ||
		loads[1] != "table inet litenode-probe\ndelete table inet litenode-probe\n" {
		t.Errorf("nft loads: %q", loads)
	}
}

func TestNodeCheckBlockedAndUnreached(t *testing.T) {
	f := checkSystem(t, probeJSON("9", "0", ""))
	r := runNodes(t, f, "1\n4\n1\n\n0\n0\n")
	if !strings.Contains(r.stdout, "TCP：本机拦截（到达 9 个包，通过 0 个）\n│       被系统中其他入站过滤规则拦截") {
		t.Errorf("blocked, no firewall:\n%s", r.stdout)
	}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	if r := runNodes(t, f, "1\n4\n1\n\n0\n0\n"); !strings.Contains(r.stdout, "被 ufw 拦截") {
		t.Errorf("blocked by ufw:\n%s", r.stdout)
	}
	f = checkSystem(t, probeJSON("0", "0", ""))
	if r := runNodes(t, f, "1\n4\n1\n\n0\n0\n"); !strings.Contains(r.stdout, "TCP：未到达（到达 0 个包，通过 0 个）\n│       没有收到数据包") {
		t.Errorf("unreached:\n%s", r.stdout)
	}
}

func TestNodeCheckPreconditions(t *testing.T) {
	f := checkSystem(t, "")
	// Disable the node (3), then check it.
	r := runNodes(t, f, "1\n3\n1\n4\n1\n0\n0\n")
	if !strings.Contains(r.stdout, "节点已禁用，请先启用节点再检测。") {
		t.Errorf("disabled node:\n%s", r.stdout)
	}
	for _, c := range f.Calls() {
		if c.Command == "nft -f -" {
			t.Error("probe table loaded for a disabled node")
		}
	}
}
