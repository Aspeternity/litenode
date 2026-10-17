// Package app implements the startup flow and subcommand routing.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// Env is what the startup flow needs from the outside world.
type Env struct {
	Runner system.Runner
	Euid   int
	Stdout io.Writer
	Stderr io.Writer
	// LockPath is the instance lock file.
	LockPath string
	// Store loads and saves the state file.
	Store *state.Store
}

// Run checks the environment and starts LiteNode. It returns the process
// exit code.
func Run(ctx context.Context, env Env) int {
	info, code := startupCheck(ctx, env)
	if code != 0 {
		return code
	}
	lock, err := state.AcquireLock(env.LockPath)
	if errors.Is(err, state.ErrLocked) {
		fmt.Fprintln(env.Stderr, text.AlreadyRunning)
		return 1
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, text.LockFailed(err))
		return 1
	}
	defer lock.Release()
	if _, code := loadState(env); code != 0 {
		return code
	}
	// Placeholder until the main menu (M0-06): show the status lines.
	fmt.Fprintln(env.Stdout, ui.SystemLine(info))
	if line := ui.ConflictLine(info.Conflicts); line != "" {
		fmt.Fprintln(env.Stdout, line)
	}
	fmt.Fprintln(env.Stdout, text.MenuInDevelopment)
	return 0
}

// startupCheck requires root and a supported system, then returns the
// detected system information.
func startupCheck(ctx context.Context, env Env) (system.Info, int) {
	if env.Euid != 0 {
		fmt.Fprintln(env.Stderr, text.NotRoot)
		return system.Info{}, 1
	}
	info, err := system.Detect(ctx, env.Runner)
	if err != nil {
		fmt.Fprintln(env.Stderr, text.DetectFailed(err))
		return system.Info{}, 1
	}
	if !info.OS.Supported() {
		name := info.OS.ShortName()
		if name == "" {
			name = text.UnknownOS
		}
		fmt.Fprintln(env.Stderr, text.UnsupportedOS(name))
		return system.Info{}, 1
	}
	return info, 0
}

// loadState loads the state file and explains failures to the user.
func loadState(env Env) (*state.State, int) {
	st, err := env.Store.Load()
	if err == nil {
		return st, 0
	}
	var corrupt *state.CorruptError
	var tooNew *state.TooNewError
	switch {
	case errors.As(err, &corrupt):
		fmt.Fprintln(env.Stderr, text.StateCorrupt(env.Store.Path, env.Store.BackupDir, corrupt.Err))
	case errors.As(err, &tooNew):
		fmt.Fprintln(env.Stderr, text.StateTooNew(env.Store.Path))
	default:
		fmt.Fprintln(env.Stderr, text.StateLoadFailed(err))
	}
	return nil, 1
}
