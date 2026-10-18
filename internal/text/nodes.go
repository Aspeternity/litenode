package text

import (
	"fmt"
	"strings"
)

// WithDefault formats a prompt with a default value.
func WithDefault(label, def string) string {
	return fmt.Sprintf("%s [%s]: ", label, def)
}

// InvalidNumber is shown for a number outside min..max.
func InvalidNumber(min, max int) string {
	return fmt.Sprintf("请输入 %d 到 %d 之间的数字。", min, max)
}

// Node list menu.
const (
	NodeAdd            = "添加节点"
	NodeModify         = "修改节点"
	NodeToggle         = "启用 / 禁用节点"
	NodeClients        = "客户端管理"
	NodeShareAddress   = "分享地址"
	NodeShareAddrHint  = "(修改 / 重新检测)"
	NodeDelete         = "删除节点"
	NodeNone           = "（暂无节点）"
	ColID              = "ID"
	ColRemark          = "备注"
	ColProtocol        = "协议"
	ColPort            = "端口"
	ColClients         = "客户端"
	ColState           = "状态"
	StateEnabled       = "● 启用"
	StateDisabled      = "○ 禁用"
	Recommended        = "★推荐"
	NotSet             = "未设置"
	PromptNodeNumber   = "请输入节点序号"
	PromptPort         = "端口"
	PromptRemark       = "备注"
	PromptTarget       = "目标网站域名（可带端口，默认 443）"
	PromptShareAddress = "分享地址（IP 或域名）"
	PromptSharePort    = "分享端口（0 表示与监听端口相同）"
	Checking           = "正在检测……"
	InstallingXray     = "首次添加节点，正在安装 Xray……"
	ApplyingChange     = "正在应用配置……"
	Done               = "完成。"
	Canceled           = "已取消。"
)

// ShareAddressLine is the status line of the node list.
func ShareAddressLine(addr string) string {
	if addr == "" {
		addr = NotSet
	}
	return "分享地址: " + addr
}

// SharePortCurrent describes the share port in the modify menu.
func SharePortCurrent(sharePort int) string {
	if sharePort == 0 {
		return "当前: 同监听端口"
	}
	return fmt.Sprintf("当前: %d", sharePort)
}

// Current formats a "current value" hint.
func Current(v any) string { return fmt.Sprintf("当前: %v", v) }

// ChoosePreset heads the preset menu.
const ChoosePreset = "选择节点类型"

// Target selection (DESIGN section 10.1).
const (
	ChooseTarget      = "选择目标网站 (SNI)"
	TargetCommon      = "从常用列表选择"
	TargetManual      = "手动输入"
	TargetNoneUsable  = "常用列表中没有通过检测的网站，请手动输入。"
	CommonListHint    = "提示：常用列表是大型网站，在部分地区可能受到干扰；以后版本推荐使用\"扫描附近网站\"。"
	TargetNotUsable   = "该网站未通过检测，不能使用。"
	TargetUseAnyway   = "仍要使用这个网站吗？"
	PromptTargetIndex = "请输入网站序号"
)

// CheckLine formats one target check result (DESIGN section 10.2).
func CheckLine(host string, ms int64, tls13, h2, x25519, cert bool) string {
	mark := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}
	return fmt.Sprintf("%s  %4d ms  TLS1.3 %s  H2 %s  X25519 %s  证书 %s", host, ms, mark(tls13), mark(h2), mark(x25519), mark(cert))
}

// MLKEMUnsupported explains a target that negotiates ML-KEM with an Xray
// that cannot handle it (D-054).
func MLKEMUnsupported(min string) string {
	return fmt.Sprintf("该网站使用 X25519MLKEM768 密钥交换，需要 Xray %s 或更高版本才能作为 Reality 目标。", min)
}

// CheckUnreachable is shown when a target cannot be reached.
func CheckUnreachable(host string, err error) string {
	return fmt.Sprintf("%s  无法连接：%v", host, err)
}

// RiskyKeyword warns about a high-risk domain (DESIGN section 10.4).
func RiskyKeyword(kw string) string {
	return fmt.Sprintf("⚠ 域名包含高风险关键词 \"%s\"，可能被他人利用回落偷跑流量。", kw)
}

// LargeHomepage warns about a large homepage.
func LargeHomepage(size int64) string {
	return fmt.Sprintf("⚠ 首页较大（%.1f MB），回落限速可能影响正常访问，也更容易被偷跑流量。", float64(size)/(1<<20))
}

// HomepageSize shows the measured homepage size.
func HomepageSize(size int64) string {
	if size < 0 {
		return "首页大小: 未知"
	}
	return fmt.Sprintf("首页大小: %.2f MB", float64(size)/(1<<20))
}

// Port step hints.
const (
	Non443Hint = "提示：Reality 使用 443 以外的端口更容易被识别。"
	NATHint    = "提示：检测到的公网 IP 不在本机网卡上，这可能是 NAT 机。请输入商家分配的端口；外部端口与内部端口不同时，添加后在\"修改节点\"中设置分享端口。"
)

// PortTaken explains a port conflict. owner is "" for another program.
func PortTaken(port int, proto, owner string) string {
	if owner == "" {
		return fmt.Sprintf("端口 %d/%s 已被系统中的其他程序占用。", port, proto)
	}
	return fmt.Sprintf("端口 %d/%s 已被 %s 使用。", port, proto, ownerName(owner))
}

func ownerName(owner string) string {
	kind, id, _ := strings.Cut(owner, ":")
	switch kind {
	case "node":
		return "节点（ID " + id + "）"
	case "forward":
		return "转发规则（ID " + id + "）"
	case "ssh":
		return "SSH"
	}
	return owner
}

// Errors shown for node operations.
const (
	ErrRemarkEmpty     = "备注不能为空。"
	ErrRemarkControl   = "备注不能包含控制字符。"
	ErrRemarkDuplicate = "备注已被使用（不区分大小写），请换一个。"
	ErrNodeNotFound    = "节点不存在。"
	ErrInvalidAddress  = "请输入有效的 IP 地址或域名。"
)

// ErrRemarkTooLong is shown for an over-long remark.
func ErrRemarkTooLong(max int) string {
	return fmt.Sprintf("备注最多 %d 个字符。", max)
}

// XrayInstallFailed explains a failed Xray download (D-042).
func XrayInstallFailed(err error) string {
	return fmt.Sprintf("安装 Xray 失败：%v\n如果这台机器无法访问 GitHub，请参考 README 中的手动安装说明。", err)
}

// ChangeFailed explains a failed configuration change; restored reports
// whether the previous configuration is back.
func ChangeFailed(err error, restored bool) string {
	if restored {
		return fmt.Sprintf("应用配置失败，已恢复修改前的配置：%v", err)
	}
	return fmt.Sprintf("应用配置失败，且恢复也失败：%v\n请检查 /etc/litenode/backups/ 中的备份，或通过 VNC 控制台处理。", err)
}

// After adding a node.
const (
	NodeAdded         = "节点已添加。"
	ShareLinkTitle    = "分享链接："
	SecurityGroupHint = "连不上时，请检查云厂商的安全组或防火墙是否放行了 TCP 端口 %d。"
)

// SecurityGroup formats the security group hint.
func SecurityGroup(port int) string { return fmt.Sprintf(SecurityGroupHint, port) }

// ForeignFilter warns about existing inbound filtering (D-044).
func ForeignFilter(port int, where []string) string {
	return fmt.Sprintf("⚠ 检测到系统已有入站过滤规则（%s），可能拦截端口 %d。可以手动放行，或启用 LiteNode 防火墙统一管理。", strings.Join(where, "、"), port)
}

// Share address.
const (
	ShareAddrEnter    = "输入地址"
	ShareAddrDetect   = "重新检测"
	ChooseShareAddr   = "检测到多个公网地址，请选择分享链接使用的地址"
	ShareAddrNone     = "未能检测到公网地址，请手动输入。"
	ShareAddrChanged  = "分享地址已修改，所有节点的分享链接已变化，请重新导出。"
	ShareAddrDetected = "检测到公网地址："
)

// Modify node menu (DESIGN section 9.3).
const (
	ModPort         = "端口"
	ModTarget       = "目标网站 (SNI)"
	ModFallback     = "回落限速"
	ModRegenKeys    = "重新生成密钥"
	ModSharePort    = "分享端口"
	ModRemark       = "备注"
	LinksWillBreak  = "此操作会让该节点所有客户端的分享链接失效，是否继续？"
	NewLinksTitle   = "该节点所有客户端的新链接："
	KeysRegenerated = "已重新生成密钥。"
)

// ModifyTitle heads the modify menu.
func ModifyTitle(n int, remark, proto string) string {
	return fmt.Sprintf("修改节点 %d: %s (%s)", n, remark, proto)
}

// Fallback limit tiers (P-009, D-053).
const (
	TierDefault   = "默认"
	TierLoose     = "宽松"
	TierStrict    = "严格"
	TierCustom    = "自定义"
	TierOff       = "关闭"
	PromptAfterMB = "传输多少 MB 后开始限速"
	PromptRateKB  = "限速后的速率（KB/s）"
	PromptBurstKB = "突发速率（KB/s）"
)

// FallbackDescribe summarizes a fallback limit.
func FallbackDescribe(afterBytes, bytesPerSec int64) string {
	if afterBytes == 0 && bytesPerSec == 0 {
		return "关闭"
	}
	return fmt.Sprintf("约 %s 后限速 %s/s", humanBytes(afterBytes), humanBytes(bytesPerSec))
}

// FallbackTier labels a tier with its values.
func FallbackTier(name string, afterBytes, bytesPerSec, burst int64) string {
	return fmt.Sprintf("%s  (%s 后限速 %s/s，突发 %s/s)", name, humanBytes(afterBytes), humanBytes(bytesPerSec), humanBytes(burst))
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<20 && b%(1<<20) == 0:
		return fmt.Sprintf("%dMB", b>>20)
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%dKB", b>>10)
	}
}

// Enable / disable / delete.
const (
	NodeEnabled  = "节点已启用。"
	NodeDisabled = "节点已禁用。"
	NodeDeleted  = "节点已删除。"
)

// ConfirmDelete asks before deleting a node.
func ConfirmDelete(remark string, clients int) string {
	return fmt.Sprintf("删除节点 %s 及其 %d 个客户端？", remark, clients)
}

// Main menu Xray status.
const (
	XrayRunning       = "● 运行中"
	XrayStopped       = "○ 已停止"
	XrayStoppedByUser = "○ 已停止（手动）"
	XrayManualWarning = "Xray 已手动停止，节点当前不可用。在\"Xray 服务\"中启动。"
)

// Client management (DESIGN section 9.4).
const (
	ClientAdd          = "添加客户端"
	ClientModify       = "修改客户端"
	ClientModifyHint   = "(备注 / UUID)"
	ClientToggle       = "启用 / 禁用客户端"
	ClientLink         = "分享链接 / 二维码"
	ClientDelete       = "删除客户端"
	ClientRemark       = "备注"
	ClientResetUUID    = "重置 UUID"
	PromptClientNumber = "请输入客户端序号"
	ClientAdded        = "客户端已添加。"
	ClientEnabled      = "客户端已启用。"
	ClientDisabled     = "客户端已禁用。"
	ClientDeleted      = "客户端已删除。"
	ClientUUIDReset    = "UUID 已重置，该客户端的旧链接已失效，新链接如下。"
	ConfirmResetUUID   = "重置后该客户端的旧链接立即失效（其他客户端不受影响），是否继续？"
	LastClientDelete   = "节点至少需要一个客户端，是否同时删除节点？"
	ErrClientNotFound  = "客户端不存在。"
)

// ClientsTitle heads the client menu, e.g. "节点 1: LA (VLESS-TCP-Reality :443)".
func ClientsTitle(seq int, remark, proto string, port int) string {
	return fmt.Sprintf("节点 %d: %s (%s :%d)", seq, remark, proto, port)
}

// ConfirmDeleteClient asks before deleting a client.
func ConfirmDeleteClient(remark string) string {
	return fmt.Sprintf("删除客户端 %s？", remark)
}
