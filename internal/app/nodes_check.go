package app

import (
	"slices"
	"time"

	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodecheck"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// now is the clock of the connectivity check (replaced in tests).
var now = time.Now

// check runs the node connectivity check (DESIGN section 9.9).
func (u *nodeUI) check() error {
	n, err := u.chooseNode()
	if err != nil || n == nil {
		return err
	}
	switch {
	case !n.Enabled:
		u.s.EndWarn(text.NodeCheckDisabled)
		return nil
	case !slices.ContainsFunc(n.Clients, func(c node.Client) bool { return c.Enabled }):
		u.s.EndWarn(text.NodeCheckNoClient)
		return nil
	case !u.svc.XrayStatus(u.ctx).Active:
		u.s.EndWarn(text.NodeCheckXrayDown)
		return nil
	}
	r := u.svc.Runner
	if !firewall.NftInstalled(u.ctx, r) {
		ok, err := u.s.Confirm(text.ConfirmNodeCheckNft)
		if err != nil || !ok {
			return err
		}
		if err := firewall.EnsureNft(u.ctx, r); err != nil {
			return err
		}
	}
	ps := nodecheck.NodePorts(*n)
	var names []string
	for _, p := range ps {
		names = append(names, p.Proto)
	}
	if err := nodecheck.Start(u.ctx, r, ps); err != nil {
		return err
	}
	// The table goes away however the check ends; a leftover one (the
	// program killed) is removed at the next start.
	defer nodecheck.Stop(u.ctx, r)
	tr := nodecheck.NewTracker(names, now())
	var snap nodecheck.Snapshot
	read := func() error {
		c, err := nodecheck.Read(u.ctx, r)
		if err != nil {
			return err
		}
		snap = tr.Update(c, now())
		return nil
	}
	u.s.Println(text.NodeCheckIntro)
	end, err := u.s.Watch(u.ctx, time.Second, text.NodeCheckPressEnter, func() ([]string, bool, error) {
		if err := read(); err != nil {
			return nil, false, err
		}
		return u.checkLines(*n, snap), snap.Done, nil
	})
	if err != nil {
		return err
	}
	switch end {
	case ui.WatchCancel:
		u.s.End(text.NodeCheckCanceled)
		return nil
	case ui.WatchEnter:
		// Piped input never refreshed; either way conclude on the latest
		// counts.
		if err := read(); err != nil {
			return err
		}
	}
	if snap.TimedOut {
		u.s.Println(u.s.Colors.Warn(text.NodeCheckTimedOut))
	}
	protos := nodecheck.Final(snap)
	var env nodecheck.Env
	if nodecheck.AnyBlocked(protos) {
		env = nodecheck.DetectEnv(u.ctx, r, u.state().Firewall.Enabled)
	}
	u.s.Println()
	u.s.Println(text.NodeCheckConclusion)
	sources := false
	for i, f := range nodecheck.Conclude(protos, env) {
		u.printFinding(*n, protos[i], f)
		sources = sources || len(f.Sources) > 0
	}
	if sources {
		u.s.Println(text.NodeCheckScanners)
	}
	u.s.End(text.NodeCheckFinished)
	return nil
}

// checkLines is the live display of the check.
func (u *nodeUI) checkLines(n node.Node, s nodecheck.Snapshot) []string {
	lines := []string{text.NodeCheckHeading(n.Remark, n.Port)}
	for _, p := range s.Protos {
		lines = append(lines, text.NodeCheckRow(p.Name, p.Count.Arrived, p.Count.Passed, u.statusLabel(p.Status)))
	}
	lines = append(lines, text.NodeCheckElapsed(int(s.Elapsed/time.Second)))
	for _, h := range s.Hints {
		switch h {
		case nodecheck.HintUDP:
			lines = append(lines, u.s.Colors.Warn(text.NodeCheckHintUDP))
		case nodecheck.HintNoPackets:
			lines = append(lines, u.s.Colors.Warn(text.NodeCheckHintNone))
		}
	}
	return lines
}

func (u *nodeUI) statusLabel(s nodecheck.Status) string {
	l := text.NodeCheckStatus[s]
	switch s {
	case nodecheck.Open:
		return u.s.Colors.Good(l)
	case nodecheck.Blocked, nodecheck.Unreached:
		return u.s.Colors.Warn(l)
	}
	return l
}

func (u *nodeUI) printFinding(n node.Node, p nodecheck.Proto, f nodecheck.Finding) {
	u.s.Println(text.NodeCheckResult(p.Name, u.statusLabel(p.Status), p.Count.Arrived, p.Count.Passed))
	for _, r := range f.Reasons {
		switch r {
		case nodecheck.ReasonOpen:
			u.s.Println(text.NodeCheckOpenNote)
		case nodecheck.ReasonBlockedUFW:
			u.s.Println(text.NodeCheckUFW)
		case nodecheck.ReasonBlockedFirewall:
			u.s.Println(text.NodeCheckFirewall)
		case nodecheck.ReasonBlockedOther:
			u.s.Println(text.NodeCheckOther(f.Foreign))
		case nodecheck.ReasonUnreached:
			u.s.Println(text.NodeCheckUnreached(p.Name, n.Port))
		case nodecheck.ReasonUnreachedUDP:
			u.s.Println(text.NodeCheckUDPCauses)
		}
	}
	if len(f.Sources) > 0 {
		var addrs []string
		for _, a := range f.Sources[:min(len(f.Sources), 5)] {
			addrs = append(addrs, a.String())
		}
		u.s.Println(text.NodeCheckSources(addrs))
	}
}
