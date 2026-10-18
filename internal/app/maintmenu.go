package app

import (
	"context"
	"errors"
	"io/fs"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/maint"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// maintUI is the maintenance menu (DESIGN section 16). It only displays
// and reads input; maint.Service does the work.
type maintUI struct {
	ctx context.Context
	s   *ui.Session
	svc *maint.Service
	reg change.Registry
}

func (u *maintUI) menu() ui.Menu {
	return ui.Menu{
		Heading: text.MenuMaint,
		Items: []ui.Item{
			{Label: text.MaintExport, Hint: text.MaintExportHint, Action: u.export},
			{Label: text.MaintImport, Action: u.importConfig},
		},
	}
}

func (u *maintUI) export() error {
	u.s.Println(text.ExportNote)
	path, err := u.s.PromptDefault(text.PromptExportPath, u.svc.DefaultExportPath())
	if err != nil {
		return err
	}
	sum, err := u.svc.Export(path)
	if err != nil {
		return err
	}
	u.s.Done(text.ExportDone(path, sum.Nodes, sum.Clients, sum.Forwards, sum.Rules))
	u.s.Println(u.s.Colors.Warn(text.ExportSecret))
	return nil
}

func (u *maintUI) importConfig() error {
	u.s.Println(text.ImportNote)
	path, err := u.s.Prompt(text.PromptImportPath)
	if err != nil || path == "" {
		return err
	}
	e, err := u.svc.Read(path)
	var tooNew *state.TooNewError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		u.s.End(u.s.Colors.Warn(text.ErrImportNoFile))
		return nil
	case errors.Is(err, maint.ErrNotExport):
		u.s.End(u.s.Colors.Warn(text.ErrNotExportFile))
		return nil
	case errors.As(err, &tooNew):
		u.s.End(u.s.Colors.Warn(text.ErrImportTooNew))
		return nil
	case errors.Is(err, maint.ErrInvalid):
		u.s.End(u.s.Colors.Warn(text.ErrImportInvalid(err)))
		return nil
	case err != nil:
		return err
	}
	in := maint.Summarize(e)
	u.s.Println(text.ImportSummary(in.Nodes, in.Clients, in.Forwards, in.Rules, e.ExportedAt, e.LiteNodeVersion))
	st, err := u.svc.Store.Load()
	if err != nil {
		return err
	}
	cur := maint.SummarizeState(st)
	ok, err := u.s.Confirm(text.ConfirmImport(cur.Nodes, cur.Clients, cur.Forwards, cur.Rules))
	if err != nil || !ok {
		return err
	}
	confirm, err := u.svc.Import(u.ctx, e)
	var taken *maint.PortsTakenError
	if errors.As(err, &taken) {
		u.s.Println(u.s.Colors.Warn(text.ImportPortsTaken))
		for _, c := range taken.Claims {
			u.s.Println("  " + text.ImportPortTaken(c.Port, string(c.Proto), c.Owner))
		}
		u.s.End(u.s.Colors.Warn(text.ImportAborted))
		return nil
	}
	if err != nil {
		return err
	}
	if confirm {
		if kept, err := awaitConfirm(u.ctx, u.s, u.svc.Confirmer, u.reg); err != nil || !kept {
			return err
		}
	}
	u.s.Done(text.ImportDone)
	u.s.Println(text.ImportShareHint)
	return nil
}
