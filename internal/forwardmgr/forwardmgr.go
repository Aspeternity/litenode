// Package forwardmgr implements port forwarding operations (DESIGN
// section 11): adding, modifying, enabling, disabling and deleting rules.
// Each operation loads the current state, checks the input, and applies
// the new state as one change of the "sysctl", "firewall" and "ufw" kinds
// (D-040), so a failure anywhere restores everything.
package forwardmgr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// ErrNotFound means no forward has the given ID.
var ErrNotFound = errors.New("forward not found")

// PortTakenError lists why the local ports cannot be used.
type PortTakenError struct {
	Conflicts []ports.Conflict
}

func (e *PortTakenError) Error() string {
	return fmt.Sprintf("port taken: %v", e.Conflicts)
}

// Service runs forward operations.
type Service struct {
	Runner  system.Runner
	Store   *state.Store
	Changes *change.Manager
	// Resolver looks up domain targets; nil uses net.DefaultResolver.
	Resolver forward.Resolver
	// Executable is the litenode binary the resolve timer runs.
	Executable string
	// UnitDir holds the resolve timer units; empty uses DefaultUnitDir.
	UnitDir string
}

// Kinds are the configuration kinds forward operations change, in apply
// order: kernel forwarding first, then the table, then ufw. The rollback
// subcommand registers the same kinds.
func (s *Service) Kinds() []*change.Kind {
	return []*change.Kind{sysctl.Kind(), firewall.SyncKind(s.Store), ufw.Kind(s.Store)}
}

func (s *Service) resolver() forward.Resolver {
	if s.Resolver != nil {
		return s.Resolver
	}
	return net.DefaultResolver
}

// apply writes the sysctl file for st and applies it with the table and
// the ufw reconciliation, then sets up or removes the resolve timer.
func (s *Service) apply(ctx context.Context, st *state.State) error {
	if slices.ContainsFunc(st.Forwards, func(f forward.Forward) bool { return f.Enabled }) {
		if err := firewall.EnsureNft(ctx, s.Runner); err != nil {
			return err
		}
	}
	env, err := sysctl.ReadEnv(s.Runner)
	if err != nil {
		return err
	}
	f, ok, err := sysctl.ConfigFile(s.Runner, sysctl.Desired(st, env))
	if err != nil {
		return err
	}
	k := s.Kinds()
	var parts []change.Part
	if ok {
		parts = append(parts, change.Part{Kind: k[0], Files: []change.File{f}})
	}
	parts = append(parts, change.Part{Kind: k[1]}, change.Part{Kind: k[2]})
	if _, err := s.Changes.Apply(ctx, change.Change{Parts: parts, State: st}); err != nil {
		return err
	}
	if err := s.syncTimer(ctx, st); err != nil {
		return &TimerError{Err: err}
	}
	return nil
}

// Input is what the add and modify flows collect.
type Input struct {
	Remark     string
	Protocol   string
	ListenPort string
	TargetHost string
	TargetPort string
}

// build makes the rule for in, resolving a domain target (an address in
// use by old stays while the domain still has it).
func (s *Service) build(ctx context.Context, in Input, old forward.Forward) (forward.Forward, error) {
	remark, err := forward.NormalizeRemark(in.Remark)
	if err != nil {
		return forward.Forward{}, err
	}
	f := forward.Forward{
		ID: old.ID, Remark: remark, Enabled: old.Enabled, Protocol: in.Protocol,
		ListenPort: in.ListenPort, TargetHost: in.TargetHost, TargetPort: in.TargetPort,
	}
	if err := forward.CheckFields(f); err != nil {
		return forward.Forward{}, err
	}
	if f.IsDomain() {
		if f.ResolvedIPv4, f.ResolvedIPv6, err = forward.Resolve(ctx, s.resolver(), f.TargetHost, old.ResolvedIPv4, old.ResolvedIPv6); err != nil {
			return forward.Forward{}, err
		}
	}
	if err := forward.Check(f); err != nil {
		return forward.Forward{}, err
	}
	return f, nil
}

// checkPorts fails with *PortTakenError if f's local ports are used by a
// program, a node or another forward (D-037).
func (s *Service) checkPorts(st *state.State, f forward.Forward) error {
	l, err := ports.Listening(s.Runner)
	if err != nil {
		return err
	}
	from, to, err := forward.ParsePorts(f.ListenPort)
	if err != nil {
		return err
	}
	c := ports.Checker{Listening: l, Claims: ports.StateClaims(st.Nodes, st.Forwards)}
	if conflicts := c.Check(ports.Range(f.Protocol, from, to), forward.Owner(f.ID), nil); len(conflicts) > 0 {
		return &PortTakenError{Conflicts: conflicts}
	}
	return nil
}

func index(st *state.State, id int64) int {
	return slices.IndexFunc(st.Forwards, func(f forward.Forward) bool { return f.ID == id })
}

// Add creates an enabled rule.
func (s *Service) Add(ctx context.Context, in Input) (forward.Forward, error) {
	st, err := s.Store.Load()
	if err != nil {
		return forward.Forward{}, err
	}
	f, err := s.build(ctx, in, forward.Forward{Enabled: true})
	if err != nil {
		return forward.Forward{}, err
	}
	if err := s.checkPorts(st, f); err != nil {
		return forward.Forward{}, err
	}
	f.ID = st.AllocID()
	st.Forwards = append(st.Forwards, f)
	return f, s.apply(ctx, st)
}

// Update replaces a rule's settings; its ID and whether it is enabled
// stay.
func (s *Service) Update(ctx context.Context, id int64, in Input) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	i := index(st, id)
	if i < 0 {
		return ErrNotFound
	}
	f, err := s.build(ctx, in, st.Forwards[i])
	if err != nil {
		return err
	}
	if err := s.checkPorts(st, f); err != nil {
		return err
	}
	st.Forwards[i] = f
	return s.apply(ctx, st)
}

// SetEnabled enables or disables a rule. Enabling checks the ports again:
// a program may have started listening on them since.
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	i := index(st, id)
	if i < 0 {
		return ErrNotFound
	}
	if enabled && !st.Forwards[i].Enabled {
		if err := s.checkPorts(st, st.Forwards[i]); err != nil {
			return err
		}
	}
	st.Forwards[i].Enabled = enabled
	return s.apply(ctx, st)
}

// Delete removes a rule and its firewall source limit.
func (s *Service) Delete(ctx context.Context, id int64) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	i := index(st, id)
	if i < 0 {
		return ErrNotFound
	}
	st.Forwards = slices.Delete(st.Forwards, i, i+1)
	delete(st.Firewall.AutoSources, forward.Owner(id))
	return s.apply(ctx, st)
}
