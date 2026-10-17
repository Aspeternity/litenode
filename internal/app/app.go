// Package app implements the startup flow and subcommand routing.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
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
	// Args are the command-line arguments without the program name.
	Args []string
	// Confirmer schedules and performs confirm-or-rollback.
	Confirmer *change.Confirmer
	// Kinds lists the configuration kinds the rollback subcommand handles.
	Kinds change.Registry
	// Version is shown in the main menu title.
	Version string
	// Session reads menu input. Nil means no interactive menu (tests).
	Session *ui.Session
}

// Run checks the environment and starts LiteNode. It returns the process
// exit code.
func Run(ctx context.Context, env Env) int {
	// Minimal routing until the subcommand framework (M0-07).
	if len(env.Args) > 0 {
		if len(env.Args) == 3 && env.Args[0] == "internal" && env.Args[1] == "rollback" {
			return rollback(ctx, env, env.Args[2])
		}
		fmt.Fprintln(env.Stderr, text.UnknownCommand(strings.Join(env.Args, " ")))
		return 2
	}
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
	st, code := loadState(env)
	if code != 0 {
		return code
	}
	if env.Session == nil {
		return 0
	}
	// Both going back from the main menu and quitting end the program.
	env.Session.Run(func() ui.Menu {
		return mainMenu(env.Session, env.Version, info, st)
	})
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

// rollback is run by the systemd timer when a change was not confirmed in
// time. It deliberately ignores the instance lock: the menu that made the
// change is usually still running (or hung after SSH dropped), and the
// rollback must happen regardless.
func rollback(ctx context.Context, env Env, kind string) int {
	if env.Euid != 0 {
		fmt.Fprintln(env.Stderr, text.NotRoot)
		return 1
	}
	done, err := env.Confirmer.Rollback(ctx, env.Kinds, kind)
	switch {
	case err != nil:
		fmt.Fprintln(env.Stderr, text.RollbackFailed(err))
		return 1
	case done:
		fmt.Fprintln(env.Stdout, text.RollbackDone(kind))
	default:
		fmt.Fprintln(env.Stdout, text.RollbackNotNeeded)
	}
	return 0
}
