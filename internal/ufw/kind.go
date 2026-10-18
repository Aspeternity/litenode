package ufw

import (
	"context"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Kind is the "ufw" configuration kind. It has no files: Apply reconciles
// ufw with the state on disk, so after a rollback the rules added by the
// rolled-back change are removed again (D-038, D-040).
func Kind(store *state.Store) *change.Kind {
	return &change.Kind{
		Name: "ufw",
		Apply: func(ctx context.Context, r system.Runner) error {
			st, err := store.Load()
			if err != nil {
				return err
			}
			// The LiteNode firewall (M3) is not implemented yet: treat it
			// as disabled.
			return Reconcile(ctx, r, Desired(st), false)
		},
	}
}
