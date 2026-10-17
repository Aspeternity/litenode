// Command litenode is a lightweight VPS node deployment and management tool.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Aspeternity/litenode/internal/app"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ui"
)

// version is set at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	exe, err := os.Executable()
	if err != nil {
		exe = "/usr/local/bin/litenode"
	}

	// Interrupts are handled by the menu session: they quit at a prompt and
	// are deferred while an operation runs. SIGHUP covers a dropped SSH
	// connection.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	session := ui.NewSession(os.Stdin, os.Stdout, sigs, ui.NewColors(os.Getenv("NO_COLOR")))

	os.Exit(app.Run(context.Background(), app.Env{
		Runner:     system.NewReal(system.NewOpLog(system.DefaultLogPath)),
		Euid:       os.Geteuid(),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Args:       os.Args[1:],
		Version:    version,
		Executable: exe,
		LockPath:   state.DefaultLockPath,
		Session:    session,
	}))
}
