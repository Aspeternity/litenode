package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/forwardmgr"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
)

// forwards builds the forward service on r.
func (env Env) forwards(r system.Runner, store *state.Store) *forwardmgr.Service {
	return &forwardmgr.Service{Runner: r, Store: store, Changes: change.NewManager(r, store),
		Resolver: env.ForwardResolver, Executable: env.Executable, Dialer: env.ForwardDialer}
}

// resolve is run by litenode-resolve.timer: it looks up the domain
// targets of port forwards again and updates the rules when an address
// changed (DESIGN section 11). While a menu holds the instance lock it
// skips the round, so it never changes the state under the menu.
func resolve(ctx context.Context, env Env) int {
	if env.Euid != 0 {
		fmt.Fprintln(env.Stderr, text.NotRoot)
		return 1
	}
	lock, err := state.AcquireLock(env.LockPath)
	if errors.Is(err, state.ErrLocked) {
		fmt.Fprintln(env.Stdout, text.ResolveSkipped)
		return 0
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, text.LockFailed(err))
		return 1
	}
	defer lock.Release()

	res, err := env.forwards(env.Runner, state.NewStore(env.Runner)).Resolve(ctx)
	for _, e := range res.Failed {
		fmt.Fprintln(env.Stderr, text.ResolveLookupFailed(e))
	}
	var te *forwardmgr.TimerError
	switch {
	case errors.As(err, &te):
		fmt.Fprintln(env.Stderr, text.ResolveTimerFailed(te.Err))
	case err != nil:
		fmt.Fprintln(env.Stderr, text.ResolveFailed(err))
		return 1
	}
	for _, f := range res.Changed {
		fmt.Fprintln(env.Stdout, text.ResolveUpdated(f.ID, f.TargetHost, f.ResolvedIPv4, f.ResolvedIPv6))
	}
	if te != nil {
		return 1
	}
	return 0
}
