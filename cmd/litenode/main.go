// Command litenode is a lightweight VPS node deployment and management tool.
package main

import (
	"context"
	"os"

	"github.com/Aspeternity/litenode/internal/app"
	"github.com/Aspeternity/litenode/internal/system"
)

func main() {
	os.Exit(app.Run(context.Background(), app.Env{
		Runner: system.NewReal(system.NewOpLog(system.DefaultLogPath)),
		Euid:   os.Geteuid(),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}
