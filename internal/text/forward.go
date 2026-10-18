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
