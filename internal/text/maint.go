package text

import "fmt"

// Maintenance menu (DESIGN section 16).
const (
	MaintExport = "导出配置"
	MaintImport = "导入配置"
	MaintUpdate = "更新脚本"
	MaintRemove = "卸载脚本"
	MaintPurge  = "全部卸载"

	PromptExportPath = "导出文件路径（0 返回）"
	PromptImportPath = "导入文件路径（0 返回）"
	ExportNote       = "导出文件不包含 SSH 设置、系统优化设置、主机地址和节点出站 IP。"
	ExportSecret     = "文件包含节点私钥和客户端 UUID，权限已设为 600，请妥善保管，不要发给他人。"
	ImportNote       = "导入会替换当前的节点、客户端、转发规则和手动防火墙规则（导入前自动备份当前状态）；SSH、系统优化和防火墙开关保持不变。"
	ImportDone       = "配置已导入。"
	ImportShareHint  = "分享链接的地址按本机重新检测：在\"分享节点\"中确认主机地址。"
	ErrNotExportFile = "这不是 LiteNode 导出的配置文件。"
	ErrImportTooNew  = "导出文件来自更新版本的 LiteNode，请先更新 LiteNode 再导入。"
	ImportPortsTaken = "以下端口已被本机其他程序占用，请停止这些程序或修改导出文件后重试："
	ImportAborted    = "导入已中止，当前配置未改变。"
	ErrImportNoFile  = "文件不存在。"
)

// ExportDone reports the export.
func ExportDone(path string, nodes, clients, forwards, rules int) string {
	return fmt.Sprintf("已导出到 %s：%d 个节点、%d 个客户端、%d 条转发规则、%d 条防火墙规则。", path, nodes, clients, forwards, rules)
}

// ImportSummary describes what the import replaces.
func ImportSummary(nodes, clients, forwards, rules int, exported, version string) string {
	return fmt.Sprintf("导出文件（%s，LiteNode %s）包含 %d 个节点、%d 个客户端、%d 条转发规则、%d 条防火墙规则。", exported, version, nodes, clients, forwards, rules)
}

// ConfirmImport asks before replacing the current items.
func ConfirmImport(nodes, clients, forwards, rules int) string {
	return fmt.Sprintf("将替换当前的 %d 个节点、%d 个客户端、%d 条转发规则、%d 条防火墙规则。确定导入？", nodes, clients, forwards, rules)
}

// ErrImportInvalid explains an export file that fails the checks.
func ErrImportInvalid(err error) string {
	return fmt.Sprintf("导出文件内容有误，未导入：%v", err)
}

// ImportPortTaken names an imported item whose port is taken.
func ImportPortTaken(port int, proto, owner string) string {
	return fmt.Sprintf("%s：端口 %d/%s", ownerName(owner), port, proto)
}

// Self update (DESIGN section 3).
const (
	UpdateChecking  = "正在查询最新版本……"
	UpdateUpdating  = "正在下载并校验……"
	UpdateRestart   = "当前运行的仍是旧版本，请重新运行 lnode 使用新版本。"
	ErrUpdateDigest = "下载的文件 sha256 校验失败，可能已损坏或被篡改，未做任何修改。"
)

// UpdateLatest reports that no newer release exists.
func UpdateLatest(current string) string { return "已是最新版本（" + current + "）。" }

// ConfirmUpdate asks before updating.
func ConfirmUpdate(current, latest string) string {
	return fmt.Sprintf("当前版本 %s，最新版本 %s。确定更新？", current, latest)
}

// UpdateDone reports the update and where the old program is.
func UpdateDone(latest, bak string) string {
	return fmt.Sprintf("已更新到 %s，旧版本保留为 %s（恢复：mv %s /usr/local/bin/litenode）。", latest, bak, bak)
}

// UpdateFailed explains a failed query or download (D-042); noIPv4 as in
// XrayInstallFailed.
func UpdateFailed(err error, noIPv4 bool) string {
	if noIPv4 {
		return fmt.Sprintf("更新失败，未做任何修改：%v\n%s", err, NoIPv4Download)
	}
	return fmt.Sprintf("更新失败，未做任何修改：%v\n如果这台机器无法访问 GitHub，手动更新方法见 %s", err, NoGitHubURL)
}

// ErrUpdateBadBinary explains a downloaded program that does not run.
func ErrUpdateBadBinary(err error) string {
	return fmt.Sprintf("下载的程序无法正常运行，未做任何修改：%v", err)
}

// Uninstall (DESIGN section 16).
const (
	ConfirmRemoveProgram = "将删除 LiteNode 程序（litenode 和 lnode），Xray、节点、转发规则、防火墙和 /etc/litenode/ 中的配置保留并继续运行，重新安装后可以继续管理。确定卸载？"
	RemoveProgramDone    = "LiteNode 程序已删除，Xray 和转发规则继续运行。"
	ConfirmPurgeAgain    = "此操作不可恢复。再次确认全部卸载？"
	PurgeDone            = "LiteNode 已全部卸载。"
	KeptTitle            = "以下设置保留未改动（卸载不会回滚，可以以后手动处理）："
	ReinstallHint        = "重新安装：bash <(curl -fsSL https://raw.githubusercontent.com/Aspeternity/litenode/main/scripts/install.sh)"
)

// ConfirmPurge lists what the full uninstall deletes.
func ConfirmPurge(nodes, clients, forwards int, firewallOn bool) string {
	fw := "LiteNode 防火墙未启用，其规则文件一并删除"
	if firewallOn {
		fw = "LiteNode 防火墙保持生效（只保留 SSH 端口和手动规则）"
	}
	return fmt.Sprintf("将删除 Xray、%d 个节点、%d 个客户端、%d 条转发规则、LiteNode 的服务和程序，以及 /etc/litenode/ 中的状态和备份；%s。需要保留节点可以先导出配置。确定全部卸载？", nodes, clients, forwards, fw)
}

// KeptNames describe the settings that stay, by maint.Kept* key.
var KeptNames = map[string]string{
	"ssh":      "SSH 设置",
	"ufw-ssh":  "ufw 中的 SSH 端口规则（litenode:ssh）",
	"firewall": "LiteNode 防火墙（开机由 litenode-nft.service 加载）",
	"sysctl":   "sysctl 设置（BBR 等）",
	"swap":     "swap 文件（在 /etc/fstab 中挂载）",
	"dns":      "DNS 设置",
	"gai":      "系统 IPv4 / IPv6 优先级（LiteNode 标记的段落）",
	"log":      "LiteNode 操作日志",
}

// KeptLine names one kept setting and its file.
func KeptLine(what, path string) string {
	name, ok := KeptNames[what]
	if !ok {
		name = what
	}
	return "  • " + name + "：" + path
}
