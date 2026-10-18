package app

import (
	"context"
	"errors"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// awaitConfirm asks the user to test a new login after a change that may
// affect SSH access, then keeps or undoes it (DESIGN section 6). It
// reports whether the change was kept. If the user quits, the timer undoes
// the change.
func awaitConfirm(ctx context.Context, s *ui.Session, c *change.Confirmer, reg change.Registry) (bool, error) {
	ok, err := s.Confirm(text.ConfirmKeep(c.Seconds))
	if errors.Is(err, ui.ErrQuit) {
		s.Println(s.Colors.Warn(text.QuitUnconfirmed))
		return false, err
	}
	if err != nil {
		return false, err
	}
	if ok {
		err = c.Disarm(ctx)
	} else {
		err = c.Revert(ctx, reg)
	}
	switch {
	case errors.Is(err, change.ErrRolledBack):
		s.Println(s.Colors.Warn(text.ConfirmTimedOut))
		return false, nil
	case err != nil:
		return false, err
	case ok:
		s.Println(s.Colors.Good(text.ChangeKept))
		return true, nil
	}
	s.Println(text.ChangeReverted)
	return false, nil
}
