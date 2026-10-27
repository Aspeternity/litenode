package app

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/forwardmgr"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// forwardProtocols are the protocol choices, most common first.
var forwardProtocols = []string{forward.TCPUDP, forward.TCP, forward.UDP}

// forwardSteps is the number of steps of the add and modify flows.
const forwardSteps = 5

// forwardUI holds the port forwarding menu (DESIGN section 11). It only
// displays and reads input; forwardmgr.Service does the work.
type forwardUI struct {
	ctx    context.Context
	s      *ui.Session
	svc    *forwardmgr.Service
	egress Egress
}

func (u *forwardUI) state() (*state.State, error) { return u.svc.Store.Load() }

// targetLabel shows a rule's target ("1.2.3.4:443", "[2001:db8::1]:443"),
// marking a target reached over one address family only (D-046).
func targetLabel(f forward.Forward) string {
	l := net.JoinHostPort(f.TargetHost, f.TargetPort)
	switch v4, v6 := f.IPv4(), f.IPv6(); {
	case v4 != "" && v6 == "":
		l += "  " + text.OnlyIPv4
	case v4 == "" && v6 != "":
		l += "  " + text.OnlyIPv6
	}
	return l
}

// forwardRow is a rule's row in the list, without its number.
func (u *forwardUI) forwardRow(f forward.Forward) []string {
	st := u.s.Colors.Off(text.StateDisabled)
	if f.Enabled {
		st = u.s.Colors.Good(text.StateEnabled)
	}
	return []string{f.ListenPort, f.Protocol, targetLabel(f), st, f.Remark}
}

func (u *forwardUI) menu() ui.Menu {
	st, err := u.state()
	if err != nil {
		return ui.Menu{Heading: text.MenuForwards, Status: []string{u.s.Colors.Bad(text.ActionFailed(err))}}
	}
	var status []string
	if len(st.Forwards) == 0 {
		status = []string{text.ForwardNone}
	} else {
		rows := [][]string{{text.ColID, text.ColLocalPort, text.ColProtocol, text.ColTarget, text.ColState, text.ColRemark}}
		for i, f := range st.Forwards {
			rows = append(rows, append([]string{strconv.Itoa(i + 1)}, u.forwardRow(f)...))
		}
		status = ui.Table(rows)
	}
	none := len(st.Forwards) == 0
	return ui.Menu{
		Heading: text.MenuForwards,
		Status:  status,
		Items: []ui.Item{
			{Label: text.ForwardAdd, Action: u.add},
			{Label: text.ForwardModify, Hidden: none, Action: u.modify},
			{Label: text.ForwardToggle, Hidden: none, Action: u.toggle},
			{Label: text.ForwardProbe, Hidden: none, Action: u.probe},
			{Label: text.ForwardDelete, Hidden: none, Action: u.delete},
		},
	}
}

// chooseForward asks for a rule's number in the list; nil means back.
func (u *forwardUI) chooseForward() (*forward.Forward, int, error) {
	st, err := u.state()
	if err != nil || len(st.Forwards) == 0 {
		return nil, 0, err
	}
	i, err := u.s.PromptNumber(text.PromptForwardNumber, -1, 0, len(st.Forwards))
	if err != nil || i == 0 {
		return nil, 0, err
	}
	f := st.Forwards[i-1]
	return &f, i, nil
}

// ask runs the steps of the add and modify flows, starting from cur (the
// rule being modified, or defaults); ok is false when the user goes back.
func (u *forwardUI) ask(cur forward.Forward) (in forwardmgr.Input, ok bool, err error) {
	for {
		u.s.Step(1, forwardSteps, text.PromptListenPort)
		if in.ListenPort, err = u.askPorts(text.PromptListenPort, cur.ListenPort, true); err != nil || in.ListenPort == "" {
			return in, false, err
		}
		u.s.Step(2, forwardSteps, text.PromptForwardProto)
		if in.Protocol, err = u.askProto(cur.Protocol); err != nil || in.Protocol == "" {
			return in, false, err
		}
		// Port conflicts depend on the protocol, so they are checked
		// here and the port is asked again (D-037).
		var pt *forwardmgr.PortTakenError
		err = u.svc.CheckPorts(cur.ID, in.Protocol, in.ListenPort)
		if !errors.As(err, &pt) {
			if err != nil {
				return in, false, err
			}
			break
		}
		for _, c := range pt.Conflicts {
			u.s.Println(u.s.Colors.Warn(text.PortTaken(c.Port, string(c.Proto), c.Owner)))
		}
	}
	u.s.Step(3, forwardSteps, text.PromptTargetHost)
	if in.TargetHost, err = u.askHost(cur.TargetHost); err != nil || in.TargetHost == "" {
		return in, false, err
	}
	u.s.Step(4, forwardSteps, text.PromptTargetPort)
	def := cur.TargetPort
	if def == "" {
		def = in.ListenPort
	}
	for {
		if in.TargetPort, err = u.askPorts(text.PromptTargetPort, def, false); err != nil {
			return in, false, err
		}
		if forward.CheckFields(forward.Forward{Protocol: in.Protocol, ListenPort: in.ListenPort,
			TargetHost: in.TargetHost, TargetPort: in.TargetPort}) == nil {
			break
		}
		u.s.Println(u.s.Colors.Warn(text.ErrTargetPortLength))
	}
	u.s.Step(5, forwardSteps, text.PromptForwardRemark)
	for {
		if cur.Remark != "" {
			in.Remark, err = u.s.PromptDefault(text.PromptForwardRemark, cur.Remark)
		} else {
			in.Remark, err = u.s.Prompt(text.PromptForwardRemark + ": ")
		}
		if err != nil {
			return in, false, err
		}
		if in.Remark, err = forward.NormalizeRemark(in.Remark); err == nil {
			return in, true, nil
		}
		u.s.Println(u.s.Colors.Warn(describe(err)))
	}
}

// askPorts asks for a port or range. With back set, "0" (or an empty
// answer without a default) returns "".
func (u *forwardUI) askPorts(label, def string, back bool) (string, error) {
	for {
		var in string
		var err error
		if def != "" {
			in, err = u.s.PromptDefault(label, def)
		} else {
			in, err = u.s.Prompt(label + ": ")
		}
		if err != nil || (back && (in == "0" || in == "")) {
			return "", err
		}
		if _, _, err := forward.ParsePorts(in); err == nil {
			return in, nil
		}
		u.s.Println(u.s.Colors.Warn(text.InvalidPorts))
	}
}

// askProto asks for a protocol, marking the current one; "" means back.
func (u *forwardUI) askProto(cur string) (string, error) {
	m := ui.Menu{Heading: text.PromptForwardProto}
	for _, p := range forwardProtocols {
		it := ui.Item{Label: text.Protocol(p)}
		if p == cur {
			it.Hint = text.CurrentMark
		}
		m.Items = append(m.Items, it)
	}
	c, err := u.s.Choose(m)
	if err != nil || c == 0 {
		return "", err
	}
	return forwardProtocols[c-1], nil
}

// askHost asks for the target address; "" means back.
func (u *forwardUI) askHost(def string) (string, error) {
	for {
		var in string
		var err error
		if def != "" {
			in, err = u.s.PromptDefault(text.PromptTargetHost, def)
		} else {
			in, err = u.s.Prompt(text.PromptTargetHost + ": ")
		}
		if err != nil || in == "0" || in == "" {
			return "", err
		}
		if share.ValidAddress(in) {
			return in, nil
		}
		u.s.Println(u.s.Colors.Warn(text.ErrTargetHost))
	}
}

// finish closes a flow whose change was applied: err is nil or a
// *forwardmgr.TimerError, which leaves the rules in effect (D-076).
func (u *forwardUI) finish(err error, done string) error {
	var te *forwardmgr.TimerError
	if err != nil && !errors.As(err, &te) {
		return err
	}
	u.s.Done(done)
	if te != nil {
		u.s.Hold()
		u.s.Println(u.s.Colors.Warn(text.ForwardTimerFailed(te.Err)))
	}
	return nil
}

// confirmTarget asks before saving a rule whose target is only reachable
// over a family the machine has no network of its own for (D-134). WARP
// does not count: connections stay within their family (D-046) and WARP
// brings no incoming connections. A target that does not resolve is
// reported by the change itself.
func (u *forwardUI) confirmTarget(in forwardmgr.Input) (bool, error) {
	v4, v6, err := u.svc.TargetFamilies(u.ctx, in)
	if err != nil {
		return true, nil
	}
	eg := u.egress.Status(u.ctx)
	if ipv6, bad := forward.Unreachable(v4, v6, eg.IPv4.Native(), eg.IPv6.Native()); bad {
		return u.s.Confirm(text.ConfirmForwardNoExit(ipv6))
	}
	return true, nil
}

func (u *forwardUI) add() error {
	in, ok, err := u.ask(forward.Forward{})
	if err != nil || !ok {
		return err
	}
	if ok, err := u.confirmTarget(in); err != nil || !ok {
		return err
	}
	u.s.Println(text.ApplyingChange)
	_, err = u.svc.Add(u.ctx, in)
	return u.finish(err, text.ForwardAdded)
}

func (u *forwardUI) modify() error {
	f, _, err := u.chooseForward()
	if err != nil || f == nil {
		return err
	}
	in, ok, err := u.ask(*f)
	if err != nil || !ok {
		return err
	}
	if in == (forwardmgr.Input{Remark: f.Remark, Protocol: f.Protocol, ListenPort: f.ListenPort,
		TargetHost: f.TargetHost, TargetPort: f.TargetPort}) {
		u.s.End(text.ForwardNoChange)
		return nil
	}
	if ok, err := u.confirmTarget(in); err != nil || !ok {
		return err
	}
	u.s.Println(text.ApplyingChange)
	return u.finish(u.svc.Update(u.ctx, f.ID, in), text.ForwardUpdated)
}

func (u *forwardUI) toggle() error {
	f, _, err := u.chooseForward()
	if err != nil || f == nil {
		return err
	}
	done := text.ForwardEnabled
	if f.Enabled {
		done = text.ForwardDisabled
	}
	return u.finish(u.svc.SetEnabled(u.ctx, f.ID, !f.Enabled), done)
}

// probe tests the TCP connection from this machine to the rule's target.
func (u *forwardUI) probe() error {
	f, _, err := u.chooseForward()
	if err != nil || f == nil {
		return err
	}
	if f.Protocol == forward.UDP {
		u.s.End(text.ProbeUDPOnly)
		return nil
	}
	if f.Protocol == forward.TCPUDP {
		u.s.Println(text.ProbeTCPOnly)
	}
	if from, to, err := forward.ParsePorts(f.TargetPort); err == nil && to > from {
		u.s.Println(text.ProbeFirstPort(from))
	}
	u.s.Hold()
	u.s.Println(text.Probing)
	allOK := true
	for _, r := range u.svc.Probe(u.ctx, *f) {
		if r.Err != nil {
			allOK = false
			u.s.Println(u.s.Colors.Warn(text.ProbeFailed(r.Address, text.NetReason(r.Err))))
			continue
		}
		u.s.Println(text.ProbeOK(r.Address, r.Latency.Milliseconds()))
	}
	if allOK {
		u.s.Done(text.ProbeAllOK)
	} else {
		u.s.EndWarn(text.ProbeSomeFails)
	}
	return nil
}

func (u *forwardUI) delete() error {
	f, i, err := u.chooseForward()
	if err != nil || f == nil {
		return err
	}
	ok, err := u.s.Confirm(text.ConfirmDeleteForward(i, f.ListenPort+" → "+net.JoinHostPort(f.TargetHost, f.TargetPort)))
	if err != nil || !ok {
		return err
	}
	return u.finish(u.svc.Delete(u.ctx, f.ID), text.ForwardDeleted)
}

// describeForward maps forward errors to text; ok is false for other
// errors.
func describeForward(err error) (string, bool) {
	for target, msg := range map[error]string{
		forwardmgr.ErrNotFound: text.ErrForwardNotFound, forward.ErrPort: text.InvalidPorts,
		forward.ErrPortLength: text.ErrTargetPortLength, forward.ErrTargetHost: text.ErrTargetHost,
		forward.ErrNoAddresses: text.ErrTargetNoAddresses,
	} {
		if errors.Is(err, target) {
			return msg, true
		}
	}
	var pt *forwardmgr.PortTakenError
	if errors.As(err, &pt) {
		return portConflicts(pt.Conflicts), true
	}
	return "", false
}
