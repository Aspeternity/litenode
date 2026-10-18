package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// firewallUI holds the firewall menu (DESIGN section 14). It only displays
// and reads input; firewall.Service does the work.
type firewallUI struct {
	ctx context.Context
	s   *ui.Session
	svc *firewall.Service
	// reg is the rollback registry, for undoing a change the user rejects.
	reg change.Registry
}

// firewallMainStatus is the firewall field of the main menu.
func firewallMainStatus(s *ui.Session, enabled bool) string {
	if enabled {
		return s.Colors.Good(text.FirewallOn)
	}
	return s.Colors.Off(text.FirewallOff)
}

// ruleLines renders rules as the numbered list of the menu status.
func ruleLines(es []firewall.Entry) []string {
	var rows [][]string
	for i, e := range es {
		port := e.Ports
		if e.Proto != "tcp+udp" {
			port += "/" + e.Proto
		}
		src := text.FirewallAnySource
		if len(e.Sources) > 0 {
			src = strings.Join(e.Sources, ",")
		}
		rows = append(rows, []string{"  [" + strconv.Itoa(i+1) + "]", port, src, ownerLabel(e)})
	}
	return ui.Table(rows)
}

func ownerLabel(e firewall.Entry) string {
	switch {
	case e.Owner == firewall.OwnerSSH:
		return text.FirewallSSH
	case e.NodeIndex > 0:
		return text.FirewallNode(e.NodeIndex)
	case e.Note != "":
		return text.FirewallManual + " " + e.Note
	}
	return text.FirewallManual
}

func (u *firewallUI) menu() ui.Menu {
	st, err := u.svc.Store.Load()
	if err != nil {
		return ui.Menu{Heading: text.MenuFirewall, Status: []string{u.s.Colors.Bad(text.ActionFailed(err))}}
	}
	if !st.Firewall.Enabled {
		return ui.Menu{
			Heading: text.MenuFirewall,
			Status:  []string{text.StatusFirewall(u.s.Colors.Off(text.FirewallOff))},
			Items:   []ui.Item{{Label: text.FirewallEnable, Hint: text.FirewallEnableHnt, Action: u.enable}},
		}
	}
	status := []string{text.FirewallStatusLine(u.s.Colors.Good(text.FirewallOn), !st.Firewall.BlockPing)}
	if es, err := u.svc.Entries(u.ctx); err != nil {
		status = append(status, u.s.Colors.Bad(text.ActionFailed(err)))
	} else if len(es) == 0 {
		status = append(status, text.FirewallNoRules)
	} else {
		status = append(status, text.FirewallRules)
		status = append(status, ruleLines(es)...)
	}
	if u.svc.BothActive(u.ctx, st) {
		status = append(status, u.s.Colors.Warn(text.FirewallUFWBoth))
	}
	return ui.Menu{
		Heading: text.MenuFirewall,
		Status:  status,
		Items: []ui.Item{
			{Label: text.FirewallDisable, Action: u.disable},
			{Label: text.FirewallReset, Action: u.reset},
		},
	}
}

func (u *firewallUI) confirmed(done string) error {
	kept, err := awaitConfirm(u.ctx, u.s, u.svc.Confirmer, u.reg)
	if kept {
		u.s.Done(done)
	}
	return err
}

func (u *firewallUI) enable() error {
	p := u.svc.Check(u.ctx)
	switch {
	case p.UFW:
		u.s.End(u.s.Colors.Warn(text.FirewallUFWActive))
		return nil
	case p.Firewalld:
		u.s.End(u.s.Colors.Warn(text.FirewallFirewalldActive))
		return nil
	}
	if p.Docker {
		u.s.Println(u.s.Colors.Warn(text.FirewallDocker))
	}
	if p.Foreign.Found() {
		u.s.Println(u.s.Colors.Warn(text.FirewallForeign(p.Foreign.Where)))
	}
	if p.NftMissing {
		u.s.Println(text.FirewallNftInstall)
	}
	es, err := u.svc.Entries(u.ctx)
	if err != nil {
		return err
	}
	u.s.Println(text.FirewallWillOpen)
	for _, l := range ruleLines(es) {
		u.s.Println(l)
	}
	ok, err := u.s.Confirm(text.ConfirmFirewallEnable)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.Enable(u.ctx); err != nil {
		return err
	}
	return u.confirmed(text.FirewallEnabled)
}

func (u *firewallUI) disable() error {
	ok, err := u.s.Confirm(text.ConfirmFirewallDisable)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.Disable(u.ctx); err != nil {
		return err
	}
	return u.confirmed(text.FirewallDisabled)
}

func (u *firewallUI) reset() error {
	ok, err := u.s.Confirm(text.ConfirmFirewallReset)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.Reset(u.ctx); err != nil {
		return err
	}
	return u.confirmed(text.FirewallResetDone)
}

// describeFirewall maps firewall errors to text; ok is false for other
// errors.
func describeFirewall(err error) (string, bool) {
	for target, msg := range map[error]string{
		firewall.ErrUFWActive: text.FirewallUFWActive, firewall.ErrFirewalldActive: text.FirewallFirewalldActive,
		firewall.ErrEnabled: text.ErrFirewallEnabled, firewall.ErrDisabled: text.ErrFirewallDisabled,
	} {
		if errors.Is(err, target) {
			return msg, true
		}
	}
	return "", false
}
