// Package app implements the startup flow and subcommand routing.
package app

import (
	"context"
	"fmt"
	"io"

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
}

// Run checks the environment and starts LiteNode. It returns the process
// exit code.
func Run(ctx context.Context, env Env) int {
	info, code := startupCheck(ctx, env)
	if code != 0 {
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
