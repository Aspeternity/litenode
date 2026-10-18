package text

import "fmt"

// Maintenance menu (DESIGN section 16).
const (
	MaintExport     = "导出配置"
	MaintExportHint = "(节点 + 客户端 + 转发 + 防火墙规则)"
	MaintImport     = "导入配置"

	PromptExportPath = "导出文件路径"
	PromptImportPath = "导入文件路径（直接回车取消）"
	ExportNote       = "导出文件不包含 SSH 设置、系统优化设置、分享地址和节点出站 IP。"
	ExportSecret     = "文件包含节点私钥和客户端 UUID，权限已设为 600，请妥善保管，不要发给他人。"
	ImportNote       = "导入会替换当前的节点、客户端、转发规则和手动防火墙规则（导入前自动备份当前状态）；SSH、系统优化和防火墙开关保持不变。"
	ImportDone       = "配置已导入。"
	ImportShareHint  = "分享链接的地址按本机重新检测：在节点的分享链接中确认地址。"
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
