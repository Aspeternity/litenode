package app

import (
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// shareMenu is the share menu of the main menu (DESIGN section 9.5): the
// links of all clients and the host addresses they use.
func (u *nodeUI) shareMenu() ui.Menu {
	st := u.state()
	status := []string{text.ShareAddressLine(st.Server.DefaultShareAddress(), len(st.Server.ShareAddresses))}
	status = append(status, u.clientTable(st)...)
	none := len(allClients(st)) == 0
	return ui.Menu{
		Heading: text.MenuShare,
		Status:  status,
		Items: []ui.Item{
			{Label: text.ShareLinkQR, Hidden: none, Action: u.viewLink},
			{Label: text.ShareAllLinks, Hidden: none, Action: u.allLinks},
			{Label: text.MenuHostAddress, Page: true, Action: u.shareAddress},
		},
	}
}

// viewLink shows one client's link and QR code; with only one client it
// does not ask which.
func (u *nodeUI) viewLink() error {
	refs := allClients(u.state())
	var r clientRef
	switch len(refs) {
	case 0:
		return nil
	case 1:
		r = refs[0]
	default:
		n, c, err := u.chooseClient()
		if err != nil || c == nil {
			return err
		}
		r = clientRef{node: *n, client: *c}
	}
	if !r.node.Enabled || !r.client.Enabled {
		u.s.Println(u.s.Colors.Warn(text.ShareClientOff))
	}
	u.showClientLink(r.node.ID, r.client.ID, "")
	return nil
}

// allLinks prints the links of all enabled clients of enabled nodes,
// without QR codes, for the address the user picks.
func (u *nodeUI) allLinks() error {
	addr, isDefault, err := u.chooseLinkAddress()
	if err != nil || addr == "" {
		return err
	}
	var lines []string
	for _, r := range allClients(u.state()) {
		if !r.node.Enabled || !r.client.Enabled {
			continue
		}
		link, err := share.LinkFor(u.svc.Registry, r.node, r.client, addr, isDefault)
		if err != nil {
			return err
		}
		lines = append(lines, "", r.node.Remark+" / "+r.client.Remark+":", link)
	}
	if len(lines) == 0 {
		u.s.End(text.ShareAllNone)
		return nil
	}
	u.s.End(text.LinkBelow)
	u.s.Println("\n" + text.AllLinksTitle)
	for _, l := range lines[1:] {
		u.s.Println(l)
	}
	return nil
}
