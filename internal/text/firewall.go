package text

import (
	"fmt"
	"strings"
)

// Firewall menu (DESIGN section 14).
const (
	FirewallOff       = "○ 未启用"
	FirewallOn        = "● 已启用"
	FirewallEnable    = "安装并启用防火墙"
	FirewallEnableHnt = "(自动放行当前 SSH、节点、转发端口)"
	FirewallDisable   = "关闭防火墙"
	FirewallReset     = "重置所有规则"
	FirewallRules     = "规则:"
	FirewallNoRules   = "规则: （无）"
	FirewallAnySource = "任意来源"
	FirewallSSH       = "SSH"
	FirewallManual    = "手动"

	FirewallUFWActive       = "ufw 正在运行。请先执行 ufw disable 停用 ufw，再启用 LiteNode 防火墙（两套防火墙同时启用会互相干扰）。"
	FirewallFirewalldActive = "firewalld 正在运行。请先执行 systemctl disable --now firewalld 停用，再启用 LiteNode 防火墙。"
	FirewallDocker          = "⚠ 检测到 Docker：Docker 会自行修改 nftables / iptables 规则，可能与防火墙冲突，容器发布的端口不受本防火墙控制。"
	FirewallNftInstall      = "未安装 nftables，将先通过 apt 安装。"
	FirewallWillOpen        = "启用后默认拒绝入站，只放行以下端口："
	ConfirmFirewallEnable   = "确定启用防火墙？"
	ConfirmFirewallDisable  = "关闭后所有端口都不再过滤（规则会保留，下次启用时恢复）。确定关闭？"
	ConfirmFirewallReset    = "将删除全部手动规则和自动规则的来源限制，并恢复允许 ping。确定重置？"
	FirewallEnabled         = "防火墙已启用。"
	FirewallDisabled        = "防火墙已关闭。"
	FirewallResetDone       = "已重置所有规则。"
	FirewallUFWBoth         = "⚠ LiteNode 防火墙和 ufw 同时启用，端口须被两者都放行才能访问。请停用其中一个（ufw disable 或在防火墙管理中关闭）。"

	ErrFirewallEnabled  = "防火墙已经启用。"
	ErrFirewallDisabled = "防火墙未启用。"
)

// FirewallStatusLine is the first status line of the enabled firewall.
func FirewallStatusLine(state string, ping bool) string {
	p := "允许"
	if !ping {
		p = "禁止"
	}
	return fmt.Sprintf("防火墙: %s   默认: 拒绝入站   ping: %s", state, p)
}

// FirewallNode labels a node's rule by its number in the node list.
func FirewallNode(index int) string { return fmt.Sprintf("节点 %d", index) }

// FirewallForeign warns about filtering other software installed.
func FirewallForeign(where []string) string {
	return fmt.Sprintf("⚠ 检测到系统已有入站过滤规则（%s）。这些规则与 LiteNode 防火墙同时生效，可能拦截 LiteNode 放行的端口。", strings.Join(where, "、"))
}
