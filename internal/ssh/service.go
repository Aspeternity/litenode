package ssh

import (
	"context"
	"io"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/ufw"
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
	// Firewall keeps the firewall rules in step with the SSH port
	// (firewall.SyncKind, which imports this package). Port changes apply
	// it after sshd, so it opens the port sshd then uses. Nil means none.
	Firewall *change.Kind
}

// portParts are the parts of a change that may move sshd to another
// port: ufw first, so the new port is open before sshd moves (D-065),
// then sshd, then the firewall (D-074).
func (s *Service) portParts(sshFile change.File) []change.Part {
	parts := []change.Part{
		{Kind: ufw.Kind(s.Store)},
		{Kind: Kind(s.Store), Files: []change.File{sshFile}},
	}
	if s.Firewall != nil {
		parts = append(parts, change.Part{Kind: s.Firewall})
	}
	return parts
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
		Parts: []change.Part{{Kind: Kind(s.Store), Files: []change.File{ConfigFile(st.SSH, HasIPv6(s.runner()))}}},
		State: st,
	})
	return err
}
