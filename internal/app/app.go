// Package app implements the startup flow and subcommand routing.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// Env is what LiteNode needs from the outside world.
type Env struct {
	// Runner executes system operations for real. Run wraps it in a
	// dry-run Runner when --dry-run is given.
	Runner system.Runner
	Euid   int
	Stdout io.Writer
	Stderr io.Writer
	// Args are the command-line arguments without the program name.
	Args []string
	// Version is the build version, "dev" when not injected.
	Version string
	// Executable is the absolute path of the litenode binary, used in
	// systemd units that call back into LiteNode.
	Executable string
	// LockPath is the instance lock file.
	LockPath string
	// Kinds lists the configuration kinds the rollback subcommand handles.
	Kinds change.Registry
	// Session reads menu input. Nil means no interactive menu (tests).
	Session *ui.Session
}

// Run dispatches the command line and returns the process exit code.
//
//	litenode [--dry-run]           main menu
//	litenode version | -v | --version
//	litenode help | -h | --help
//	litenode internal rollback <kind>   (called by systemd; not in help)
func Run(ctx context.Context, env Env) int {
	args := env.Args
	switch {
	case len(args) == 0:
		return runMenu(ctx, env, false)
	case len(args) == 1 && args[0] == "--dry-run":
		return runMenu(ctx, env, true)
	case len(args) == 1 && (args[0] == "version" || args[0] == "-v" || args[0] == "--version"):
		fmt.Fprintln(env.Stdout, text.Version(env.Version, runtime.GOOS+"/"+runtime.GOARCH))
		return 0
	case len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help"):
		fmt.Fprint(env.Stdout, text.Help)
		return 0
	case len(args) >= 1 && args[0] == "internal":
		return runInternal(ctx, env, args[1:])
	}
	fmt.Fprintln(env.Stderr, text.UnknownCommand(strings.Join(args, " ")))
	return 2
}

// runInternal handles subcommands that systemd units call. They are not
// listed in help.
func runInternal(ctx context.Context, env Env, args []string) int {
	if len(args) == 2 && args[0] == "rollback" {
		return rollback(ctx, env, args[1])
	}
	fmt.Fprintln(env.Stderr, text.UnknownCommand(strings.Join(append([]string{"internal"}, args...), " ")))
	return 2
}

// runMenu checks the environment, takes the instance lock, loads the state
// and shows the main menu.
func runMenu(ctx context.Context, env Env, dryRun bool) int {
	r := env.Runner
	if dryRun {
		r = system.NewDryRun(env.Runner, env.Stdout)
	}
	info, code := startupCheck(ctx, env.Euid, r, env.Stderr)
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

	store := state.NewStore(r)
	st, code := loadState(store, env.Stderr)
	if code != 0 {
		return code
	}
	if env.Session == nil {
		return 0
	}
	if dryRun {
		env.Session.Println(env.Session.Colors.Warn(text.DryRunBanner))
	}
	// Both going back from the main menu and quitting end the program.
	env.Session.Run(func() ui.Menu {
		return mainMenu(env.Session, env.Version, info, st)
	})
	return 0
}

// startupCheck requires root and a supported system, then returns the
// detected system information.
func startupCheck(ctx context.Context, euid int, r system.Runner, stderr io.Writer) (system.Info, int) {
	if euid != 0 {
		fmt.Fprintln(stderr, text.NotRoot)
		return system.Info{}, 1
	}
	info, err := system.Detect(ctx, r)
	if err != nil {
		fmt.Fprintln(stderr, text.DetectFailed(err))
		return system.Info{}, 1
	}
	if !info.OS.Supported() {
		name := info.OS.ShortName()
		if name == "" {
			name = text.UnknownOS
		}
		fmt.Fprintln(stderr, text.UnsupportedOS(name))
		return system.Info{}, 1
	}
	return info, 0
}

// loadState loads the state file and explains failures to the user.
func loadState(store *state.Store, stderr io.Writer) (*state.State, int) {
	st, err := store.Load()
	if err == nil {
		return st, 0
	}
	var corrupt *state.CorruptError
	var tooNew *state.TooNewError
	switch {
	case errors.As(err, &corrupt):
		fmt.Fprintln(stderr, text.StateCorrupt(store.Path, store.BackupDir, corrupt.Err))
	case errors.As(err, &tooNew):
		fmt.Fprintln(stderr, text.StateTooNew(store.Path))
	default:
		fmt.Fprintln(stderr, text.StateLoadFailed(err))
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
	store := state.NewStore(env.Runner)
	confirmer := change.NewConfirmer(change.NewManager(env.Runner, store), env.Executable)
	done, err := confirmer.Rollback(ctx, env.Kinds, kind)
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
