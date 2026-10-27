package text

import (
	"fmt"
	"strconv"
	"time"
)

// System optimization menu (DESIGN section 15).
const (
	SysoptBBR         = "BBR + fq"
	SysoptBBRByOther  = "BBR 已由系统的其他配置开启，无需操作。"
	ConfirmBBREnable  = "将开启 BBR 拥塞控制和 fq 队列（写入 /etc/sysctl.d/99-zz-litenode.conf，重启后保持）。确定开启？"
	ConfirmBBRDisable = "将关闭 BBR，恢复为系统其他 sysctl 配置中的设置（没有时为默认的 cubic）。确定关闭？"
	BBREnabledDone    = "BBR 已开启。"
	BBRDisabledDone   = "BBR 已关闭。"
	StateOn           = "✓"
	StateOff          = "✗"
)

// SysoptUnavailableIn names the environment that hides some options.
func SysoptUnavailableIn(virt string) string {
	return "部分功能在当前环境（" + virt + "）中不可用，已隐藏。"
}

// BBRKeptBy reports that LiteNode's BBR setting is removed but another
// sysctl file still turns BBR on.
func BBRKeptBy(file string) string {
	return "已删除 LiteNode 的 BBR 设置，但系统配置 " + file + " 仍开启 BBR，BBR 保持开启。如需关闭，请修改该文件。"
}

// StatusBBR labels the BBR field of the system optimization status.
func StatusBBR(v string) string { return "BBR: " + v }

// Swap (DESIGN section 15).
const (
	SysoptSwap        = "swap"
	SwapNone          = "无"
	SwapResize        = "调整大小"
	SwapDelete        = "删除"
	PromptSwapSize    = "swap 大小（如 512M、1G，0 返回）"
	ConfirmSwapDelete = "将停用并删除 swap 文件。确定删除？"
	SwapDeleted       = "swap 已删除。"
	ErrSwapExists     = "swap 文件已存在。"
	ErrNoSwap         = "没有 LiteNode 创建的 swap。"
	ErrSwapInUse      = "swap 中的数据超过当前可用内存，停用可能导致程序被系统结束。请先减少内存占用再试。"
	SwapCreating      = "正在创建 swap 文件，可能需要一些时间……"
)

// StatusSwap labels the swap field of the system optimization status.
func StatusSwap(v string) string { return "swap: " + v }

// SwapLine describes one active swap area.
func SwapLine(path, size string, usedMB int, own bool) string {
	l := "  " + path + "  " + size + "，已用 " + strconv.Itoa(usedMB) + "M"
	if !own {
		l += "（非 LiteNode 创建）"
	}
	return l
}

// SwapMemory shows the memory size and, when small, the suggested swap.
func SwapMemory(mem, suggest string) string {
	if suggest == "" {
		return "内存 " + mem + "。"
	}
	return "内存 " + mem + "，建议创建 " + suggest + " swap。"
}

// SwapCreated reports a new swap file.
func SwapCreated(size string) string { return "已创建 " + size + " swap，重启后保持。" }

// SwapResized reports a resized swap file.
func SwapResized(size string) string { return "swap 已调整为 " + size + "。" }

// ErrSwapSize explains the allowed swap sizes.
func ErrSwapSize(min, max string) string { return "大小须在 " + min + " 到 " + max + " 之间。" }

// SwapMax shows the largest swap the disk can hold.
func SwapMax(max string) string {
	return "磁盘空间最多可创建 " + max + "（保留 256M 空闲空间）。"
}

// SwapResizeNote explains why resizing allows less than creating: the
// new file is made before the old one is removed.
const SwapResizeNote = "调整大小时先创建新文件再删除旧文件，需要更大的 swap 时可以先删除再创建。"

// ErrNoDiskSpace reports that the disk cannot hold the swap file.
const ErrNoDiskSpace = "磁盘空间不足（创建后需保留 256M 空闲空间）。"

// IP preference (DESIGN section 15).
const (
	SysoptPreference  = "系统 IPv4 / IPv6 优先级"
	PreferIPv4        = "优先 IPv4"
	PreferIPv6        = "优先 IPv6（系统默认）"
	PreferenceHint    = "影响系统程序（curl、apt 等）解析域名后的出站顺序，写入 /etc/gai.conf。不影响节点出站，节点出站在\"Xray 服务\"的\"节点出站 IP\"中设置。"
	PreferIPv4Done    = "已设置为优先 IPv4。"
	PreferIPv6Done    = "已恢复为优先 IPv6（系统默认）。"
	PreferenceForeign = "/etc/gai.conf 中有其他行设置了优先 IPv4（非 LiteNode 添加），需手动修改该文件才能优先 IPv6。"
)

// StatusPreference labels the preference field of the status.
func StatusPreference(ipv4 bool) string {
	if ipv4 {
		return "优先: IPv4"
	}
	return "优先: IPv6"
}

// DNS (DESIGN section 15).
const (
	SysoptDNS        = "DNS"
	DNSCustom        = "自定义"
	DNSReset         = "恢复系统默认"
	PromptDNSServers = "DNS 服务器 IP（最多 3 个，空格分隔，0 返回）"
	ErrDNSServers    = "请输入 1 到 3 个 IP 地址，用空格分隔。"
	ConfirmDNSFailed = "仍要使用这些 DNS 服务器？"
	DNSResetDone     = "已恢复系统默认 DNS。"
	DNSVerifyFailed  = "⚠ 设置后解析测试失败，请检查网络或换用其他 DNS。"
	DNSNone          = "无"
	DNSByLiteNode    = "LiteNode 设置"
	DNSBySystem      = "系统配置"
)

// DNSCurrent shows the DNS servers in use and where they come from.
func DNSCurrent(servers, source string, resolved bool) string {
	via := "/etc/resolv.conf"
	if resolved {
		via = "systemd-resolved"
	}
	return "当前 DNS：" + servers + "（" + source + "，" + via + "）"
}

// DNSPresetLabel is a preset choice.
func DNSPresetLabel(name, servers string) string { return name + "  " + servers }

// DNSProbeFailed reports a DNS server that did not answer; reason comes
// from NetReason.
func DNSProbeFailed(server, reason string) string {
	return "⚠ " + server + " 无法解析域名：" + reason
}

// DNSUnreachable marks a current DNS server that did not answer.
func DNSUnreachable(server string) string { return server + "（不可达）" }

// DNSUnreachableHint follows a current server list with unreachable
// servers.
const DNSUnreachableHint = "标为不可达的 DNS 服务器连不上，会拖慢域名解析，建议更换。"

// DNSSlowFirst explains at startup that every lookup waits for an
// unreachable IPv4 DNS server (D-138).
func DNSSlowFirst(server string) string {
	return fmt.Sprintf("⚠ 系统 DNS 的第一个服务器 %s 是 IPv4 地址，本机没有 IPv4 出口，访问不到它：每次解析域名都要先等它超时，添加节点、下载等操作会等待很久。", server)
}

// ConfirmDNSFix offers the IPv6 DNS servers of a preset (D-138).
func ConfirmDNSFix(name, servers string) string {
	return fmt.Sprintf("是否改为 %s 的 IPv6 DNS（%s）？以后可在 系统优化 → DNS 修改", name, servers)
}

// DNSFixDeclined tells where to change DNS later; LiteNode does not ask
// again (D-138).
const DNSFixDeclined = "未修改，以后不再询问。可在 系统优化 → DNS 修改。"

// DNSSet reports the new DNS servers.
func DNSSet(servers string) string { return "DNS 已设置为 " + servers + "。" }

// Time zone and time synchronization (DESIGN section 15).
const (
	SysoptClock       = "时区与时间同步"
	ClockSetTimezone  = "设置时区"
	ClockEnableNTP    = "开启时间同步"
	TimezoneCustom    = "其他（手动输入）"
	PromptTimezone    = "时区（如 Asia/Shanghai，0 返回）"
	ErrTimezone       = "系统中没有这个时区，请按 地区/城市 格式输入，如 Asia/Shanghai。"
	NTPWaiting        = "正在等待时间同步（最多 30 秒）……"
	NTPSyncedDone     = "时间同步已开启，时钟已同步。"
	NTPPendingDone    = "时间同步已开启，尚未完成同步，请稍后查看状态。"
	NTPAlreadySynced  = "时间同步已开启，时钟已同步。"
	NTPInstallingNote = "系统没有时间同步服务，将安装 systemd-timesyncd。"
)

// StatusClock labels the time zone field; synced is shown only where
// the system can synchronize time.
func StatusClock(tz, synced string) string {
	if synced == "" {
		return "时区: " + tz
	}
	return "时区: " + tz + "（同步 " + synced + "）"
}

// ClockCurrent describes the time zone and synchronization state.
func ClockCurrent(tz string, ntp, synced bool) string {
	sync := "未开启"
	switch {
	case synced:
		sync = "已同步"
	case ntp:
		sync = "已开启，尚未同步"
	}
	return "当前时区 " + tz + "，时间同步：" + sync + "。Reality 对时间敏感，建议保持同步。"
}

// TimezoneSet reports the new time zone.
func TimezoneSet(tz string) string { return "时区已设置为 " + tz + "。" }

// WARP (DESIGN section 19.4).
const (
	SysoptWARP         = "WARP"
	WARPEnable         = "开启 WARP"
	WARPReselect       = "重新选择端点"
	WARPDisable        = "关闭 WARP"
	WARPOff            = "WARP: 未开启。可为只有 IPv6（或只有 IPv4）的机器补上缺少的出口。"
	WARPNoExit         = "本机 IPv4 和 IPv6 都没有出口，无法向 Cloudflare 注册 WARP。请先检查网络。"
	WARPPreparing      = "正在准备（缺少时安装 wireguard-tools）并向 Cloudflare 注册……"
	WARPNoKernel       = "当前内核不支持 WireGuard（容器环境或内核缺少模块），无法开启 WARP。"
	ConfirmWARPDisable = "关闭后该协议将没有出口，依赖它的节点出站和下载会失败。确定关闭 WARP？"
	WARPDisabled       = "WARP 已关闭，配置已删除。"
	WARPNoHandshake    = "尚未握手"
	WARPInactive       = "服务未运行"
)

// Messages of "litenode internal warp-check", run by
// litenode-warp-check.timer.
const WARPCheckSkipped = "LiteNode 菜单正在运行，跳过本轮 WARP 检查。"

// WARPCheckFailed reports a failed check; the tunnel keeps its endpoint.
func WARPCheckFailed(err error) string {
	return fmt.Sprintf("WARP 检查失败，下一轮再试：%v", err)
}

// StatusWARP labels the WARP field of the system optimization status.
func StatusWARP(v string) string { return "WARP: " + v }

// WARPNone is the WARP field when it is off.
const WARPNone = "无"

// familyLabel is "IPv4" or "IPv6".
func familyLabel(ipv6 bool) string {
	if ipv6 {
		return "IPv6"
	}
	return "IPv4"
}

// WARPNotNeeded explains that both exits exist; byWARP names a family
// that already goes through a WARP LiteNode did not set up.
func WARPNotNeeded(byWARP string) string {
	if byWARP != "" {
		return byWARP + " 出口已经是 WARP（不是 LiteNode 开启的），另一个协议也有出口，不需要再开启。"
	}
	return "本机 IPv4 和 IPv6 都有出口，不需要 WARP。"
}

// ConfirmWARPEnable explains what enabling does for the missing family.
func ConfirmWARPEnable(ipv6 bool) string {
	f := familyLabel(ipv6)
	return "将安装 wireguard-tools 并向 Cloudflare 注册一个免费 WARP 账号，只让 " + f + " 出站流量经 WARP；" +
		"另一个协议、入站连接和 SSH 不受影响。WARP 出口 IP 属于 Cloudflare，不会用作分享地址。确定开启？"
}

// WARPTrying shows the endpoint being tried.
func WARPTrying(ep string) string { return "尝试端点 " + ep + " ……" }

// WARPEnabled confirms the tunnel and its exit address.
func WARPEnabled(ipv6 bool, exit string) string {
	if exit == "" {
		return "WARP 已开启，" + familyLabel(ipv6) + " 出站经 WARP。"
	}
	return "WARP 已开启，" + familyLabel(ipv6) + " 出站经 WARP，出口 " + exit + "。"
}

// WARPNoEndpoint explains that no endpoint worked; nothing was kept.
func WARPNoEndpoint(err error) string {
	return fmt.Sprintf("所有 WARP 端点都无法连通，已撤销，未保留任何配置：%v\n可能是商家屏蔽了 WireGuard（UDP），可稍后重试。", err)
}

// WARPReselected confirms the new endpoint.
func WARPReselected(ep string) string { return "已改用端点 " + ep + "。" }

// WARPOn is the status line of an enabled tunnel.
func WARPOn(ipv6 bool, exit, endpoint, handshake string) string {
	s := "WARP: 已开启（" + familyLabel(ipv6) + " 经 WARP"
	if exit != "" {
		s += "，出口 " + exit
	}
	return s + "，端点 " + endpoint + "，" + handshake + "）"
}

// WARPHandshakeAgo formats the time since the latest handshake.
func WARPHandshakeAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("最近握手 %d 秒前", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("最近握手 %d 分钟前", int(d.Minutes()))
	}
	return fmt.Sprintf("最近握手 %d 小时前", int(d.Hours()))
}

// DNSResolvConfNote says which files a DNS change writes when the system
// does not use systemd-resolved; with openresolv the servers stay first
// after a reboot (D-139).
func DNSResolvConfNote(openresolv bool) string {
	if openresolv {
		return "系统未使用 systemd-resolved，将修改 /etc/resolv.conf，并在 /etc/resolvconf.conf 中把这些服务器排在最前，重启后仍然有效。"
	}
	return "系统未使用 systemd-resolved，将直接修改 /etc/resolv.conf。该文件可能被 DHCP 或 cloud-init 改回，届时请重新设置。"
}
