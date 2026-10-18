package app

import (
	"errors"
	"strconv"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// clients picks a node and shows its client menu (DESIGN section 9.4).
func (u *nodeUI) clients() error {
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
		status := []string{text.ClientsTitle(seq, n.Remark, u.protocolLabel(n), n.Port)}
		rows := [][]string{{text.ColID, text.ColRemark, text.ColState}}
		for i, c := range n.Clients {
			state := u.s.Colors.Off(text.StateDisabled)
			if c.Enabled {
				state = u.s.Colors.Good(text.StateEnabled)
			}
			rows = append(rows, []string{strconv.Itoa(i + 1), c.Remark, state})
		}
		none := len(n.Clients) == 0
		if none {
			status = append(status, text.ClientNone)
		} else {
			status = append(status, ui.Table(rows)...)
		}
		return ui.Menu{
			Heading: text.NodeClients,
			Status:  status,
			Items: []ui.Item{
				{Label: text.ClientAdd, Action: func() error { return u.addClient(id) }},
				{Label: text.ClientModify, Hint: text.ClientModifyHint, Hidden: none, Action: func() error { return u.modifyClient(id) }},
				{Label: text.ClientToggle, Hidden: none, Action: func() error { return u.toggleClient(id) }},
				{Label: text.ClientLink, Hidden: none, Action: func() error { return u.clientLink(id) }},
				{Label: text.ClientDelete, Hidden: none, Action: func() error { return u.deleteClient(id) }},
			},
		}
	})
}

// chooseClient asks for a client's sequence number within node id.
func (u *nodeUI) chooseClient(id int64) (*node.Node, *node.Client, error) {
	for _, n := range u.state().Nodes {
		if n.ID != id {
			continue
		}
		i, err := u.s.PromptNumber(text.PromptClientNumber, -1, 0, len(n.Clients))
		if err != nil || i == 0 {
			return nil, nil, err
		}
		c := n.Clients[i-1]
		return &n, &c, nil
	}
	return nil, nil, nodemgr.ErrNotFound
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
		u.s.Println(u.s.Colors.Good(text.ClientAdded))
		u.showClientLink(id, c.ID)
		return nil
	}
}

func (u *nodeUI) modifyClient(id int64) error {
	n, c, err := u.chooseClient(id)
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
				u.s.Println(text.Done)
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
	u.s.Println(text.ClientUUIDReset)
	u.showClientLink(n.ID, c.ID)
	return nil
}

func (u *nodeUI) toggleClient(id int64) error {
	n, c, err := u.chooseClient(id)
	if err != nil || c == nil {
		return err
	}
	if err := u.svc.SetClientEnabled(u.ctx, n.ID, c.ID, !c.Enabled); err != nil {
		return err
	}
	if c.Enabled {
		u.s.Println(text.ClientDisabled)
	} else {
		u.s.Println(text.ClientEnabled)
	}
	return nil
}

func (u *nodeUI) clientLink(id int64) error {
	_, c, err := u.chooseClient(id)
	if err != nil || c == nil {
		return err
	}
	u.showClientLink(id, c.ID)
	return nil
}

// showClientLink prints a client's link with its QR code.
func (u *nodeUI) showClientLink(nodeID, clientID int64) {
	addr, err := u.ensureShareAddress()
	if err != nil || addr == "" {
		return
	}
	for _, n := range u.state().Nodes {
		if n.ID != nodeID {
			continue
		}
		for _, c := range n.Clients {
			if c.ID == clientID {
				u.showLink(n, c, addr, true)
			}
		}
	}
}

// deleteClient deletes a client after confirmation. Deleting the last one
// keeps the node, which cannot be used until a client is added (D-060).
func (u *nodeUI) deleteClient(id int64) error {
	n, c, err := u.chooseClient(id)
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
	u.s.Println(text.ClientDeleted)
	return nil
}
