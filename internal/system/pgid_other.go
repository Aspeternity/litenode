//go:build !unix

package system

import "os/exec"

func ownProcessGroup(*exec.Cmd) {}
