package firewall

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// Errors of Enable, Disable and Reset.
var (
	ErrUFWActive       = errors.New("ufw is active")
	ErrFirewalldActive = errors.New("firewalld is active")
	ErrEnabled         = errors.New("firewall already enabled")
	ErrDisabled        = errors.New("firewall not enabled")
)

// Firewalld and Docker are the units the enable checks look for.
const (
	FirewalldUnit = "firewalld.service"
	DockerUnit    = "docker.service"
)

// Service runs the firewall operations (DESIGN section 14). Every change
// goes through Confirmer: a firewall change can cut off SSH.
type Service struct {
	Store     *state.Store
	Confirmer *change.Confirmer
}

func (s *Service) runner() system.Runner { return s.Confirmer.Manager.Runner }

// Kinds are the kinds of a firewall change, in order: the nftables table,
// then the ufw reconciliation (which resumes when the firewall goes off).
func (s *Service) Kinds() []*change.Kind {
	return []*change.Kind{nft.Kind(), ufw.Kind(s.Store)}
}

// Preflight is what the enable step reports before asking (DESIGN section
// 14, enable steps 1 and 2).
type Preflight struct {
	// UFW and Firewalld block enabling until the user stops them.
	UFW, Firewalld bool
	// Docker changes nftables / iptables rules on its own.
	Docker bool
	// Foreign is inbound filtering installed by other software.
	Foreign ForeignFilter
	// NftMissing means the nftables package is installed first.
	NftMissing bool
}

// Check looks at the system before enabling.
func (s *Service) Check(ctx context.Context) Preflight {
	r := s.runner()
	p := Preflight{UFW: ufw.Active(ctx, r), Foreign: DetectForeignFilter(ctx, r), NftMissing: !nftInstalled(ctx, r)}
	p.Firewalld, _ = system.ServiceActive(ctx, r, FirewalldUnit)
	_, err := r.Query(ctx, "systemctl", "cat", DockerUnit)
	p.Docker = err == nil
	return p
}

// EnsureNft installs the nftables package if nft is missing.
func EnsureNft(ctx context.Context, r system.Runner) error {
	if nftInstalled(ctx, r) {
		return nil
	}
	if _, err := r.Exec(ctx, "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "nftables"); err != nil {
		return fmt.Errorf("install nftables: %w", err)
	}
	return nil
}

func nftInstalled(ctx context.Context, r system.Runner) bool {
	_, err := r.Query(ctx, nft.NftPath, "--version")
	return err == nil
}

// SSHPorts are the SSH ports the firewall keeps open now.
func (s *Service) SSHPorts(ctx context.Context, st *state.State) ([]int, error) {
	return sshPorts(ctx, s.runner(), st)
}

// Entries lists the rules the firewall has (or would have once enabled).
func (s *Service) Entries(ctx context.Context) ([]Entry, error) {
	st, err := s.Store.Load()
	if err != nil {
		return nil, err
	}
	ports, err := s.SSHPorts(ctx, st)
	if err != nil {
		return nil, err
	}
	return Entries(st, ports), nil
}

// apply writes and loads the table for st under confirm-or-rollback; the
// caller then asks the user to confirm.
func (s *Service) apply(ctx context.Context, st *state.State) error {
	ports, err := s.SSHPorts(ctx, st)
	if err != nil {
		return err
	}
	return s.write(ctx, st, ports, true)
}

// write writes and loads the table for st, under confirm-or-rollback when
// confirm is set.
func (s *Service) write(ctx context.Context, st *state.State, sshPorts []int, confirm bool) error {
	f, err := nft.ConfigFile(Config(st, sshPorts))
	if err != nil {
		return err
	}
	k := s.Kinds()
	ch := change.Change{
		Parts: []change.Part{{Kind: k[0], Files: []change.File{f}}, {Kind: k[1]}},
		State: st,
	}
	if confirm {
		_, err = s.Confirmer.Apply(ctx, ch)
	} else {
		_, err = s.Confirmer.Manager.Apply(ctx, ch)
	}
	return err
}

// Edit changes the firewall settings with edit and applies them. A change
// to who may reach the SSH ports goes through confirm-or-rollback, and
// Edit reports confirm = true: the caller then asks the user to confirm.
// Other changes (opening a port, ping) only add or remove access to
// other ports and are applied directly (D-073).
func (s *Service) Edit(ctx context.Context, edit func(st *state.State) error) (confirm bool, err error) {
	st, err := s.Store.Load()
	if err != nil {
		return false, err
	}
	if !st.Firewall.Enabled {
		return false, ErrDisabled
	}
	ports, err := s.SSHPorts(ctx, st)
	if err != nil {
		return false, err
	}
	before, err := s.Store.Load()
	if err != nil {
		return false, err
	}
	if err := edit(st); err != nil {
		return false, err
	}
	confirm = AffectsSSH(before, st, ports)
	return confirm, s.write(ctx, st, ports, confirm)
}

// Inspect compares the loaded rules with the state ("check and repair").
func (s *Service) Inspect(ctx context.Context) (Report, error) {
	st, err := s.Store.Load()
	if err != nil {
		return Report{}, err
	}
	ports, err := s.SSHPorts(ctx, st)
	if err != nil {
		return Report{}, err
	}
	want := Config(st, ports).Firewall
	if want == nil {
		return Report{}, ErrDisabled
	}
	var got *nft.Chain
	// nft fails when the table or chain does not exist.
	if res, err := s.runner().Query(ctx, nft.NftPath, append(append([]string{"list", "chain"}, strings.Fields(nft.Table)...), "input")...); err == nil {
		c, err := nft.ParseChain(res.Stdout)
		if err != nil {
			return Report{}, err
		}
		got = &c
	}
	return Compare(want, got)
}

// Repair loads the rules the state calls for again, under
// confirm-or-rollback (it rewrites the SSH rule too).
func (s *Service) Repair(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	if !st.Firewall.Enabled {
		return ErrDisabled
	}
	return s.apply(ctx, st)
}

// Enable turns the firewall on, opening the SSH ports, the nodes' ports
// and the user's rules kept from before. ufw and firewalld must be
// stopped first; nftables is installed if missing. The rules LiteNode
// added to the stopped ufw are deleted first, so they do not linger if
// ufw comes back; reconciliation restores them then (D-038).
func (s *Service) Enable(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	if st.Firewall.Enabled {
		return ErrEnabled
	}
	r := s.runner()
	if ufw.Active(ctx, r) {
		return ErrUFWActive
	}
	if on, _ := system.ServiceActive(ctx, r, FirewalldUnit); on {
		return ErrFirewalldActive
	}
	if err := EnsureNft(ctx, r); err != nil {
		return err
	}
	if err := ufw.Sync(ctx, r, nil); err != nil {
		return fmt.Errorf("remove LiteNode rules from ufw: %w", err)
	}
	st.Firewall.Enabled = true
	return s.apply(ctx, st)
}

// Disable turns the input filter off. The user's rules stay in the state
// for the next Enable. If ufw is running, it is reconciled again.
func (s *Service) Disable(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	if !st.Firewall.Enabled {
		return ErrDisabled
	}
	st.Firewall.Enabled = false
	return s.apply(ctx, st)
}

// Reset deletes the user's rules and the source limits of the automatic
// rules and allows ping again: the firewall is as right after enabling.
func (s *Service) Reset(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	if !st.Firewall.Enabled {
		return ErrDisabled
	}
	st.Firewall = state.Firewall{Enabled: true}
	return s.apply(ctx, st)
}

// BothActive reports whether the LiteNode firewall and ufw are both on,
// so packets must pass both (the main menu warns, DESIGN section 14).
func (s *Service) BothActive(ctx context.Context, st *state.State) bool {
	return st.Firewall.Enabled && ufw.Active(ctx, s.runner())
}
