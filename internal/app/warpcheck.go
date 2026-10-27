package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/warp"
)

// warpCheck is run by litenode-warp-check.timer: it moves the WARP tunnel
// to an endpoint that answers when its handshake is stale (DESIGN section
// 19.4). While the menu runs it skips the round.
func warpCheck(ctx context.Context, env Env) int {
	if env.Euid != 0 {
		fmt.Fprintln(env.Stderr, text.NotRoot)
		return 1
	}
	lock, err := state.AcquireLock(env.LockPath)
	if errors.Is(err, state.ErrLocked) {
		fmt.Fprintln(env.Stdout, text.WARPCheckSkipped)
		return 0
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, text.LockFailed(err))
		return 1
	}
	defer lock.Release()

	svc := &warp.Service{Runner: env.Runner}
	ep, err := svc.Watch(ctx)
	switch {
	case errors.Is(err, warp.ErrNotEnabled):
		return 0
	case err != nil:
		fmt.Fprintln(env.Stderr, text.WARPCheckFailed(err))
		return 1
	case ep.IsValid():
		fmt.Fprintln(env.Stdout, text.WARPReselected(ep.String()))
	}
	return 0
}
