// Command litenode is a lightweight VPS node deployment and management tool.
package main

import (
	"context"
	"os"

	"github.com/Aspeternity/litenode/internal/app"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

func main() {
	r := system.NewReal(system.NewOpLog(system.DefaultLogPath))
	os.Exit(app.Run(context.Background(), app.Env{
		Runner:   r,
		Euid:     os.Geteuid(),
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		LockPath: state.DefaultLockPath,
		Store:    state.NewStore(r),
	}))
}
