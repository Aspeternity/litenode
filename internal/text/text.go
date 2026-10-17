// Package text holds every piece of user-visible text (menus, prompts,
// confirmations and error messages). Other packages must not contain
// Chinese string literals; see TestNoChineseOutsideText.
package text

import (
	"fmt"
	"strings"
)

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

// ConflictXUI is the display name of x-ui or 3x-ui.
const ConflictXUI = "x-ui/3x-ui"

// ConflictXray is the display name of an Xray service from the official script.
const ConflictXray = "Xray（官方脚本）"

// ConflictDocker is the display name of Docker.
const ConflictDocker = "Docker"

// ConflictWarning lists detected software that may conflict with LiteNode.
func ConflictWarning(names []string) string {
	return "⚠ 检测到 " + strings.Join(names, "、") + "，可能与 LiteNode 冲突"
}

// MenuInDevelopment is shown until the main menu is implemented.
const MenuInDevelopment = "主菜单开发中。"
