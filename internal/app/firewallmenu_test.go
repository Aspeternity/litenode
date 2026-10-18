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
	// main: 7 firewall; enable, yes, keep; disable, yes, keep; back; quit.
	r := runSSH(t, f, "7", "1", "y", "y", "6", "y", "y", "0", "0")
	for _, want := range []string{
		" 防火墙: ○ 未启用",
		" 1. 安装并启用防火墙\n",
		"启用后默认拒绝入站，只放行以下端口：",
		"[1]  22/tcp  任意来源  SSH",
		"防火墙已启用。",
		" 防火墙: ● 已启用   默认: 拒绝入站   ping: 允许",
		" 1. 放行端口\n",
		" 2. 修改规则\n",
		" 3. 允许 / 禁止 ping",
		" 4. 检查并修复规则\n",
		" 5. 删除规则",
		" 6. 关闭防火墙",
		" 7. 重置所有规则",
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
	r := runSSH(t, f, "7", "1", "0", "0")
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

// enabledFirewall is a system with the LiteNode firewall on.
func enabledFirewall(t *testing.T) *systemtest.Fake {
	t.Helper()
	f := firewallSystem()
	store := state.NewStore(f)
	st, _ := store.Load()
	st.Firewall.Enabled = true
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFirewallRuleManagement(t *testing.T) {
	f := enabledFirewall(t)
	r := runSSH(t, f,
		"7",
		// open: bad port, range, TCP, bad source, two sources, note.
		"1", "99999", "28000-28010", "2", "1.2.3", "1.2.3.4, 10.0.0.0/8", "备注",
		// the same again is refused.
		"1", "28000-28010", "2",
		// modify rule 2: sources to anyone, then back twice.
		"2", "2", "3", "", "0",
		// ping off.
		"3",
		// delete SSH (automatic): refused; delete rule 2, yes.
		"5", "1", "5", "2", "y",
		"0", "0")
	for _, want := range []string{
		"端口无效",
		"来源 IP 无效",
		"[2]  28000-28010/tcp  1.2.3.4,10.0.0.0/8  手动 备注",
		"已放行。",
		"已有相同端口和协议的手动规则",
		"规则已修改。",
		"[2]  28000-28010/tcp  任意来源",
		"已禁止 ping",
		"ping: 禁止",
		"该规则由 SSH 自动管理，请在 SSH 管理中修改。",
		"确定删除规则 [2] 28000-28010/tcp？",
		"规则已删除。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	conf := string(f.Files[nft.ConfigPath])
	if strings.Contains(conf, "28000") || strings.Contains(conf, "icmp type echo-request accept") {
		t.Errorf("rules after delete and ping off:\n%s", conf)
	}
}

func TestFirewallAutoSources(t *testing.T) {
	f := enabledFirewall(t)
	// modify rule 1 (SSH): set the source, keep it (the change limits SSH).
	r := runSSH(t, f, "7", "2", "1", "1", "1.2.3.4", "y", "0", "0", "0")
	for _, want := range []string{
		"该规则由 SSH 自动管理，这里只能设置来源 IP 限制",
		"规则已修改。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	if !strings.Contains(string(f.Files[nft.ConfigPath]), `ip saddr { 1.2.3.4 } tcp dport 22 accept comment "litenode:ssh"`) {
		t.Errorf("SSH not limited:\n%s", f.Files[nft.ConfigPath])
	}
}

func TestFirewallCheck(t *testing.T) {
	f := enabledFirewall(t)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, nft.NftPath, "list", "chain", "inet", "litenode", "input")
	r := runSSH(t, f, "7", "4", "y", "y", "0", "0")
	for _, want := range []string{"防火墙规则没有加载", "重新应用防火墙规则？", "已重新应用防火墙规则。"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
}
