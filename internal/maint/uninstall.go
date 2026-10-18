package maint

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// Kept names a setting both uninstalls leave in place (DESIGN section 16),
// with the file that holds it.
type Kept struct {
	// What is one of the Kept* constants.
	What string
	Path string
}

// Settings that stay after uninstalling.
const (
	KeptSSH      = "ssh"
	KeptUFWSSH   = "ufw-ssh"
	KeptFirewall = "firewall"
	KeptSysctl   = "sysctl"
	KeptSwap     = "swap"
	KeptDNS      = "dns"
	KeptGai      = "gai"
	KeptLog      = "log"
)

// UninstallProgram removes the program and the lnode link only (DESIGN
// section 16): Xray, forwards, the firewall, the state and ufw rules stay,
// and installing again resumes managing them. The resolve timer is brought
// up to date first, so it skips its runs while the program is missing.
func (s *Service) UninstallProgram(ctx context.Context) ([]Kept, error) {
	st, err := s.Store.Load()
	if err != nil {
		return nil, err
	}
	if err := s.Forwards.SyncTimer(ctx, st); err != nil {
		return nil, err
	}
	if err := s.removeProgram(ctx); err != nil {
		return nil, err
	}
	return s.keptSettings(st), nil
}

// UninstallAll removes Xray, all nodes and clients, forwards, LiteNode's
// units and program, and everything under /etc/litenode except the
// firewall rules while the firewall is on (DESIGN section 16, P-007). SSH,
// firewall, sysctl, swap, DNS and gai.conf settings stay; the returned
// list says where they are.
func (s *Service) UninstallAll(ctx context.Context) ([]Kept, error) {
	st, err := s.Store.Load()
	if err != nil {
		return nil, err
	}
	// One change removes the nodes and forwards: Xray stops, the table
	// keeps only the firewall, forwarding sysctls go.
	next := *st
	next.Nodes, next.Forwards = nil, []forward.Forward{}
	next.Xray.Stopped = false
	next.Firewall.AutoSources = maps.Clone(st.Firewall.AutoSources)
	maps.DeleteFunc(next.Firewall.AutoSources, func(owner string, _ []string) bool { return owner != sshOwner })
	parts, err := s.parts(ctx, &next)
	if err != nil {
		return nil, err
	}
	if _, err := s.Confirmer.Manager.Apply(ctx, change.Change{Parts: parts, State: &next}); err != nil {
		return nil, err
	}
	r := s.Runner
	var errs []error
	if err := s.Forwards.SyncTimer(ctx, &next); err != nil {
		errs = append(errs, err)
	}
	// ufw keeps litenode:ssh (D-019, D-038), also while ufw is off.
	if err := deleteUFWRules(ctx, r); err != nil {
		errs = append(errs, err)
	}
	if err := s.Nodes.Xray.Uninstall(ctx); err != nil {
		errs = append(errs, err)
	}
	if !next.Firewall.Enabled {
		// The change removed the table and disabled the unit.
		for _, p := range []string{nft.UnitPath, nft.ConfigPath} {
			if err := r.Remove(p); err != nil {
				errs = append(errs, fmt.Errorf("remove %s: %w", p, err))
			}
		}
	}
	bootUnit := filepath.Join(change.DefaultBootUnitDir, change.BootUnit)
	if _, err := r.ReadFile(bootUnit); err == nil {
		_ = system.Systemctl(ctx, r, system.ServiceDisable, change.BootUnit)
		if err := r.Remove(bootUnit); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", bootUnit, err))
		}
	}
	if err := system.DaemonReload(ctx, r); err != nil {
		errs = append(errs, err)
	}
	if err := s.removeConfigDir(next.Firewall.Enabled); err != nil {
		errs = append(errs, err)
	}
	if err := s.removeProgram(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return s.keptSettings(&next), &PartialError{Err: err}
	}
	return s.keptSettings(&next), nil
}

// PartialError means the full uninstall failed after it began removing
// files; LiteNode cannot continue. Other errors of UninstallAll changed
// nothing.
type PartialError struct{ Err error }

func (e *PartialError) Error() string { return "uninstall incomplete: " + e.Err.Error() }
func (e *PartialError) Unwrap() error { return e.Err }

// deleteUFWRules deletes the node and forward rules LiteNode added to ufw.
func deleteUFWRules(ctx context.Context, r system.Runner) error {
	current, err := ufw.Current(r)
	if err != nil {
		return err
	}
	var errs []error
	for _, c := range current {
		if !strings.HasPrefix(c.Comment, ufw.Marker+"node:") && !strings.HasPrefix(c.Comment, ufw.Marker+"forward:") {
			continue
		}
		if _, err := r.Exec(ctx, "ufw", ufw.Args(c, false)...); err != nil {
			errs = append(errs, fmt.Errorf("delete ufw rule %s: %w", c.Comment, err))
		}
	}
	return errors.Join(errs...)
}

// removeConfigDir deletes /etc/litenode, keeping nftables.nft (and so the
// directory) when the firewall stays on.
func (s *Service) removeConfigDir(keepFirewall bool) error {
	r := s.Runner
	if !keepFirewall {
		if err := r.RemoveAll(state.ConfigDir); err != nil {
			return fmt.Errorf("remove %s: %w", state.ConfigDir, err)
		}
		return nil
	}
	names, err := r.ReadDir(state.ConfigDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", state.ConfigDir, err)
	}
	var errs []error
	for _, n := range names {
		p := filepath.Join(state.ConfigDir, n)
		if p == nft.ConfigPath {
			continue
		}
		if err := r.RemoveAll(p); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}

// removeProgram deletes the program, its update backup and the lnode link
// (only if it points to litenode).
func (s *Service) removeProgram(ctx context.Context) error {
	r := s.Runner
	var errs []error
	if res, err := r.Query(ctx, "readlink", LinkPath); err == nil {
		if t := strings.TrimSpace(res.Stdout); t == "litenode" || t == BinPath {
			if err := r.Remove(LinkPath); err != nil {
				errs = append(errs, fmt.Errorf("remove %s: %w", LinkPath, err))
			}
		}
	}
	for _, p := range []string{BakPath, BinPath} {
		if err := r.Remove(p); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}

// keptSettings lists the settings LiteNode made that stay on the system.
func (s *Service) keptSettings(st *state.State) []Kept {
	r := s.Runner
	exists := func(p string) bool {
		_, err := r.ReadFile(p)
		return err == nil
	}
	var out []Kept
	if exists(ssh.ConfigPath) {
		out = append(out, Kept{KeptSSH, ssh.ConfigPath})
	}
	if rules, err := ufw.Current(r); err == nil {
		for _, c := range rules {
			if c.Comment == ufw.SSHComment {
				out = append(out, Kept{KeptUFWSSH, ufw.UserRules})
				break
			}
		}
	}
	if st.Firewall.Enabled {
		out = append(out, Kept{KeptFirewall, nft.ConfigPath})
	}
	if exists(sysctl.ConfigPath) {
		out = append(out, Kept{KeptSysctl, sysctl.ConfigPath})
	}
	if swapKept(r) {
		out = append(out, Kept{KeptSwap, sysopt.SwapPath})
	}
	if exists(sysopt.ResolvedDropIn) {
		out = append(out, Kept{KeptDNS, sysopt.ResolvedDropIn})
	}
	if data, err := r.ReadFile(sysopt.GaiPath); err == nil && sysopt.ParseGai(data).Managed {
		out = append(out, Kept{KeptGai, sysopt.GaiPath})
	}
	// The log may be large: look for it rather than read it.
	if names, err := r.ReadDir(filepath.Dir(system.DefaultLogPath)); err == nil && slices.Contains(names, filepath.Base(system.DefaultLogPath)) {
		out = append(out, Kept{KeptLog, system.DefaultLogPath})
	}
	return out
}

// swapKept reports whether LiteNode's swap file is in use or mounted at
// boot. The file itself is not read: it is large.
func swapKept(r system.Runner) bool {
	for _, p := range []string{"/proc/swaps", "/etc/fstab"} {
		if data, err := r.ReadFile(p); err == nil && strings.Contains(string(data), sysopt.SwapPath) {
			return true
		}
	}
	return false
}
