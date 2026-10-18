// Package sysopt implements the system optimizations of DESIGN section
// 15: BBR, swap, IP preference, DNS and time zone. File changes go through
// the change flow, so a failure restores the previous files.
package sysopt

import (
	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Service runs system optimization operations.
type Service struct {
	Runner  system.Runner
	Store   *state.Store
	Changes *change.Manager
}
