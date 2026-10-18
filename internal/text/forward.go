package text

import "fmt"

// Messages of "litenode internal resolve", run by litenode-resolve.timer.
// They end up in the journal.
const ResolveSkipped = "LiteNode 菜单正在运行，跳过本轮重新解析，下一轮再更新。"

// ResolveLookupFailed reports a lookup that failed; the rule keeps its
// addresses.
func ResolveLookupFailed(err error) string {
	return fmt.Sprintf("域名解析失败，对应的转发规则保持不变：%v", err)
}

// ResolveUpdated reports a forward whose target addresses changed.
func ResolveUpdated(id int64, host, v4, v6 string) string {
	return fmt.Sprintf("转发规则（ID %d）的目标 %s 地址已更新：IPv4 %s，IPv6 %s", id, host, orNone(v4), orNone(v6))
}

// ResolveFailed reports that the new rules could not be applied; the
// change flow restored the old ones.
func ResolveFailed(err error) string {
	return fmt.Sprintf("更新转发规则失败，已保持原来的规则：%v", err)
}

// ResolveTimerFailed reports that the resolve timer could not be set up
// or removed; the forwarding rules themselves were applied.
func ResolveTimerFailed(err error) string {
	return fmt.Sprintf("转发规则已更新，但设置定时重新解析（litenode-resolve.timer）失败：%v", err)
}

func orNone(s string) string {
	if s == "" {
		return "无"
	}
	return s
}

// Port forwarding menu (DESIGN section 11).
const (
	ForwardAdd          = "添加规则"
	ForwardModify       = "修改规则"
	ForwardToggle       = "启用 / 禁用规则"
	ForwardProbe        = "测试目标连通性"
	ForwardDelete       = "删除规则"
	ForwardNone         = "（暂无转发规则）"
	ColLocalPort        = "本机端口"
	ColTarget           = "目标"
	OnlyIPv4            = "仅 IPv4"
	OnlyIPv6            = "仅 IPv6"
	PromptForwardNumber = "请输入规则序号 (0 返回)"
	PromptListenPort    = "本机端口（单个端口或 起始-结束，0 返回）"
	PromptForwardProto  = "协议"
	PromptTargetHost    = "目标地址（IP 或域名，0 返回）"
	PromptTargetPort    = "目标端口"
	PromptForwardRemark = "备注（可以为空）"
	CurrentMark         = "(当前)"
	ForwardAdded        = "转发规则已添加。"
	ForwardUpdated      = "转发规则已修改。"
	ForwardEnabled      = "转发规则已启用。"
	ForwardDisabled     = "转发规则已禁用。"
	ForwardDeleted      = "转发规则已删除。"
	ForwardNoChange     = "没有修改。"

	ErrForwardNotFound   = "转发规则不存在，可能已被删除。"
	ErrTargetPortLength  = "目标端口段的长度必须与本机端口段相同（如本机 30000-30010 对应目标 40000-40010）。"
	ErrTargetHost        = "目标地址无效：请输入 IP 地址或域名。"
	ErrTargetNoAddresses = "目标域名没有解析到 IPv4 或 IPv6 地址，请检查域名是否正确、是否已生效。"

	Probing        = "正在测试……"
	ProbeUDPOnly   = "UDP 无法可靠测试连通性，请用客户端实际连接验证。"
	ProbeTCPOnly   = "只测试 TCP；UDP 无法可靠测试，请用客户端实际连接验证。"
	ProbeAllOK     = "目标可以连通。"
	ProbeSomeFails = "目标无法连通：请检查目标地址、端口，以及落地机的防火墙和安全组。"
)

// ProbeFirstPort notes that only the first port of a range is tested.
func ProbeFirstPort(port int) string {
	return fmt.Sprintf("端口段只测试第一个端口 %d。", port)
}

// ProbeOK reports a target address that accepted the connection.
func ProbeOK(addr string, ms int64) string {
	return fmt.Sprintf("%s  连通，延迟 %d ms", addr, ms)
}

// ProbeFailed reports a target address that could not be reached.
func ProbeFailed(addr string, err error) string {
	return fmt.Sprintf("%s  无法连接：%v", addr, err)
}

// ConfirmDeleteForward asks before deleting a forward.
func ConfirmDeleteForward(index int, rule string) string {
	return fmt.Sprintf("确定删除转发规则 %d（%s）？", index, rule)
}

// ForwardTimerFailed is shown after a change that took effect while the
// resolve timer could not be set up or removed (D-076).
func ForwardTimerFailed(err error) string {
	return fmt.Sprintf("⚠ 转发规则已生效，但设置定时重新解析（litenode-resolve.timer）失败：%v。域名目标的 IP 变化后不会自动更新；可以稍后修改或启用 / 禁用一次规则重试。", err)
}
