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
	NodeDelete         = "删除节点"
	NodeNone           = "（暂无节点）"
	ColID              = "ID"
	ColRemark          = "备注"
	ColProtocol        = "协议"
	ColPort            = "端口"
	ColClients         = "客户端"
	ColState           = "状态"
	ColNode            = "节点"
	StateEnabled       = "● 启用"
	StateDisabled      = "○ 禁用"
	NotSet             = "未设置"
	PromptNodeNumber   = "请输入节点序号（0 返回）"
	PromptPort         = "端口"
	PromptRemark       = "备注"
	PromptTarget       = "目标网站域名（可带端口，默认 443；0 返回）"
	PromptShareAddress = "主机地址（IP 或域名，0 返回）"
	PromptSharePort    = "分享端口（0 表示与监听端口相同）"
	Checking           = "正在检测……"
	InstallingXray     = "首次添加节点，正在安装 Xray……"
	ApplyingChange     = "正在应用配置……"
	Done               = "完成。"
)

// ShareAddressLine is the status line of the share menu: the default
// address and how many others there are.
func ShareAddressLine(def string, n int) string {
	if def == "" {
		return "主机地址: " + NotSet
	}
	if n > 1 {
		return fmt.Sprintf("主机地址: %s（另有 %d 个）", def, n-1)
	}
	return "主机地址: " + def
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
const ChoosePreset = "节点类型"

// Target selection (DESIGN section 10.1).
const (
	ChooseTarget     = "目标网站 (SNI)"
	TargetCommon     = "从常用列表选择"
	TargetManual     = "手动输入"
	TargetNoneUsable = "常用列表中没有通过检测的网站，请选择其他方式。"
	SlowDNS          = "域名解析慢，建议在 系统优化 → DNS 中更换。"
	CommonHeading    = "常用目标网站"
	ScanHeading      = "附近的网站"
	CommonListHint   = "提示：常用列表中的大型网站被很多节点使用，且与本机 IP 不在同一网段，扫描附近网站得到的目标更不容易被识别。"
	TargetScan       = "扫描附近网站"
	TargetScanHint   = "（推荐，约 1 分钟）"
	ScanWarning      = "扫描会连接本机所在 /24 网段其他服务器的 443 端口。部分 VPS 商家禁止端口扫描，可能导致滥用投诉。是否继续？"
	ScanNoAddress    = "无法确定本机的公网 IPv4 地址，不能扫描。"
	ScanIPv6Only     = "纯 IPv6 机器不能扫描附近网站，请从常用列表选择或手动输入。"
	ScanNoResult     = "附近没有找到可用的网站，请选择其他方式。"
	ScanFound        = "找到以下可用网站（按延迟排序）："
	TargetNotUsable  = "该网站未通过检测，不能使用。"
	TargetUseAnyway  = "仍要使用这个网站吗？"
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

// ScanProgress shows how far the scan is.
func ScanProgress(done, total int) string {
	return fmt.Sprintf("正在扫描 %d/%d……", done, total)
}

// ScanStart announces the scanned range.
func ScanStart(prefix string) string {
	return fmt.Sprintf("正在扫描 %s 的 443 端口……", prefix)
}

// MLKEMUnsupported explains a target that negotiates ML-KEM with an Xray
// that cannot handle it (D-054).
func MLKEMUnsupported(min string) string {
	return fmt.Sprintf("该网站使用 X25519MLKEM768 密钥交换，需要 Xray %s 或更高版本才能作为 Reality 目标。", min)
}

// CheckUnreachable is shown when a target cannot be reached; reason
// comes from NetReason.
func CheckUnreachable(host, reason string) string {
	return host + "  无法连接：" + reason
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
		return "首页: 没有返回网页（不像真实网站，不能作为目标）"
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
	ErrNodeNotFound    = "节点不存在，可能已被删除。"
	ErrInvalidAddress  = "请输入有效的 IP 地址或域名。"
)

// ErrRemarkTooLong is shown for an over-long remark.
func ErrRemarkTooLong(max int) string {
	return fmt.Sprintf("备注最多 %d 个字符。", max)
}

// XrayInstallFailed explains a failed Xray download (D-042); noIPv4 means
// the host has no IPv4 exit and no NAT64 (DESIGN section 19.3).
func XrayInstallFailed(err error, noIPv4 bool) string {
	return fmt.Sprintf("安装 Xray 失败：%v\n%s", err, noGitHubHint(noIPv4))
}

// XrayDownloadFailed explains a failed Xray update download (D-042).
func XrayDownloadFailed(err error, noIPv4 bool) string {
	return fmt.Sprintf("下载 Xray 失败，未做任何修改：%v\n%s", err, noGitHubHint(noIPv4))
}

func noGitHubHint(noIPv4 bool) string {
	if noIPv4 {
		return NoIPv4Download
	}
	return "如果这台机器无法访问 GitHub，请参考 " + NoGitHubURL
}

// NoIPv4Download explains why GitHub cannot be reached from an IPv6-only
// host without NAT64 (DESIGN section 19.3).
const NoIPv4Download = "本机没有 IPv4 出口，也没有检测到商家的 NAT64，而 GitHub 只有 IPv4 地址。\n可在 系统优化 → WARP 开启 IPv4 出口后重试，或参考 " + NoGitHubURL

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
	SecurityGroupHint = "连不上时，请检查云厂商的安全组或防火墙是否放行了 %s 端口 %d。"
)

// VLESSEncClientHint follows a VLESS Encryption link (DESIGN section 9.7).
const VLESSEncClientHint = "需要客户端支持 VLESS Encryption（支持的客户端见 README）。"

// SecurityGroup formats the security group hint for the node's port
// protocol: "tcp", "udp" or "tcp+udp" (node.PortProto).
func SecurityGroup(port int, proto string) string {
	label := map[string]string{"tcp": "TCP", "udp": "UDP", "tcp+udp": "TCP 和 UDP"}[proto]
	return fmt.Sprintf(SecurityGroupHint, label, port)
}

// ForeignFilter warns about existing inbound filtering (D-044).
func ForeignFilter(port int, where []string) string {
	return fmt.Sprintf("⚠ 检测到系统已有入站过滤规则（%s），可能拦截端口 %d。可以手动放行，或启用 LiteNode 防火墙统一管理。", strings.Join(where, "、"), port)
}

// Share address.
const (
	ShareAddrAdd       = "添加地址"
	ShareAddrDefault   = "设为默认"
	ShareAddrDelete    = "删除地址"
	ShareAddrDetect    = "重新检测"
	ShareAddrNone      = "未能检测到公网地址，请手动输入。"
	ShareAddrDefaultOK = "默认主机地址已修改，新添加节点的链接和\"修改节点\"后列出的链接使用这个地址。"
	ShareAddrAdded     = "地址已添加。"
	ShareAddrDeleted   = "地址已删除。"
	ShareAddrExists    = "这个地址已经在列表中。"
	ShareAddrsNone     = "（暂无，生成链接时自动检测）"
	ShareAddrDetected  = "检测到公网地址："
	PromptAddressNum   = "请输入地址序号（0 返回）"
	LinkAddressHeading = "选择链接使用的地址"
	OtherAddressesHint = "其他主机地址（如 IPv6、域名）的链接：主菜单 → 分享节点。"
	KindDomain         = "域名"
	DefaultMark        = "默认"
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
	// ModRegenCert replaces ModRegenKeys on Hysteria2 nodes (DESIGN
	// section 9.10).
	ModRegenCert     = "重新生成证书"
	ConfirmRegenCert = "重新生成证书会更换证书指纹：线路机需要改用新的指纹，所有客户端的旧链接失效。是否继续？"
	CertRegenerated  = "已重新生成证书，请在线路机上更新证书指纹。"
	SharePortSet     = "分享端口已修改。"
)

// PortSet ends a successful port change.
func PortSet(port int) string { return fmt.Sprintf("端口已改为 %d。", port) }

// TargetSet ends a successful target change.
func TargetSet(target string) string { return "目标网站已改为 " + target + "。" }

// ModifyTitle heads the modify menu.
func ModifyTitle(n int, remark, proto string) string {
	return fmt.Sprintf("节点 %d: %s (%s)", n, remark, proto)
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
	return fmt.Sprintf("%s  （%s 后限速 %s/s，突发 %s/s）", name, humanBytes(afterBytes), humanBytes(bytesPerSec), humanBytes(burst))
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
	ClientToggle       = "启用 / 禁用客户端"
	ClientDelete       = "删除客户端"
	ClientRemark       = "备注"
	ClientResetUUID    = "重置 UUID"
	ClientResetKey     = "重置密钥"
	PromptClientNumber = "请输入客户端序号（0 返回）"
	ClientAdded        = "客户端已添加。"
	ClientEnabled      = "客户端已启用。"
	ClientDisabled     = "客户端已禁用。"
	ClientDeleted      = "客户端已删除。"
	ClientUUIDReset    = "UUID 已重置，该客户端的旧链接已失效。"
	ClientKeyReset     = "密钥已重置，该客户端的旧链接已失效。"
	ClientResetPass    = "重置密码"
	ClientPassReset    = "密码已重置，该客户端的旧链接已失效。"
	ConfirmResetUUID   = "重置后该客户端的旧链接立即失效（其他客户端不受影响），是否继续？"
	ClientNone         = "（暂无客户端）"
	ClientNoNodes      = "（暂无节点，请先在\"节点管理\"中添加节点）"
	ChooseNodeHeading  = "选择节点"
	ErrClientNotFound  = "客户端不存在，可能已被删除。"
)

// NodeChoice labels a node when choosing one, e.g. "LA (VLESS-TCP-Reality :443)".
func NodeChoice(remark, proto string, port int) string {
	return fmt.Sprintf("%s (%s :%d)", remark, proto, port)
}

// NodeWithoutClients warns that a node has no client and cannot be used
// until one is added (D-060).
func NodeWithoutClients(remark string) string {
	return fmt.Sprintf("节点 %s 没有客户端，当前不可用；添加客户端后即可使用。", remark)
}

// ConfirmDeleteClient asks before deleting a client.
func ConfirmDeleteClient(remark string) string {
	return fmt.Sprintf("删除客户端 %s？", remark)
}

// ConfirmDeleteLastClient asks before deleting a node's last client; the
// node stays but cannot be used until a client is added (D-060).
func ConfirmDeleteLastClient(remark string) string {
	return fmt.Sprintf("%s 是该节点最后一个客户端，删除后节点暂时不可用，直到添加新客户端。是否继续？", remark)
}

// Share menu (DESIGN section 9.5).
const (
	ShareLinkQR    = "导出单个链接"
	ShareAllLinks  = "导出全部链接"
	ShareClientOff = "该客户端或其节点已禁用，链接暂时不能使用。"
	ShareAllNone   = "没有启用的客户端。"
	AllLinksTitle  = "全部链接："
)

// ClientLine names the client being modified.
func ClientLine(remark string) string {
	return "客户端: " + remark
}

// XrayLogsTitle heads the recent log lines.
func XrayLogsTitle(n int) string { return fmt.Sprintf("最近 %d 行日志：", n) }

// Xray service menu (DESIGN section 12).
const (
	XrayStart            = "启动"
	XrayStop             = "停止"
	XrayRestart          = "重启"
	XrayLogs             = "查看日志"
	XrayUpdate           = "更新 Xray"
	XrayUninstall        = "卸载 Xray"
	XrayNotInstalledNote = "Xray 未安装。"
	XrayMissingNote      = "Xray 程序不存在（可能被手动删除），节点当前无法运行。"
	XrayInstall          = "安装 Xray"
	XrayInstallingNow    = "正在安装 Xray……"
	XrayReinstalled      = "Xray 已安装，节点配置已应用。"
	XrayInstalledNoNodes = "Xray 已安装。没有启用的节点时不运行，添加节点后自动启动。"
	ErrXrayMissing       = "Xray 程序不存在，无法应用需要运行的节点。请先在\"Xray 服务\"中选择\"安装 Xray\"。"
	XrayStarted          = "Xray 已启动。"
	XrayStoppedMsg       = "Xray 已停止。在添加或修改节点后也会保持停止，直到在这里启动。"
	XrayRestarted        = "Xray 已重启。"
	XrayNoNodes          = "没有启用的节点，Xray 无需运行。"
	XrayStoppedByUserErr = "Xray 已被手动停止，请先选择\"启动\"。"
	XrayAlreadyRunning   = "Xray 已在运行，没有重启。需要重启请选择\"重启\"。"
	XrayAlreadyStopped   = "Xray 已经是停止状态。"
	XrayRecentLogs       = "最近日志"
	XrayLiveLogs         = "实时日志"
	PromptLogLines       = "显示行数（最多 1000，0 返回）"
	XrayLiveLogsTitle    = "实时日志（按 Ctrl+C 回到菜单）："
	PromptXrayVersion    = "要安装的版本（直接回车为默认版本，0 返回）"
	XrayUpdating         = "正在下载并校验……"
	XrayUninstalled      = "Xray 已卸载，所有节点和客户端已删除。"
)

// Node outbound IP (DESIGN section 12, D-084).
const (
	XrayOutboundIP        = "节点出站 IP"
	OutboundIPDefault     = "默认（由系统决定）"
	OutboundIPPreferIPv4  = "优先 IPv4"
	OutboundIPPreferIPv6  = "优先 IPv6"
	OutboundIPIPv4Only    = "仅 IPv4"
	OutboundIPIPv6Only    = "仅 IPv6"
	OutboundIPNote        = "设置节点访问目标网站时使用的 IP 协议，只影响 Xray，不影响本机 curl、apt 等程序（它们在\"系统优化\"中设置）。"
	OutboundIPOnlyWarning = "选择\"仅\"时，没有该协议地址的目标网站将无法访问。"
)

// OutboundIPLabels are the mode names in xray.OutboundModes order.
var OutboundIPLabels = []string{OutboundIPDefault, OutboundIPPreferIPv4, OutboundIPPreferIPv6, OutboundIPIPv4Only, OutboundIPIPv6Only}

// StatusOutboundIP is the outbound field of the Xray menu status.
func StatusOutboundIP(label string) string { return "节点出站: " + label }

// OutboundIPDone reports the new mode.
func OutboundIPDone(label string) string { return "节点出站 IP 已设置为" + label + "。" }

// OutboundIPUnavailable explains a refused "only" mode.
func OutboundIPUnavailable(ipv6 bool) string {
	fam := "IPv4"
	if ipv6 {
		fam = "IPv6"
	}
	return fmt.Sprintf("本机没有 %s 出口，不能设置为仅 %s。可在 系统优化 → WARP 补上缺少的出口。", fam, fam)
}

// ConfirmOutboundNoExit asks before preferring a family without an exit.
func ConfirmOutboundNoExit(ipv6 bool) string {
	fam := "IPv4"
	if ipv6 {
		fam = "IPv6"
	}
	return fmt.Sprintf("本机没有 %s 出口，优先 %s 时，有 %s 地址的网站可能无法经节点访问。确定设置？", fam, fam, fam)
}

// XrayStatusLine is the status of the Xray menu, e.g.
// "Xray v26.10.17  ● 运行中  已运行 3天2小时  内存 28M".
func XrayStatusLine(version, state string, uptimeSecs int64, memory int64) string {
	line := fmt.Sprintf("Xray %s  %s", version, state)
	if uptimeSecs > 0 {
		line += "  已运行 " + duration(uptimeSecs)
	}
	if memory > 0 {
		line += fmt.Sprintf("  内存 %dM", memory>>20)
	}
	return line
}

func duration(s int64) string {
	d, h, m := s/86400, s%86400/3600, s%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("%d天%d小时", d, h)
	case h > 0:
		return fmt.Sprintf("%d小时%d分钟", h, m)
	case m > 0:
		return fmt.Sprintf("%d分钟", m)
	}
	return fmt.Sprintf("%d秒", s)
}

// XrayUpdated reports a successful update.
func XrayUpdated(v string) string { return "Xray 已更新到 " + v + "。" }

// VLESSEncHint follows the choice of a VLESS Encryption node (DESIGN
// section 9.7).
const VLESSEncHint = "适合经中转连接的落地机；需要客户端支持 VLESS Encryption。"

// SS2022Hint follows the choice of a Shadowsocks 2022 node (DESIGN
// section 9.8).
const SS2022Hint = "兼容选项：客户端支持时优先使用 VLESS Encryption。"

// RelayParams lists a Hysteria2 client's outbound settings for the relay
// below its link (DESIGN section 9.10, D-120).
func RelayParams(host string, port int, password, sni, certHex, pubB64 string) string {
	return fmt.Sprintf("线路机出站参数（3x-ui 可以直接导入上面的链接）：\n"+
		"  地址：%s\n  端口：%d\n  密码：%s\n  SNI：%s\n%s"+
		"  不要开启跳过证书验证（insecure / skip-cert-verify），用证书指纹固定证书。",
		host, port, password, sni, Fingerprints(certHex, pubB64))
}

// Fingerprints are a Hysteria2 node's pinned hashes with the client
// fields they go into.
func Fingerprints(certHex, pubB64 string) string {
	return fmt.Sprintf("  证书 SHA-256：%s\n    （Xray / 3x-ui pinnedPeerCertSha256、mihomo fingerprint）\n"+
		"  公钥 SHA-256：%s\n    （sing-box certificate_public_key_sha256）\n", certHex, pubB64)
}

// NewFingerprintsTitle heads the fingerprints after the certificate was
// regenerated.
const NewFingerprintsTitle = "新的证书指纹（请更新线路机的出站配置）："

// Hy2Hint follows the choice of a Hysteria2 node (DESIGN section 9.10).
const Hy2Hint = "适合线路机出站连接落地机：使用自签证书，线路机按证书指纹连接；只使用 UDP 端口。"

// FeatureVLESSEnc and FeatureHysteria name node types that need a minimum
// Xray version.
const (
	FeatureVLESSEnc = "VLESS Encryption"
	FeatureHysteria = "Hysteria2"
)

// XrayTooOldFor stops adding a node of a type (FeatureVLESSEnc, ...) that
// an old Xray does not support.
func XrayTooOldFor(feature, v, min string) string {
	return fmt.Sprintf("当前 Xray %s 低于 %s，不支持 %s。请先在\"Xray 服务\"中选择\"更新 Xray\"。", v, min, feature)
}

// XrayOldVersionFor warns that existing nodes of a type need a newer Xray.
func XrayOldVersionFor(feature, v, min string) string {
	return fmt.Sprintf("⚠ %s 低于 %s，不支持 %s 节点：配置检查会失败，Xray 保持当前版本。", v, min, feature)
}

// XrayVersionUnknown warns that the Xray version to install could not be
// queried before checking REALITY targets (D-130).
func XrayVersionUnknown(err error, min string) string {
	return fmt.Sprintf("⚠ 查询 Xray 最新版本失败：%v\n  使用 X25519MLKEM768 的目标网站暂按可用显示；安装 Xray 时需要能访问 GitHub，并且版本不低于 %s。", err, min)
}

// XrayOldVersion warns about a version without ML-KEM target support.
func XrayOldVersion(v, min string) string {
	return fmt.Sprintf("⚠ %s 低于 %s，使用 X25519MLKEM768 的目标网站（多数大型网站）将无法使用。", v, min)
}

// ConfirmUninstallXray lists what uninstalling removes.
func ConfirmUninstallXray(nodes, clients int) string {
	return fmt.Sprintf("将删除 Xray 以及 %d 个节点、%d 个客户端，端口转发、SSH 和防火墙的其他设置不变。需要保留节点可以先在\"维护\"中导出配置。确定卸载？", nodes, clients)
}

// ShareAddrItem lists one share address in the share address page.
func ShareAddrItem(n int, addr, kind string, isDefault bool) string {
	if kind == "" {
		kind = KindDomain
	}
	s := fmt.Sprintf("  [%d] %s  %s", n, addr, kind)
	if isDefault {
		s += "  " + DefaultMark
	}
	return s
}

// ShareAddrsDetected reports the detected addresses and the default.
func ShareAddrsDetected(list []string) string {
	return ShareAddrDetected + strings.Join(list, "、") + "（默认 " + list[0] + "）"
}

// DomainNotHere warns about a domain that does not resolve to this
// server.
func DomainNotHere(domain string, resolved []string) string {
	return fmt.Sprintf("⚠ %s 解析到 %s，不是本机的地址（可能经过 CDN 或解析未生效），Reality 节点无法通过它连接。", domain, strings.Join(resolved, "、"))
}

// DomainUnresolved warns about a domain that does not resolve.
func DomainUnresolved(domain string) string {
	return fmt.Sprintf("⚠ 无法解析 %s（可能还没有生效）。", domain)
}

// ConfirmAddAnyway asks whether to keep a doubtful domain.
const ConfirmAddAnyway = "仍要添加吗？"

// Node connectivity check (DESIGN section 9.9).
const (
	NodeCheck           = "检测节点连通性"
	NodeCheckDisabled   = "节点已禁用，请先启用节点再检测。"
	NodeCheckNoClient   = "节点没有启用的客户端，客户端无法连接。请先在“客户端管理”中添加或启用客户端。"
	NodeCheckXrayDown   = "Xray 没有运行，请先在“Xray 服务”中启动。"
	ConfirmNodeCheckNft = "检测需要 nftables，目前未安装。现在通过 apt 安装？"
	NodeCheckIntro      = "请现在用客户端连接这个节点（直连或经线路机都可以）并打开几个网站。\nLiteNode 只在本机统计到达的数据包，不会主动发送探测。"
	NodeCheckPressEnter = "按回车结束检测"
	NodeCheckCanceled   = "检测已取消。"
	NodeCheckFinished   = "检测结束。"
	NodeCheckTimedOut   = "已达到 30 分钟上限，按当前计数给出结论。"
	NodeCheckConclusion = "检测结论："
	NodeCheckHintUDP    = "TCP 已连通，但还没有收到 UDP 包：请在客户端开启 UDP，并访问支持 HTTP/3 的网站（如 www.google.com）。"
	NodeCheckHintNone   = "还没有收到任何数据包：请核对客户端中的地址和端口；经线路机时核对线路机的转发目标。"
	NodeCheckOpenNote   = "    本机这一侧是通的。如果客户端仍然连不上，请检查客户端配置（链接、密钥、UDP 设置）。"
	NodeCheckScanners   = "  （来源中可能包含扫描器；经线路机连接时，来源是线路机的 IP）"
	NodeCheckUFW        = "    被 ufw 拦截：请用 ufw status 检查该端口。LiteNode 添加的规则带 litenode: 备注，缺少时重新打开 LiteNode 或禁用再启用节点会重新同步；同时检查有没有拦截该端口的 deny 规则。"
	NodeCheckFirewall   = "    被 LiteNode 防火墙拦截：请在“防火墙管理 → 检查并修复规则”中修复。"
	NodeCheckUDPCauses  = "    TCP 有数据包而 UDP 没有，可能的原因：\n" +
		"    - 云厂商安全组没有放行 UDP；\n" +
		"    - 线路机的转发不含 UDP，或使用了 UDP over TCP；\n" +
		"    - 线路机使用 Vision 流控时，Xray 默认拒绝 UDP 443（QUIC）；\n" +
		"    - 客户端没有开启 UDP，或检测期间没有 UDP 流量。"
)

// NodeCheckStatus names the states of one protocol.
var NodeCheckStatus = []string{"未到达", "检测中", "连通", "本机拦截"}

// NodeCheckHeading names the node being checked.
func NodeCheckHeading(remark string, port int) string {
	return fmt.Sprintf("检测节点 %s（端口 %d）", remark, port)
}

// NodeCheckRow is one protocol's live counts.
func NodeCheckRow(proto string, arrived, passed uint64, status string) string {
	return fmt.Sprintf("  %-3s  到达 %-6d 通过 %-6d %s", strings.ToUpper(proto), arrived, passed, status)
}

// NodeCheckElapsed is the time since the check started.
func NodeCheckElapsed(sec int) string {
	return fmt.Sprintf("已用时 %d:%02d", sec/60, sec%60)
}

// NodeCheckResult starts one protocol's conclusion.
func NodeCheckResult(proto, status string, arrived, passed uint64) string {
	return fmt.Sprintf("  %s：%s（到达 %d 个包，通过 %d 个）", strings.ToUpper(proto), status, arrived, passed)
}

// NodeCheckSources lists recorded source addresses.
func NodeCheckSources(addrs []string) string {
	return "    来源：" + strings.Join(addrs, "、")
}

// NodeCheckOther: neither ufw nor the LiteNode firewall is on.
func NodeCheckOther(where []string) string {
	if len(where) == 0 {
		return "    被系统中其他入站过滤规则拦截：请检查 iptables / nftables 中放行该端口的规则。"
	}
	return "    被系统中其他入站过滤规则拦截（" + strings.Join(where, "、") + "）：请检查这些规则是否放行该端口。"
}

// NodeCheckUnreached: no packet of proto arrived.
func NodeCheckUnreached(proto string, port int) string {
	return fmt.Sprintf("    没有收到数据包，请检查：\n"+
		"    - 云厂商安全组 / 运营商防火墙是否放行 %s %d；\n"+
		"    - 客户端中的地址和端口；\n"+
		"    - 经线路机时，线路机的转发是否指向本机的 %d 端口。", strings.ToUpper(proto), port, port)
}
