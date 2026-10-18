package app

import (
	"net"
	"slices"

	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// Share addresses (DESIGN section 9.5, D-068): the server keeps a list of
// addresses links may use, the default first.

func (u *nodeUI) shareAddresses() []string {
	return u.state().Server.ShareAddresses
}

// ensureShareAddresses returns the default address, detecting the
// addresses the first time; only when nothing is detected does it ask.
// "" means the user went back.
func (u *nodeUI) ensureShareAddresses() (string, error) {
	if list := u.shareAddresses(); len(list) > 0 {
		return list[0], nil
	}
	list, err := u.detectAddresses(nil)
	if err != nil || len(list) == 0 {
		return "", err
	}
	return list[0], nil
}

// detectAddresses detects the public addresses, merges them into list and
// saves the result. With nothing detected and an empty list it asks for
// an address.
func (u *nodeUI) detectAddresses(list []string) ([]string, error) {
	u.s.Println(text.Checking)
	detected := u.detector.Detect(u.ctx).Candidates()
	if len(detected) == 0 && len(list) == 0 {
		u.s.Println(u.s.Colors.Warn(text.ShareAddrNone))
		a, err := u.askAddress()
		if err != nil || a == "" {
			return nil, err
		}
		detected = []string{a}
	}
	merged := share.MergeDetected(list, detected)
	if len(merged) > 0 {
		u.s.Println(text.ShareAddrsDetected(merged))
	}
	return merged, u.svc.SetShareAddresses(merged)
}

// askAddress reads an IP or domain; "" means back.
func (u *nodeUI) askAddress() (string, error) {
	for {
		in, err := u.s.Prompt(text.PromptShareAddress + ": ")
		if err != nil || in == "" {
			return "", err
		}
		if share.ValidAddress(in) {
			return in, nil
		}
		u.s.Println(u.s.Colors.Warn(text.ErrInvalidAddress))
	}
}

// chooseLinkAddress picks the address for a link: the only one, or one
// the user chooses. "" means back.
func (u *nodeUI) chooseLinkAddress() (string, bool, error) {
	if _, err := u.ensureShareAddresses(); err != nil {
		return "", false, err
	}
	list := u.shareAddresses()
	switch len(list) {
	case 0:
		return "", false, nil
	case 1:
		return list[0], true, nil
	}
	m := ui.Menu{Heading: text.LinkAddressHeading}
	for i, a := range list {
		m.Items = append(m.Items, ui.Item{Label: a, Hint: addressHint(a, i == 0)})
	}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return "", false, err
	}
	return list[choice-1], choice == 1, nil
}

func addressHint(addr string, isDefault bool) string {
	k := share.AddressKind(addr)
	if k == "" {
		k = text.KindDomain
	}
	if isDefault {
		k += "  " + text.DefaultMark
	}
	return k
}

// shareAddress is the share address page (DESIGN section 9.1).
func (u *nodeUI) shareAddress() error {
	return u.s.Run(func() ui.Menu {
		list := u.shareAddresses()
		var status []string
		if len(list) == 0 {
			status = []string{text.ShareAddrsNone}
		}
		for i, a := range list {
			status = append(status, text.ShareAddrItem(i+1, a, share.AddressKind(a), i == 0))
		}
		return ui.Menu{
			Heading: text.NodeShareAddress,
			Status:  status,
			Items: []ui.Item{
				{Label: text.ShareAddrAdd, Hint: text.ShareAddrAddHint, Action: u.addAddress},
				{Label: text.ShareAddrDefault, Hidden: len(list) < 2, Action: u.setDefaultAddress},
				{Label: text.ShareAddrDetect, Action: func() error {
					list, err := u.detectAddresses(u.shareAddresses())
					if err == nil && len(list) > 0 {
						u.s.Done(text.Done)
					}
					return err
				}},
				{Label: text.ShareAddrDelete, Hidden: len(list) == 0, Action: u.deleteAddress},
			},
		}
	})
}

// addAddress adds an IP or domain. A domain must resolve to this server
// (a CDN cannot carry REALITY); otherwise the user is warned.
func (u *nodeUI) addAddress() error {
	a, err := u.askAddress()
	if err != nil || a == "" {
		return err
	}
	list := u.shareAddresses()
	if slices.Contains(list, a) {
		u.s.End(text.ShareAddrExists)
		return nil
	}
	if share.AddressKind(a) == "" {
		u.s.Println(text.Checking)
		own := append(slices.Clone(list), u.detector.Detect(u.ctx).Candidates()...)
		c, err := share.CheckDomain(u.ctx, u.resolver(), a, own)
		warned := true
		switch {
		case err != nil:
			u.s.Println(u.s.Colors.Warn(text.DomainUnresolved(a)))
		case !c.Here:
			u.s.Println(u.s.Colors.Warn(text.DomainNotHere(a, c.Resolved)))
		default:
			warned = false
		}
		if warned {
			ok, err := u.s.Confirm(text.ConfirmAddAnyway)
			if err != nil || !ok {
				return err
			}
		}
	}
	if err := u.svc.SetShareAddresses(append(list, a)); err != nil {
		return err
	}
	u.s.Done(text.ShareAddrAdded)
	return nil
}

func (u *nodeUI) resolver() share.Resolver {
	if u.dns != nil {
		return u.dns
	}
	return net.DefaultResolver
}

func (u *nodeUI) setDefaultAddress() error {
	list := u.shareAddresses()
	i, err := u.s.PromptNumber(text.PromptAddressNum, -1, 0, len(list))
	if err != nil || i <= 1 {
		return err
	}
	a := list[i-1]
	list = append([]string{a}, slices.Delete(slices.Clone(list), i-1, i)...)
	if err := u.svc.SetShareAddresses(list); err != nil {
		return err
	}
	u.s.Done(text.ShareAddrDefaultOK)
	return nil
}

func (u *nodeUI) deleteAddress() error {
	list := u.shareAddresses()
	i, err := u.s.PromptNumber(text.PromptAddressNum, -1, 0, len(list))
	if err != nil || i == 0 {
		return err
	}
	if err := u.svc.SetShareAddresses(slices.Delete(slices.Clone(list), i-1, i)); err != nil {
		return err
	}
	if i == 1 && len(list) > 1 {
		u.s.Println(text.ShareAddrDefaultOK)
	}
	u.s.Done(text.ShareAddrDeleted)
	return nil
}
