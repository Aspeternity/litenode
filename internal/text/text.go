// Package text holds every piece of user-visible text (menus, prompts,
// confirmations and error messages). Other packages must not contain
// Chinese string literals; see TestNoChineseOutsideText.
package text

import "fmt"

// DryRunExec describes a command that dry-run mode skipped.
func DryRunExec(cmdline string) string {
	return "[dry-run] 执行命令：" + cmdline
}

// DryRunWriteFile describes a file write that dry-run mode skipped.
func DryRunWriteFile(path string, size int, mode string) string {
	return fmt.Sprintf("[dry-run] 写入文件：%s（%d 字节，权限 %s）", path, size, mode)
}

// DryRunRemove describes a file removal that dry-run mode skipped.
func DryRunRemove(path string) string {
	return "[dry-run] 删除文件：" + path
}

// DryRunRename describes a rename that dry-run mode skipped.
func DryRunRename(oldPath, newPath string) string {
	return fmt.Sprintf("[dry-run] 重命名：%s → %s", oldPath, newPath)
}

// DryRunChmod describes a permission change that dry-run mode skipped.
func DryRunChmod(path, mode string) string {
	return fmt.Sprintf("[dry-run] 修改权限：%s（%s）", path, mode)
}

// DryRunMkdir describes a directory creation that dry-run mode skipped.
func DryRunMkdir(path, mode string) string {
	return fmt.Sprintf("[dry-run] 创建目录：%s（权限 %s）", path, mode)
}

// NotRoot is shown when LiteNode is not run as root.
const NotRoot = "LiteNode 需要 root 权限运行。请切换到 root 用户（sudo -i）后重新执行。"

// UnsupportedOS is shown when the system is not a supported Debian/Ubuntu release.
func UnsupportedOS(name string) string {
	return fmt.Sprintf("不支持当前系统：%s。\nLiteNode 支持 Debian 11 及以上、Ubuntu 22.04 及以上的 LTS 版本。", name)
}

// DetectFailed is shown when system detection fails.
func DetectFailed(err error) string {
	return fmt.Sprintf("无法检测系统信息：%v\n请确认 /etc/os-release 存在且可读。", err)
}

// UnknownOS names a system whose os-release has no usable name.
const UnknownOS = "未知系统"

// VirtNone is the display name for bare metal.
const VirtNone = "物理机"

// VirtUnknown is the display name when virtualization cannot be detected.
const VirtUnknown = "未知虚拟化"

// BBR shows whether BBR is enabled.
func BBR(enabled bool) string {
	if enabled {
		return "BBR ✓"
	}
	return "BBR ✗"
}

// Exit marks of the main menu system line (DESIGN section 19.1).
const (
	ExitOK   = "✓"
	ExitWARP = "WARP"
	ExitNone = "✗"
)

// Exits shows the IPv4 and IPv6 exits, e.g. "IPv4 WARP  IPv6 ✓".
func Exits(v4, v6 string) string { return "IPv4 " + v4 + "  IPv6 " + v6 }

// Warning marks a warning line in a status area.
func Warning(s string) string {
	return "⚠ " + s
}

// AlreadyRunning is shown when another LiteNode instance holds the lock.
const AlreadyRunning = "另一个 LiteNode 正在运行。请先退出另一个窗口中的 LiteNode，再重新执行。"

// NoGitHubURL is the README section on machines that cannot reach
// GitHub, including the manual install steps (D-042).
const NoGitHubURL = "https://github.com/Aspeternity/litenode#无法访问-github-的机器"

// IPv6OnlyURL is the README section on IPv6-only machines.
const IPv6OnlyURL = "https://github.com/Aspeternity/litenode#纯-ipv6-的-vps"

// LockFailed is shown when the lock file cannot be created.
func LockFailed(err error) string {
	return fmt.Sprintf("无法创建锁文件：%v\n请确认以 root 运行，且 /run 目录可写。", err)
}

// StateCorrupt is shown when the state file cannot be parsed.
func StateCorrupt(path, backupDir string, err error) string {
	return fmt.Sprintf("状态文件 %s 已损坏，无法读取：%v\n"+
		"LiteNode 不会自动覆盖它。请从备份目录 %s 中选择一份复制回 %s，再重新执行。", path, err, backupDir, path)
}

// StateTooNew is shown when the state file was written by a newer LiteNode.
func StateTooNew(path string) string {
	return fmt.Sprintf("状态文件 %s 由更新版本的 LiteNode 生成，当前程序无法读取。\n"+
		"请更新 LiteNode 后重新执行。", path)
}

// StateLoadFailed is shown for other errors while loading the state file.
func StateLoadFailed(err error) string {
	return fmt.Sprintf("无法读取状态文件：%v\n请确认以 root 运行，且 /etc/litenode/ 中的文件可读。", err)
}

// RollbackDone is printed by the rollback subcommand after a timeout.
const RollbackDone = "未在限时内确认，已恢复修改前的配置。"

// RollbackNotNeeded is printed when the change was confirmed in time.
const RollbackNotNeeded = "修改已确认，无需恢复。"

// ConfirmKeep asks the user to test a new login before keeping a change
// that may affect SSH access (DESIGN section 6).
func ConfirmKeep(seconds int) string {
	return fmt.Sprintf("请新开一个终端测试能否登录。登录正常请输入 y 保留修改，输入其他内容立即恢复；%d 秒内未确认将自动恢复。", seconds)
}

// Results of the confirmation.
const (
	ChangeKept      = "已确认，修改已保留。"
	ChangeReverted  = "已恢复修改前的配置。"
	ConfirmTimedOut = "已超时，配置已自动恢复，请重新修改。"
	QuitUnconfirmed = "修改未确认，将在限时到期后自动恢复。"
)

// RollbackFailed is printed when the rollback itself fails.
func RollbackFailed(err error) string {
	return fmt.Sprintf("自动恢复失败：%v\n请通过 VNC 控制台登录，从 /etc/litenode/backups/ 中手动恢复。", err)
}

// UnknownCommand is shown for an unrecognized command line.
func UnknownCommand(args string) string {
	return "未知命令：" + args + "\n运行 litenode help 查看用法。"
}

// Menu option 0 labels.
const (
	Back = "返回"
	Exit = "退出"
)

// MainTitle is the main menu title.
func MainTitle(version string) string {
	return "LiteNode 节点管理  " + version
}

// MainMenuName is the main menu's name in page paths (D-096).
const MainMenuName = "主菜单"

// PathSeparator joins page names in a page heading: 主菜单 › 分享节点.
const PathSeparator = " › "

// Main menu items.
const (
	MenuNodes        = "节点管理"
	MenuClients      = "客户端管理"
	MenuShare        = "分享节点"
	MenuHostAddress  = "配置主机地址"
	MenuForwards     = "端口转发"
	MenuXray         = "Xray 服务"
	MenuSSH          = "SSH 管理"
	MenuFirewall     = "防火墙管理"
	MenuSysopt       = "系统优化"
	MenuMaint        = "维护"
	StatusUnknown    = "-"
	XrayNotInstalled = "○ 未安装"
)

// StatusXray, StatusNodes, StatusForwards, StatusSSH and StatusFirewall
// label the fields of the main menu status area.
func StatusXray(v string) string     { return "Xray: " + v }
func StatusNodes(n int) string       { return fmt.Sprintf("节点: %d", n) }
func StatusForwards(n int) string    { return fmt.Sprintf("转发规则: %d", n) }
func StatusSSH(v string) string      { return "SSH: " + v }
func StatusFirewall(v string) string { return "防火墙: " + v }

// PromptChoice asks for a menu option between 0 and max.
func PromptChoice(max int) string {
	return fmt.Sprintf("请输入选项 [0-%d]: ", max)
}

// InvalidChoice is shown for input that is not a listed option.
func InvalidChoice(max int) string {
	return fmt.Sprintf("无效的选项，请输入 0 到 %d 之间的数字。", max)
}

// PressEnter waits, after an operation that showed something to read,
// before the menu comes back (D-096).
const PressEnter = "按回车返回菜单"

// ConfirmSuffix is appended to yes/no questions; the default is no.
const ConfirmSuffix = " [y/N]: "

// ConfirmSuffixYes is appended to yes/no questions whose default is yes.
const ConfirmSuffixYes = " [Y/n]: "

// BusyInterrupt is shown when Ctrl+C is pressed while an operation runs.
const BusyInterrupt = "正在执行操作，完成后退出……"

// ActionFailed is shown when a menu action returns an error.
func ActionFailed(err error) string {
	return "操作失败：" + err.Error()
}

// Version is printed by "litenode version".
func Version(version, platform string) string {
	return fmt.Sprintf("LiteNode %s (%s)", version, platform)
}

// Help is printed by "litenode help". Internal subcommands are not listed.
const Help = `LiteNode：轻量的 VPS 节点部署与管理工具

用法：
  litenode              进入主菜单（快捷命令 lnode）
  litenode --dry-run    以 dry-run 模式进入主菜单：只打印将要执行的修改，不实际执行
  litenode version      显示版本（也可用 -v、--version）
  litenode help         显示本帮助（也可用 -h、--help）

需要 root 权限运行（version 和 help 除外）。
项目主页：https://github.com/Aspeternity/litenode
`

// DryRunBanner is shown when the menu starts in dry-run mode.
const DryRunBanner = "[dry-run] 当前为 dry-run 模式：所有修改操作只打印，不会实际执行。"

// Flow endings (D-069).
const (
	FlowCanceled = "已取消"
	FlowFailed   = "✗ 未完成"
	// Cancel is option 0 of a flow step.
	Cancel = "取消"
)

// LinkBelow ends the "share link" flow; the link follows it.
const LinkBelow = "分享链接如下"

// KeyGenerated ends the key generation flow; the private key follows it.
const KeyGenerated = "密钥对已生成，公钥已加入 authorized_keys。"

// Unchanged is shown when a choice equals the current setting.
const Unchanged = "设置未变化。"
