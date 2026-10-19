package app

import (
	"errors"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// modify is the modify-node menu (DESIGN section 9.3).
func (u *nodeUI) modify() error {
	chosen, err := u.chooseNode()
	if err != nil || chosen == nil {
		return err
	}
	id := chosen.ID
	return u.s.Run(func() ui.Menu {
		st := u.state()
		seq, n := 0, node.Node{}
		for i, x := range st.Nodes {
			if x.ID == id {
				seq, n = i+1, x
			}
		}
		target, limit := "", node.FallbackLimit{}
		if n.Reality != nil {
			target, limit = n.Reality.Target, n.Reality.FallbackLimit
		}
		items := []ui.Item{{Label: text.ModPort, Hint: text.Current(n.Port), Action: func() error { return u.modPort(id) }}}
		// Target and fallback limit exist only for REALITY (DESIGN
		// section 9.7).
		if n.Reality != nil {
			items = append(items,
				ui.Item{Label: text.ModTarget, Hint: text.Current(target), Action: func() error { return u.modTarget(id, target) }},
				ui.Item{Label: text.ModFallback, Hint: text.Current(text.FallbackDescribe(limit.AfterBytes, limit.BytesPerSec)),
					Action: func() error { return u.modFallback(id, target) }})
		}
		items = append(items,
			ui.Item{Label: text.ModRegenKeys, Action: func() error { return u.modRegenKeys(id) }},
			ui.Item{Label: text.ModSharePort, Hint: text.SharePortCurrent(n.SharePort), Action: func() error { return u.modSharePort(id, n.SharePort) }},
			ui.Item{Label: text.ModRemark, Hint: text.Current(n.Remark), Action: func() error { return u.modRemark(id, n.Remark) }},
		)
		return ui.Menu{
			Heading: text.NodeModify,
			Status:  []string{text.ModifyTitle(seq, n.Remark, u.protocolLabel(n))},
			Items:   items,
		}
	})
}

// confirmLinks warns that all client links of the node will change.
func (u *nodeUI) confirmLinks() (bool, error) {
	return u.s.Confirm(text.LinksWillBreak)
}

func (u *nodeUI) modPort(id int64) error {
	ok, err := u.confirmLinks()
	if err != nil || !ok {
		return err
	}
	cur, reality := 0, false
	for _, n := range u.state().Nodes {
		if n.ID == id {
			cur, reality = n.Port, n.Reality != nil
		}
	}
	for {
		port, err := u.s.PromptNumber(text.PromptPort, cur, 1, 65535)
		if err != nil {
			return err
		}
		if port == cur {
			return nil
		}
		err = u.svc.SetPort(u.ctx, id, port)
		var pt *nodemgr.PortTakenError
		if errors.As(err, &pt) {
			u.s.Println(u.s.Colors.Warn(describe(err)))
			continue
		}
		if err != nil {
			return err
		}
		if reality && port != 443 {
			u.s.Println(u.s.Colors.Warn(text.Non443Hint))
		}
		u.s.Done(text.PortSet(port))
		u.printLinks(id)
		return nil
	}
}

func (u *nodeUI) modTarget(id int64, current string) error {
	// Check the current target first, to help diagnose a failing one.
	if current != "" {
		u.s.Println(text.Checking)
		u.printCheck(u.checker.Check(u.ctx, current))
	}
	target, err := u.chooseTarget(0, 0)
	if err != nil || target == "" {
		return err
	}
	ok, err := u.confirmLinks()
	if err != nil || !ok {
		return err
	}
	if err := u.svc.SetTarget(u.ctx, id, target); err != nil {
		return err
	}
	u.s.Done(text.TargetSet(target))
	u.printLinks(id)
	return nil
}

func (u *nodeUI) modFallback(id int64, target string) error {
	if target != "" {
		u.s.Println(text.Checking)
		r := u.checker.Check(u.ctx, target)
		u.s.Println(text.HomepageSize(r.HomepageSize))
	}
	names := map[string]string{"default": text.TierDefault, "loose": text.TierLoose, "strict": text.TierStrict}
	m := ui.Menu{Heading: text.ModFallback}
	for _, t := range node.FallbackTiers {
		m.Items = append(m.Items, ui.Item{Label: text.FallbackTier(names[t.ID], t.Limit.AfterBytes, t.Limit.BytesPerSec, t.Limit.BurstBytesPerSec)})
	}
	m.Items = append(m.Items, ui.Item{Label: text.TierCustom}, ui.Item{Label: text.TierOff})
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return err
	}
	var l node.FallbackLimit
	switch {
	case choice <= len(node.FallbackTiers):
		l = node.FallbackTiers[choice-1].Limit
	case choice == len(node.FallbackTiers)+1:
		after, err := u.s.PromptNumber(text.PromptAfterMB, 10, 0, 100000)
		if err != nil {
			return err
		}
		rate, err := u.s.PromptNumber(text.PromptRateKB, 1024, 1, 10000000)
		if err != nil {
			return err
		}
		burst, err := u.s.PromptNumber(text.PromptBurstKB, max(rate, 5120), rate, 10000000)
		if err != nil {
			return err
		}
		l = node.FallbackLimit{AfterBytes: int64(after) << 20, BytesPerSec: int64(rate) << 10, BurstBytesPerSec: int64(burst) << 10}
	}
	if err := u.svc.SetFallbackLimit(u.ctx, id, l); err != nil {
		return err
	}
	u.s.Done(text.Done)
	return nil
}

func (u *nodeUI) modRegenKeys(id int64) error {
	ok, err := u.confirmLinks()
	if err != nil || !ok {
		return err
	}
	if err := u.svc.RegenerateKeys(u.ctx, id); err != nil {
		return err
	}
	u.s.Done(text.KeysRegenerated)
	u.printLinks(id)
	return nil
}

func (u *nodeUI) modSharePort(id int64, current int) error {
	port, err := u.s.PromptNumber(text.PromptSharePort, current, 0, 65535)
	if err != nil || port == current {
		return err
	}
	ok, err := u.confirmLinks()
	if err != nil || !ok {
		return err
	}
	if err := u.svc.SetSharePort(u.ctx, id, port); err != nil {
		return err
	}
	u.s.Done(text.SharePortSet)
	u.printLinks(id)
	return nil
}

func (u *nodeUI) modRemark(id int64, current string) error {
	r, err := u.askRemark(current, id, false)
	if err != nil || r == current {
		return err
	}
	if err := u.svc.SetRemark(u.ctx, id, r); err != nil {
		return err
	}
	u.s.Done(text.Done)
	return nil
}
