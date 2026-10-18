package ssh

import (
	"context"
	"io"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
)

// Service changes the SSH settings. Every change runs under
// confirm-or-rollback (DESIGN sections 6 and 13): after Change returns,
// the menu asks the user to test a new login, then keeps the change with
// Confirmer.Disarm or undoes it with Confirmer.Revert; if nobody answers,
// the systemd timer undoes it.
type Service struct {
	Store     *state.Store
	Confirmer *change.Confirmer
	Fetcher   Fetcher
	// Rand is the randomness for generated keys (crypto/rand.Reader).
	Rand io.Reader
}

// Kinds are the configuration kinds of SSH changes, for the rollback
// registry.
func (s *Service) Kinds() []*change.Kind {
	return []*change.Kind{Kind(s.Store), AuthKeysKind()}
}

// Change applies fn to the SSH settings and arms the rollback.
func (s *Service) Change(ctx context.Context, fn func(*state.SSH)) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	fn(&st.SSH)
	_, err = s.Confirmer.Apply(ctx, change.Change{
		Parts: []change.Part{{Kind: Kind(s.Store), Files: []change.File{ConfigFile(st.SSH)}}},
		State: st,
	})
	return err
}
