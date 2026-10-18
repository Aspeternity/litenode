// Package nodemgr implements node operations (DESIGN section 9): adding,
// modifying, enabling, disabling and deleting nodes. Each operation loads
// the current state, checks the input, and applies the new state as one
// change of the "xray" and "ufw" kinds (D-040), so a failure anywhere
// restores everything.
package nodemgr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/security"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/xray"
)

// ErrNotFound means no node has the given ID.
var ErrNotFound = errors.New("node not found")

// XrayInstallError means installing Xray for the first node failed.
type XrayInstallError struct{ Err error }

func (e *XrayInstallError) Error() string { return "install xray: " + e.Err.Error() }
func (e *XrayInstallError) Unwrap() error { return e.Err }

// PortTakenError lists why a port cannot be used.
type PortTakenError struct {
	Conflicts []ports.Conflict
}

func (e *PortTakenError) Error() string {
	return fmt.Sprintf("port taken: %v", e.Conflicts)
}

// Service runs node operations.
type Service struct {
	Runner   system.Runner
	Store    *state.Store
	Changes  *change.Manager
	Registry *node.Registry
	Xray     *xray.Installer
	// Rand is the randomness for keys and IDs (crypto/rand.Reader).
	Rand io.Reader
}

// Kinds are the configuration kinds node operations change, in apply
// order. The rollback subcommand registers the same kinds.
func (s *Service) Kinds() []*change.Kind {
	return []*change.Kind{xray.Kind(s.Store, s.Xray), ufw.Kind(s.Store)}
}

// apply writes the Xray config for st and applies it with the ufw sync.
func (s *Service) apply(ctx context.Context, st *state.State) error {
	f, err := xray.ConfigFile(s.Registry, st.Nodes)
	if err != nil {
		return err
	}
	k := s.Kinds()
	_, err = s.Changes.Apply(ctx, change.Change{
		Parts: []change.Part{{Kind: k[0], Files: []change.File{f}}, {Kind: k[1]}},
		State: st,
	})
	return err
}

// PortChecker returns a checker with the ports in use now.
func (s *Service) PortChecker(st *state.State) (ports.Checker, error) {
	l, err := ports.Listening(s.Runner)
	if err != nil {
		return ports.Checker{}, err
	}
	return ports.Checker{Listening: l, Claims: ports.NodeClaims(st.Nodes)}, nil
}

// checkPort fails with *PortTakenError if port cannot be used by the node
// with the given ID (0 for a new node). A node keeps its own current port.
func (s *Service) checkPort(st *state.State, id int64, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	c, err := s.PortChecker(st)
	if err != nil {
		return err
	}
	owner := ""
	var selfPorts []ports.Key
	if n := find(st, id); n != nil {
		owner = ports.NodeOwner(id)
		if n.Enabled {
			selfPorts = []ports.Key{{Proto: ports.TCP, Port: n.Port}}
		}
	}
	if conflicts := c.Check([]ports.Key{{Proto: ports.TCP, Port: port}}, owner, selfPorts); len(conflicts) > 0 {
		return &PortTakenError{Conflicts: conflicts}
	}
	return nil
}

func find(st *state.State, id int64) *node.Node {
	if id == 0 {
		return nil
	}
	for i := range st.Nodes {
		if st.Nodes[i].ID == id {
			return &st.Nodes[i]
		}
	}
	return nil
}

// AddInput is what the add-node flow collects.
type AddInput struct {
	PresetID string
	Port     int
	// Target is the REALITY target "host:port".
	Target string
	Remark string
}

// Add creates a node from a preset, installing Xray first if needed
// (DESIGN section 9.2). The first client gets the node's remark.
func (s *Service) Add(ctx context.Context, in AddInput) (node.Node, error) {
	st, err := s.Store.Load()
	if err != nil {
		return node.Node{}, err
	}
	p, ok := s.Registry.Preset(in.PresetID)
	if !ok || p.Build == nil {
		return node.Node{}, fmt.Errorf("unknown preset %q", in.PresetID)
	}
	remark, err := node.NormalizeRemark(in.Remark)
	if err != nil {
		return node.Node{}, err
	}
	if err := node.CheckNodeRemark(st.Nodes, remark, 0); err != nil {
		return node.Node{}, err
	}
	if err := node.CheckClientRemark(st.Nodes, remark, 0); err != nil {
		return node.Node{}, err
	}
	if err := s.checkPort(st, 0, in.Port); err != nil {
		return node.Node{}, err
	}
	n, err := p.Build(node.BuildInput{
		NodeID: st.AllocID(), ClientID: st.AllocID(),
		Port: in.Port, Remark: remark, Target: in.Target, Rand: s.Rand,
	})
	if err != nil {
		return node.Node{}, err
	}
	if err := s.ensureXray(ctx, st); err != nil {
		return node.Node{}, err
	}
	st.Nodes = append(st.Nodes, n)
	if err := s.apply(ctx, st); err != nil {
		return node.Node{}, err
	}
	return n, nil
}

// ensureXray installs Xray when the first node is added (DESIGN section 9.2
// step 6) and records the version in st.
func (s *Service) ensureXray(ctx context.Context, st *state.State) error {
	if v, ok := s.Xray.Installed(ctx); ok {
		st.Xray.Version = v
		return nil
	}
	v, err := s.Xray.Install(ctx, "")
	if err != nil {
		return &XrayInstallError{Err: err}
	}
	st.Xray.Version = v
	return nil
}

// modify loads the state, lets fn change node id, and applies the result.
func (s *Service) modify(ctx context.Context, id int64, fn func(st *state.State, n *node.Node) error) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	n := find(st, id)
	if n == nil {
		return ErrNotFound
	}
	if err := fn(st, n); err != nil {
		return err
	}
	return s.apply(ctx, st)
}

// SetPort moves a node to another port (links change).
func (s *Service) SetPort(ctx context.Context, id int64, port int) error {
	return s.modify(ctx, id, func(st *state.State, n *node.Node) error {
		if err := s.checkPort(st, id, port); err != nil {
			return err
		}
		n.Port = port
		return nil
	})
}

// SetTarget changes the REALITY target and SNI (links change).
func (s *Service) SetTarget(ctx context.Context, id int64, target string) error {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("invalid target %q", target)
	}
	return s.modify(ctx, id, func(_ *state.State, n *node.Node) error {
		if n.Reality == nil {
			return errors.New("node has no reality settings")
		}
		n.Reality.Target = target
		n.Reality.ServerNames = []string{host}
		return nil
	})
}

// SetFallbackLimit changes the REALITY fallback throttle (links unchanged).
func (s *Service) SetFallbackLimit(ctx context.Context, id int64, l node.FallbackLimit) error {
	return s.modify(ctx, id, func(_ *state.State, n *node.Node) error {
		if n.Reality == nil {
			return errors.New("node has no reality settings")
		}
		n.Reality.FallbackLimit = l
		return nil
	})
}

// RegenerateKeys replaces the key pair and shortId (links change).
func (s *Service) RegenerateKeys(ctx context.Context, id int64) error {
	return s.modify(ctx, id, func(_ *state.State, n *node.Node) error {
		if n.Reality == nil {
			return errors.New("node has no reality settings")
		}
		priv, pub, err := security.NewX25519(s.Rand)
		if err != nil {
			return err
		}
		sid, err := security.NewShortID(s.Rand)
		if err != nil {
			return err
		}
		n.Reality.PrivateKey, n.Reality.PublicKey, n.Reality.ShortIDs = priv, pub, []string{sid}
		return nil
	})
}

// SetSharePort sets the port put in links (0: the listening port, D-043).
func (s *Service) SetSharePort(ctx context.Context, id int64, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	return s.modify(ctx, id, func(_ *state.State, n *node.Node) error {
		n.SharePort = port
		return nil
	})
}

// SetRemark renames a node. Client remarks are not changed.
func (s *Service) SetRemark(ctx context.Context, id int64, remark string) error {
	remark, err := node.NormalizeRemark(remark)
	if err != nil {
		return err
	}
	return s.modify(ctx, id, func(st *state.State, n *node.Node) error {
		if err := node.CheckNodeRemark(st.Nodes, remark, id); err != nil {
			return err
		}
		n.Remark = remark
		return nil
	})
}

// SetEnabled enables or disables a node. Enabling checks the port again,
// since another program may have taken it meanwhile.
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return s.modify(ctx, id, func(st *state.State, n *node.Node) error {
		if enabled && !n.Enabled {
			if err := s.checkPort(st, id, n.Port); err != nil {
				return err
			}
		}
		n.Enabled = enabled
		return nil
	})
}

// Delete removes a node and its clients.
func (s *Service) Delete(ctx context.Context, id int64) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(st.Nodes, func(n node.Node) bool { return n.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	st.Nodes = slices.Delete(st.Nodes, i, i+1)
	return s.apply(ctx, st)
}

// SetShareAddresses stores the addresses links may use, the default
// first (D-068). Only the state changes, no system configuration.
func (s *Service) SetShareAddresses(list []string) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	st.Server.ShareAddresses = list
	return s.Store.Save(st)
}

// SyncUFW reconciles ufw with the state (when the menu opens: the user
// may have enabled ufw since the last change, D-038).
func (s *Service) SyncUFW(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	return ufw.Reconcile(ctx, s.Runner, ufw.Desired(st), false)
}
