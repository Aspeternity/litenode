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

// DryRunMkdir describes a directory creation that dry-run mode skipped.
func DryRunMkdir(path, mode string) string {
	return fmt.Sprintf("[dry-run] 创建目录：%s（权限 %s）", path, mode)
}
