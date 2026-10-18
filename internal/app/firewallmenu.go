package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/state"
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
		rows = append(rows, []string{"  [" + strconv.Itoa(i+1) + "]", portLabel(e), sourcesLabel(e.Sources), ownerLabel(e)})
	}
	return ui.Table(rows)
}

// portLabel shows a rule's ports and protocol ("443/tcp", "30001").
func portLabel(e firewall.Entry) string {
	if e.Proto == "tcp+udp" {
		return e.Ports
	}
	return e.Ports + "/" + e.Proto
}

func sourcesLabel(sources []string) string {
	if len(sources) == 0 {
		return text.FirewallAnySource
	}
	return strings.Join(sources, ",")
}

func ownerLabel(e firewall.Entry) string {
	switch {
	case e.Owner == firewall.OwnerSSH:
		return text.FirewallSSH
	case e.NodeIndex > 0:
		return text.FirewallNode(e.NodeIndex)
	case e.ForwardIndex > 0:
		return text.FirewallForward(e.ForwardIndex)
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
			Items:   []ui.Item{{Label: text.FirewallEnable, Action: u.enable}},
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
			{Label: text.FirewallOpen, Action: u.open},
			{Label: text.FirewallModify, Page: true, Action: u.modify},
			{Label: text.FirewallPing, Action: u.togglePing},
			{Label: text.FirewallCheck, Action: u.check},
			{Label: text.FirewallDelete, Action: u.deleteRule},
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
		firewall.ErrRuleExists: text.ErrRuleExists, firewall.ErrNoRule: text.ErrNoRule,
		firewall.ErrInvalidPorts: text.InvalidPorts, firewall.ErrInvalidSource: text.InvalidSources,
	} {
		if errors.Is(err, target) {
			return msg, true
		}
	}
	return "", false
}

// finish reports a change made by Edit: confirmed by the user when it
// touched SSH access, done otherwise.
func (u *firewallUI) finish(confirm bool, done string) error {
	if confirm {
		return u.confirmed(done)
	}
	u.s.Done(done)
	return nil
}

func (u *firewallUI) edit(done string, f func(st *state.State) error) error {
	confirm, err := u.svc.Edit(u.ctx, f)
	if err != nil {
		return err
	}
	return u.finish(confirm, done)
}

// promptPorts asks for a port or range; "" means back.
func (u *firewallUI) promptPorts(def string) (string, error) {
	for {
		var in string
		var err error
		if def != "" {
			in, err = u.s.PromptDefault(text.PromptFirewallPorts, def)
		} else {
			in, err = u.s.Prompt(text.PromptFirewallPorts + ": ")
		}
		if err != nil || in == "0" {
			return "", err
		}
		if firewall.CheckPorts(in) == nil {
			return in, nil
		}
		u.s.Println(u.s.Colors.Warn(text.InvalidPorts))
	}
}

// promptProto asks for a protocol; "" means back.
func (u *firewallUI) promptProto() (string, error) {
	m := ui.Menu{Heading: text.PromptFirewallProto}
	for _, p := range firewall.Protocols {
		m.Items = append(m.Items, ui.Item{Label: text.Protocol(p)})
	}
	c, err := u.s.Choose(m)
	if err != nil || c == 0 {
		return "", err
	}
	return firewall.Protocols[c-1], nil
}

// promptSources asks for source addresses; ok is false when the user goes
// back. Empty input means anyone.
func (u *firewallUI) promptSources(cur []string) (sources []string, ok bool, err error) {
	if len(cur) > 0 {
		u.s.Println(text.Current(sourcesLabel(cur)))
	}
	for {
		in, err := u.s.Prompt(text.PromptSources + ": ")
		if err != nil || in == "0" {
			return nil, false, err
		}
		if sources, err := firewall.ParseSources(in); err == nil {
			return sources, true, nil
		}
		u.s.Println(u.s.Colors.Warn(text.InvalidSources))
	}
}

func (u *firewallUI) open() error {
	ports, err := u.promptPorts("")
	if err != nil || ports == "" {
		return err
	}
	proto, err := u.promptProto()
	if err != nil || proto == "" {
		return err
	}
	if st, err := u.svc.Store.Load(); err != nil || firewall.Duplicate(st, ports, proto, 0) {
		if err == nil {
			err = firewall.ErrRuleExists
		}
		return err
	}
	sources, ok, err := u.promptSources(nil)
	if err != nil || !ok {
		return err
	}
	note, err := u.s.Prompt(text.PromptRuleNote + ": ")
	if err != nil {
		return err
	}
	r := state.FirewallRule{Port: ports, Protocol: proto, Sources: sources, Note: note}
	return u.edit(text.FirewallRuleAdded, func(st *state.State) error { return firewall.AddRule(st, r) })
}

// chooseRule asks for a rule's number in the list; nil means back.
func (u *firewallUI) chooseRule() (*firewall.Entry, int, error) {
	es, err := u.svc.Entries(u.ctx)
	if err != nil || len(es) == 0 {
		return nil, 0, err
	}
	i, err := u.s.PromptNumber(text.PromptRuleNumber, -1, 0, len(es))
	if err != nil || i == 0 {
		return nil, 0, err
	}
	return &es[i-1], i, nil
}

// findEntry is the current entry of owner, for the modify page.
func (u *firewallUI) findEntry(owner string) (firewall.Entry, int, error) {
	es, err := u.svc.Entries(u.ctx)
	if err != nil {
		return firewall.Entry{}, 0, err
	}
	for i, e := range es {
		if e.Owner == owner {
			return e, i + 1, nil
		}
	}
	return firewall.Entry{}, 0, firewall.ErrNoRule
}

// modify is the modify-rule page: ports, protocol, sources and note of a
// manual rule; only the sources of an automatic rule (P-004).
func (u *firewallUI) modify() error {
	chosen, _, err := u.chooseRule()
	if err != nil || chosen == nil {
		return err
	}
	owner := chosen.Owner
	return u.s.Run(func() ui.Menu {
		e, i, err := u.findEntry(owner)
		if err != nil {
			return ui.Menu{Heading: text.FirewallModify, Status: []string{u.s.Colors.Bad(describe(err))}}
		}
		status := ruleLines([]firewall.Entry{e})
		status[0] = strings.Replace(status[0], "[1]", "["+strconv.Itoa(i)+"]", 1)
		sources := ui.Item{Label: text.ModRuleSources, Hint: text.Current(sourcesLabel(e.Sources)), Action: func() error { return u.modSources(owner) }}
		if !firewall.IsManual(owner) {
			status = append(status, text.FirewallAutoSources(ownerLabel(e)))
			return ui.Menu{Heading: text.FirewallModify, Status: status, Items: []ui.Item{sources}}
		}
		return ui.Menu{
			Heading: text.FirewallModify,
			Status:  status,
			Items: []ui.Item{
				{Label: text.ModRulePorts, Hint: text.Current(e.Ports), Action: func() error { return u.modRule(owner, true) }},
				{Label: text.ModRuleProto, Hint: text.Current(text.Protocol(e.Proto)), Action: func() error { return u.modRule(owner, false) }},
				sources,
				{Label: text.ModRuleNote, Hint: text.Current(e.Note), Action: func() error { return u.modNote(owner) }},
			},
		}
	})
}

// manualRule is the stored manual rule of owner.
func (u *firewallUI) manualRule(owner string) (state.FirewallRule, error) {
	st, err := u.svc.Store.Load()
	if err != nil {
		return state.FirewallRule{}, err
	}
	for _, r := range st.Firewall.Rules {
		if firewall.ManualOwner(r.ID) == owner {
			return r, nil
		}
	}
	return state.FirewallRule{}, firewall.ErrNoRule
}

// modRule changes a manual rule's ports (ports set) or protocol.
func (u *firewallUI) modRule(owner string, ports bool) error {
	r, err := u.manualRule(owner)
	if err != nil {
		return err
	}
	if ports {
		p, err := u.promptPorts(r.Port)
		if err != nil || p == "" || p == r.Port {
			return err
		}
		r.Port = p
	} else {
		p, err := u.promptProto()
		if err != nil || p == "" || p == r.Protocol {
			return err
		}
		r.Protocol = p
	}
	return u.edit(text.FirewallRuleUpdated, func(st *state.State) error { return firewall.UpdateRule(st, owner, r) })
}

func (u *firewallUI) modNote(owner string) error {
	r, err := u.manualRule(owner)
	if err != nil {
		return err
	}
	note, err := u.s.PromptDefault(text.PromptRuleNote, r.Note)
	if err != nil || note == r.Note {
		return err
	}
	r.Note = note
	return u.edit(text.FirewallRuleUpdated, func(st *state.State) error { return firewall.UpdateRule(st, owner, r) })
}

func (u *firewallUI) modSources(owner string) error {
	e, _, err := u.findEntry(owner)
	if err != nil {
		return err
	}
	sources, ok, err := u.promptSources(e.Sources)
	if err != nil || !ok {
		return err
	}
	return u.edit(text.FirewallRuleUpdated, func(st *state.State) error { return firewall.SetSources(st, owner, sources) })
}

func (u *firewallUI) togglePing() error {
	st, err := u.svc.Store.Load()
	if err != nil {
		return err
	}
	block := !st.Firewall.BlockPing
	done := text.FirewallPingAllowed
	if block {
		done = text.FirewallPingBlocked
	}
	return u.edit(done, func(st *state.State) error {
		st.Firewall.BlockPing = block
		return nil
	})
}

func (u *firewallUI) check() error {
	rep, err := u.svc.Inspect(u.ctx)
	if err != nil {
		return err
	}
	if rep.OK() {
		u.s.Done(text.FirewallCheckOK)
		return nil
	}
	if rep.NotLoaded {
		u.s.Println(u.s.Colors.Warn(text.FirewallNotLoaded))
	}
	if rep.BaseDiffers {
		u.s.Println(u.s.Colors.Warn(text.FirewallBaseDiffers))
	}
	label := u.ownerLabels()
	for _, o := range rep.Missing {
		u.s.Println(u.s.Colors.Warn(text.FirewallMissing(label(o))))
	}
	for _, o := range rep.Extra {
		u.s.Println(u.s.Colors.Warn(text.FirewallExtra(label(o))))
	}
	for _, o := range rep.Changed {
		u.s.Println(u.s.Colors.Warn(text.FirewallChanged(label(o))))
	}
	ok, err := u.s.Confirm(text.ConfirmRepair)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.Repair(u.ctx); err != nil {
		return err
	}
	return u.confirmed(text.FirewallRepaired)
}

// ownerLabels labels owners for the check report: ports and owner label for
// rules in the list, the owner itself for rules the state no longer has.
func (u *firewallUI) ownerLabels() func(string) string {
	es, _ := u.svc.Entries(u.ctx)
	return func(owner string) string {
		var ports []string
		label := owner
		for _, e := range es {
			if e.Owner == owner {
				ports = append(ports, portLabel(e))
				label = ownerLabel(e)
			}
		}
		if len(ports) == 0 {
			return label
		}
		return strings.Join(ports, ",") + " " + label
	}
}

func (u *firewallUI) deleteRule() error {
	e, i, err := u.chooseRule()
	if err != nil || e == nil {
		return err
	}
	if !firewall.IsManual(e.Owner) {
		u.s.End(u.s.Colors.Warn(text.FirewallAutoRule(ownerLabel(*e), autoRuleMenu(e.Owner))))
		return nil
	}
	ok, err := u.s.Confirm(text.ConfirmDeleteRule(i, portLabel(*e)))
	if err != nil || !ok {
		return err
	}
	owner := e.Owner
	return u.edit(text.FirewallRuleDeleted, func(st *state.State) error { return firewall.DeleteRule(st, owner) })
}

// autoRuleMenu is the menu where an automatic rule is changed.
func autoRuleMenu(owner string) string {
	switch {
	case owner == firewall.OwnerSSH:
		return text.FirewallInSSH
	case firewall.IsForward(owner):
		return text.FirewallInForwards
	}
	return text.FirewallInNodes
}
