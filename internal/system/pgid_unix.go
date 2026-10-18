//go:build unix

package system

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup keeps Ctrl+C in the terminal from reaching cmd: only
// LiteNode gets SIGINT and decides how to stop the command.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
