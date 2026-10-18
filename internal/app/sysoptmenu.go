package app

import (
	"context"
	"strings"

	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// sysoptUI holds the system optimization menu (DESIGN section 15). It
// only displays and reads input; sysopt.Service does the work.
type sysoptUI struct {
	ctx  context.Context
	s    *ui.Session
	svc  *sysopt.Service
	info system.Info
	caps system.Capabilities
}

// mark shows a yes / no state.
func (u *sysoptUI) mark(on bool) string {
	if on {
		return u.s.Colors.Good(text.StateOn)
	}
	return u.s.Colors.Off(text.StateOff)
}

func (u *sysoptUI) menu() ui.Menu {
	var fields []string
	bbr, err := u.svc.BBRStatus()
	if err != nil {
		return ui.Menu{Heading: text.MenuSysopt, Status: []string{u.s.Colors.Bad(text.ActionFailed(err))}}
	}
	if u.caps.Has(system.CapBBR) {
		fields = append(fields, text.StatusBBR(u.mark(bbr.Active)))
	}
	m := ui.Menu{
		Heading: text.MenuSysopt,
		Status:  []string{strings.Join(fields, "   ")},
		Items: []ui.Item{
			{Label: text.SysoptBBR, Hidden: !u.caps.Has(system.CapBBR), Action: func() error { return u.bbr(bbr) }},
		},
	}
	for _, it := range m.Items {
		if it.Hidden {
			m.Notice = u.s.Colors.Warn(text.SysoptUnavailableIn(ui.VirtName(u.info.Virt)))
			break
		}
	}
	return m
}

func (u *sysoptUI) bbr(b sysopt.BBR) error {
	switch {
	case b.Active && !b.Managed:
		u.s.End(text.SysoptBBRByOther)
		return nil
	case b.Active:
		ok, err := u.s.Confirm(text.ConfirmBBRDisable)
		if err != nil || !ok {
			return err
		}
		if err := u.svc.SetBBR(u.ctx, false); err != nil {
			return err
		}
		u.s.Done(text.BBRDisabledDone)
		return nil
	}
	ok, err := u.s.Confirm(text.ConfirmBBREnable)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.SetBBR(u.ctx, true); err != nil {
		return err
	}
	u.s.Done(text.BBREnabledDone)
	return nil
}
