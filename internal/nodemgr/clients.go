package nodemgr

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
)

// ErrLastClient means the client is the node's only one; a node needs at
// least one client (DESIGN section 9.4).
var ErrLastClient = errors.New("a node needs at least one client")

// ErrClientNotFound means the node has no client with the given ID.
var ErrClientNotFound = errors.New("client not found")

func findClient(n *node.Node, id int64) *node.Client {
	for i := range n.Clients {
		if n.Clients[i].ID == id {
			return &n.Clients[i]
		}
	}
	return nil
}

// DefaultClientRemark is "<node remark>-<n>", unique across all clients
// (D-056).
func DefaultClientRemark(st *state.State, n node.Node) string {
	for i := len(n.Clients) + 1; ; i++ {
		r := n.Remark + "-" + strconv.Itoa(i)
		if len([]rune(r)) > node.MaxRemarkLen {
			r = string([]rune(r)[:node.MaxRemarkLen-len(strconv.Itoa(i))-1]) + "-" + strconv.Itoa(i)
		}
		if node.CheckClientRemark(st.Nodes, r, 0) == nil {
			return r
		}
	}
}

// AddClient adds a client with a new UUID and the same flow as the node's
// existing clients (D-056).
func (s *Service) AddClient(ctx context.Context, nodeID int64, remark string) (node.Client, error) {
	remark, err := node.NormalizeRemark(remark)
	if err != nil {
		return node.Client{}, err
	}
	var added node.Client
	err = s.modify(ctx, nodeID, func(st *state.State, n *node.Node) error {
		if err := node.CheckClientRemark(st.Nodes, remark, 0); err != nil {
			return err
		}
		uuid, err := node.NewUUID(s.Rand)
		if err != nil {
			return err
		}
		flow := ""
		if len(n.Clients) > 0 {
			flow = n.Clients[0].Flow
		}
		added = node.Client{ID: st.AllocID(), Remark: remark, UUID: uuid, Flow: flow, Enabled: true}
		n.Clients = append(n.Clients, added)
		return nil
	})
	return added, err
}

// modifyClient lets fn change one client of a node and applies the result.
func (s *Service) modifyClient(ctx context.Context, nodeID, clientID int64, fn func(st *state.State, c *node.Client) error) error {
	return s.modify(ctx, nodeID, func(st *state.State, n *node.Node) error {
		c := findClient(n, clientID)
		if c == nil {
			return ErrClientNotFound
		}
		return fn(st, c)
	})
}

// SetClientRemark renames a client (its Xray email); links keep working
// except for the name shown in the client app.
func (s *Service) SetClientRemark(ctx context.Context, nodeID, clientID int64, remark string) error {
	remark, err := node.NormalizeRemark(remark)
	if err != nil {
		return err
	}
	return s.modifyClient(ctx, nodeID, clientID, func(st *state.State, c *node.Client) error {
		if err := node.CheckClientRemark(st.Nodes, remark, clientID); err != nil {
			return err
		}
		c.Remark = remark
		return nil
	})
}

// ResetClientUUID gives a client a new UUID: only this client's link stops
// working, e.g. after it leaked (DESIGN section 9.4).
func (s *Service) ResetClientUUID(ctx context.Context, nodeID, clientID int64) error {
	return s.modifyClient(ctx, nodeID, clientID, func(_ *state.State, c *node.Client) error {
		uuid, err := node.NewUUID(s.Rand)
		if err != nil {
			return err
		}
		c.UUID = uuid
		return nil
	})
}

// SetClientEnabled enables or disables a client.
func (s *Service) SetClientEnabled(ctx context.Context, nodeID, clientID int64, enabled bool) error {
	return s.modifyClient(ctx, nodeID, clientID, func(_ *state.State, c *node.Client) error {
		c.Enabled = enabled
		return nil
	})
}

// DeleteClient removes a client. The last client of a node cannot be
// deleted on its own (ErrLastClient); delete the node instead.
func (s *Service) DeleteClient(ctx context.Context, nodeID, clientID int64) error {
	return s.modify(ctx, nodeID, func(_ *state.State, n *node.Node) error {
		i := slices.IndexFunc(n.Clients, func(c node.Client) bool { return c.ID == clientID })
		if i < 0 {
			return ErrClientNotFound
		}
		if len(n.Clients) == 1 {
			return ErrLastClient
		}
		n.Clients = slices.Delete(n.Clients, i, i+1)
		return nil
	})
}
