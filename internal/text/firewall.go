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

// FirewallForward labels a forward's rule by its number in the forward
// list.
func FirewallForward(index int) string { return fmt.Sprintf("转发 %d", index) }

// FirewallForeign warns about filtering other software installed.
func FirewallForeign(where []string) string {
	return fmt.Sprintf("⚠ 检测到系统已有入站过滤规则（%s）。这些规则与 LiteNode 防火墙同时生效，可能拦截 LiteNode 放行的端口。", strings.Join(where, "、"))
}

// Firewall rule management (DESIGN section 14, D-073).
const (
	FirewallOpen        = "放行端口"
	FirewallModify      = "修改规则"
	FirewallPing        = "允许 / 禁止 ping"
	FirewallCheck       = "检查并修复规则"
	FirewallDelete      = "删除规则"
	PromptFirewallPorts = "端口（单个端口或范围，如 443 或 10000-10100，0 返回）"
	PromptFirewallProto = "协议"
	ProtoTCPUDP         = "TCP + UDP"
	ProtoTCP            = "TCP"
	ProtoUDP            = "UDP"
	PromptSources       = "来源 IP（可填多个，用逗号分隔；网段如 1.2.3.0/24；留空表示任意来源；0 返回）"
	PromptRuleNote      = "备注（可留空）"
	PromptRuleNumber    = "请输入规则序号（0 返回）"
	InvalidPorts        = "端口无效：请输入 1 到 65535 之间的端口，或 起始-结束 的范围（如 10000-10100）。"
	InvalidSources      = "来源 IP 无效：请输入 IPv4 / IPv6 地址或网段，多个用逗号分隔。"
	ErrRuleExists       = "已有相同端口和协议的手动规则，请用\"修改规则\"修改它。"
	ErrNoRule           = "规则不存在，可能已被删除。"
	FirewallRuleAdded   = "已放行。"
	FirewallRuleUpdated = "规则已修改。"
	FirewallRuleDeleted = "规则已删除。"
	FirewallPingAllowed = "已允许 ping。"
	FirewallPingBlocked = "已禁止 ping（IPv6 邻居发现等必要的 ICMPv6 仍然放行）。"
	ModRulePorts        = "端口"
	ModRuleProto        = "协议"
	ModRuleSources      = "来源 IP"
	ModRuleNote         = "备注"
	FirewallCheckOK     = "已加载的规则与设置一致，无需修复。"
	FirewallNotLoaded   = "防火墙规则没有加载（nftables 表不存在）。"
	FirewallBaseDiffers = "基础规则（默认拒绝、ping、IPv6 邻居发现等）与设置不一致。"
	ConfirmRepair       = "重新应用防火墙规则？"
	FirewallRepaired    = "已重新应用防火墙规则。"
)

// Protocol names a rule's protocol for menus.
func Protocol(p string) string {
	switch p {
	case "tcp":
		return ProtoTCP
	case "udp":
		return ProtoUDP
	}
	return ProtoTCPUDP
}

// Where an automatic rule is changed, for FirewallAutoRule.
const (
	FirewallInNodes    = "节点管理"
	FirewallInSSH      = " SSH 管理"
	FirewallInForwards = "端口转发"
)

// FirewallAutoRule explains that an automatic rule is changed elsewhere;
// owner is the rule's label (SSH, 节点 1), where the menu to change it in.
func FirewallAutoRule(owner, where string) string {
	return fmt.Sprintf("该规则由%s自动管理，请在%s中修改。", spaced(owner), where)
}

// FirewallAutoSources introduces the source limit of an automatic rule.
func FirewallAutoSources(owner string) string {
	return fmt.Sprintf("该规则由%s自动管理，这里只能设置来源 IP 限制（端口变化后仍然保留）。", spaced(owner))
}

// ConfirmDeleteRule asks before deleting a rule.
func ConfirmDeleteRule(index int, rule string) string {
	return fmt.Sprintf("确定删除规则 [%d] %s？", index, rule)
}

// FirewallMissing, FirewallExtra and FirewallChanged list the differences
// "check and repair" found.
func FirewallMissing(owner string) string { return "缺少：" + owner }
func FirewallExtra(owner string) string   { return "多余：" + owner }
func FirewallChanged(owner string) string { return "不一致：" + owner }

// spaced puts a space after a label, and before it when it starts with a
// Latin letter ("由 SSH 自动", "由节点 1 自动").
func spaced(label string) string {
	if label != "" && label[0] < 0x80 {
		return " " + label + " "
	}
	return label + " "
}
