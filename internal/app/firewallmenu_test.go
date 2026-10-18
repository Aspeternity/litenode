package app

import (
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func firewallSystem() *systemtest.Fake {
	f := sshSystem(effective22)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", firewall.FirewalldUnit)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "systemctl", "cat", firewall.DockerUnit)
	return f
}

func TestFirewallMenu(t *testing.T) {
	f := firewallSystem()
	// main: 5 firewall; enable, yes, keep; disable, yes, keep; back; quit.
	r := runSSH(t, f, "5", "1", "y", "y", "1", "y", "y", "0", "0")
	for _, want := range []string{
		" 防火墙: ○ 未启用",
		" 1. 安装并启用防火墙  (自动放行当前 SSH、节点、转发端口)",
		"启用后默认拒绝入站，只放行以下端口：",
		"[1]  22/tcp  任意来源  SSH",
		"防火墙已启用。",
		" 防火墙: ● 已启用   默认: 拒绝入站   ping: 允许",
		" 1. 关闭防火墙",
		" 2. 重置所有规则",
		"防火墙已关闭。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	if strings.Contains(string(f.Files[nft.ConfigPath]), "chain input") {
		t.Error("table still filters after disable")
	}
}

func TestFirewallMenuUFWActive(t *testing.T) {
	f := firewallSystem()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	r := runSSH(t, f, "5", "1", "0", "0")
	if !strings.Contains(r.stdout, "ufw 正在运行。请先执行 ufw disable") {
		t.Errorf("ufw not reported:\n%s", r.stdout)
	}
	if _, ok := f.Files[nft.ConfigPath]; ok {
		t.Error("enabled despite ufw")
	}
	// Both on: the main menu warns.
	store := state.NewStore(f)
	st, _ := store.Load()
	st.Firewall.Enabled = true
	store.Save(st)
	r = runSSH(t, f, "0")
	if !strings.Contains(r.stdout, "LiteNode 防火墙和 ufw 同时启用") || !strings.Contains(r.stdout, "防火墙: ● 已启用") {
		t.Errorf("main menu:\n%s", r.stdout)
	}
}
