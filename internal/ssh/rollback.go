package ssh

import (
	"context"
	"errors"
	"reflect"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// ErrNothingToRollBack means there is no earlier SSH configuration to go
// back to.
var ErrNothingToRollBack = errors.New("no earlier SSH configuration")

// Previous returns the SSH settings from before the last change of the
// sshd configuration (DESIGN section 13).
func (s *Service) Previous() (state.SSH, error) {
	m := s.Confirmer.Manager
	ids, err := m.Backups("sshd")
	if err != nil {
		return state.SSH{}, err
	}
	if len(ids) == 0 {
		return state.SSH{}, ErrNothingToRollBack
	}
	b, err := m.LoadBackup(ids[len(ids)-1])
	if err != nil {
		return state.SSH{}, err
	}
	st, err := m.BackupState(b)
	if err != nil {
		return state.SSH{}, err
	}
	return st.SSH, nil
}

// RollBackLast restores the SSH settings from before the last change,
// under confirm-or-rollback. Only the SSH settings go back: the backup's
// whole state would also undo later node changes (D-067). The rollback is
// itself a change, so doing it again returns to where it started.
func (s *Service) RollBackLast(ctx context.Context) (state.SSH, error) {
	prev, err := s.Previous()
	if err != nil {
		return state.SSH{}, err
	}
	st, err := s.Store.Load()
	if err != nil {
		return state.SSH{}, err
	}
	if reflect.DeepEqual(prev, st.SSH) {
		return state.SSH{}, ErrNothingToRollBack
	}
	st.SSH = prev
	_, err = s.Confirmer.Apply(ctx, change.Change{
		Parts: []change.Part{
			{Kind: ufw.Kind(s.Store)},
			{Kind: Kind(s.Store), Files: []change.File{ConfigFile(prev, HasIPv6(s.runner()))}},
		},
		State: st,
	})
	return prev, err
}
