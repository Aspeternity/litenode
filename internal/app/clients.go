package app

import (
	"errors"
	"strconv"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// clientRef is a client with its node, in the order of the client table.
type clientRef struct {
	node   node.Node
	client node.Client
}

// allClients lists the clients of all nodes, node by node.
func allClients(st *state.State) []clientRef {
	var refs []clientRef
	for _, n := range st.Nodes {
		for _, c := range n.Clients {
			refs = append(refs, clientRef{node: n, client: c})
		}
	}
	return refs
}

// clientTable renders the clients of all nodes; nodes without a client
// get a warning line (D-060).
func (u *nodeUI) clientTable(st *state.State) []string {
	refs := allClients(st)
	var status []string
	switch {
	case len(st.Nodes) == 0:
		return []string{text.ClientNoNodes}
	case len(refs) == 0:
		status = []string{text.ClientNone}
	default:
		rows := [][]string{{text.ColID, text.ColNode, text.ColRemark, text.ColState}}
		for i, r := range refs {
			state := u.s.Colors.Off(text.StateDisabled)
			if r.client.Enabled {
				state = u.s.Colors.Good(text.StateEnabled)
			}
			rows = append(rows, []string{strconv.Itoa(i + 1), r.node.Remark, r.client.Remark, state})
		}
		status = ui.Table(rows)
	}
	for _, n := range st.Nodes {
		if len(n.Clients) == 0 {
			status = append(status, u.s.Colors.Warn(text.NodeWithoutClients(n.Remark)))
		}
	}
	return status
}

// clientsMenu is the client menu of the main menu (DESIGN section 9.4):
// the clients of all nodes in one table.
func (u *nodeUI) clientsMenu() ui.Menu {
	st := u.state()
	none := len(allClients(st)) == 0
	return ui.Menu{
		Heading: text.MenuClients,
		Status:  u.clientTable(st),
		Items: []ui.Item{
			{Label: text.ClientAdd, Hidden: len(st.Nodes) == 0, Action: u.addClientToNode},
			{Label: text.ClientModify, Hidden: none, Action: u.modifyClient},
			{Label: text.ClientToggle, Hidden: none, Action: u.toggleClient},
			{Label: text.ClientDelete, Hidden: none, Action: u.deleteClient},
		},
	}
}

// chooseClient asks for a client's sequence number in the client table.
func (u *nodeUI) chooseClient() (*node.Node, *node.Client, error) {
	refs := allClients(u.state())
	if len(refs) == 0 {
		return nil, nil, nil
	}
	i, err := u.s.PromptNumber(text.PromptClientNumber, -1, 0, len(refs))
	if err != nil || i == 0 {
		return nil, nil, err
	}
	r := refs[i-1]
	return &r.node, &r.client, nil
}

// addClientToNode picks the node for a new client (skipped with only one
// node) and adds the client.
func (u *nodeUI) addClientToNode() error {
	st := u.state()
	switch len(st.Nodes) {
	case 0:
		return nil
	case 1:
		return u.addClient(st.Nodes[0].ID)
	}
	m := ui.Menu{Heading: text.ChooseNodeHeading}
	for _, n := range st.Nodes {
		m.Items = append(m.Items, ui.Item{Label: text.NodeChoice(n.Remark, u.protocolLabel(n), n.Port)})
	}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return err
	}
	return u.addClient(st.Nodes[choice-1].ID)
}

func (u *nodeUI) addClient(id int64) error {
	st := u.state()
	var def string
	for _, n := range st.Nodes {
		if n.ID == id {
			def = nodemgr.DefaultClientRemark(st, n)
		}
	}
	for {
		remark, err := u.s.PromptDefault(text.ClientRemark, def)
		if err != nil {
			return err
		}
		c, err := u.svc.AddClient(u.ctx, id, remark)
		if errors.Is(err, node.ErrRemarkDuplicate) || errors.Is(err, node.ErrRemarkEmpty) ||
			errors.Is(err, node.ErrRemarkTooLong) || errors.Is(err, node.ErrRemarkControl) {
			u.s.Println(u.s.Colors.Warn(describe(err)))
			continue
		}
		if err != nil {
			return err
		}
		u.showClientLink(id, c.ID, text.ClientAdded)
		return nil
	}
}

func (u *nodeUI) modifyClient() error {
	n, c, err := u.chooseClient()
	if err != nil || c == nil {
		return err
	}
	m := ui.Menu{Heading: text.ClientModify, Status: []string{text.ClientLine(c.Remark)},
		Items: []ui.Item{{Label: text.ClientRemark, Hint: text.Current(c.Remark)}, {Label: text.ClientResetUUID}}}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return err
	}
	if choice == 1 {
		for {
			r, err := u.s.PromptDefault(text.ClientRemark, c.Remark)
			if err != nil || r == c.Remark {
				return err
			}
			err = u.svc.SetClientRemark(u.ctx, n.ID, c.ID, r)
			if errors.Is(err, node.ErrRemarkDuplicate) || errors.Is(err, node.ErrRemarkEmpty) ||
				errors.Is(err, node.ErrRemarkTooLong) || errors.Is(err, node.ErrRemarkControl) {
				u.s.Println(u.s.Colors.Warn(describe(err)))
				continue
			}
			if err == nil {
				u.s.Done(text.Done)
			}
			return err
		}
	}
	ok, err := u.s.Confirm(text.ConfirmResetUUID)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.ResetClientUUID(u.ctx, n.ID, c.ID); err != nil {
		return err
	}
	u.showClientLink(n.ID, c.ID, text.ClientUUIDReset)
	return nil
}

func (u *nodeUI) toggleClient() error {
	n, c, err := u.chooseClient()
	if err != nil || c == nil {
		return err
	}
	if err := u.svc.SetClientEnabled(u.ctx, n.ID, c.ID, !c.Enabled); err != nil {
		return err
	}
	if c.Enabled {
		u.s.Done(text.ClientDisabled)
	} else {
		u.s.Done(text.ClientEnabled)
	}
	return nil
}

// showClientLink prints a client's link with its QR code, for the
// address the user picks when there are several (D-068). It closes the
// flow first, with done as success ("" : just the link), so that the
// link and QR code are printed without the gutter.
func (u *nodeUI) showClientLink(nodeID, clientID int64, done string) {
	addr, isDefault, err := u.chooseLinkAddress()
	if done != "" {
		u.s.Done(done)
	}
	if err != nil || addr == "" {
		return
	}
	if done == "" {
		u.s.End(text.LinkBelow)
	}
	for _, n := range u.state().Nodes {
		if n.ID != nodeID {
			continue
		}
		for _, c := range n.Clients {
			if c.ID == clientID {
				u.showLink(n, c, addr, isDefault, true)
			}
		}
	}
}

// deleteClient deletes a client after confirmation. Deleting the last one
// keeps the node, which cannot be used until a client is added (D-060).
func (u *nodeUI) deleteClient() error {
	n, c, err := u.chooseClient()
	if err != nil || c == nil {
		return err
	}
	question := text.ConfirmDeleteClient(c.Remark)
	if len(n.Clients) == 1 {
		question = text.ConfirmDeleteLastClient(c.Remark)
	}
	ok, err := u.s.Confirm(question)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.DeleteClient(u.ctx, n.ID, c.ID); err != nil {
		return err
	}
	u.s.Done(text.ClientDeleted)
	return nil
}
