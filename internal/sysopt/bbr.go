package sysopt

import (
	"context"
	"fmt"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system"
)

// FallbackCongestion is the congestion control set when BBR is turned
// off: the Linux default.
const FallbackCongestion = "cubic"

// BBR is the BBR state shown in the menu.
type BBR struct {
	// Active is true when BBR is the current congestion control.
	Active bool
	// Managed is true when LiteNode's sysctl file sets BBR.
	Managed bool
}

// BBRStatus reads the running congestion control and the state.
func (s *Service) BBRStatus() (BBR, error) {
	st, err := s.Store.Load()
	if err != nil {
		return BBR{}, err
	}
	return BBR{Active: system.BBREnabled(s.Runner), Managed: st.Sysopt.BBR}, nil
}

// SetBBR turns BBR and fq on or off in /etc/sysctl.d/99-litenode.conf
// and loads the file. The new default qdisc applies to interfaces set up
// later, which BBR does not need (it paces by itself since Linux 4.13).
// Turning it off removes the lines and switches back to cubic, as the
// removed lines no longer apply until reboot.
func (s *Service) SetBBR(ctx context.Context, on bool) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	st.Sysopt.BBR = on
	if err := s.applySysctl(ctx, st); err != nil {
		return err
	}
	if !on && system.BBREnabled(s.Runner) {
		key := "net.ipv4.tcp_congestion_control=" + FallbackCongestion
		if _, err := s.Runner.Exec(ctx, "sysctl", "-w", key); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}

// applySysctl writes the sysctl file for st and loads it, saving st.
func (s *Service) applySysctl(ctx context.Context, st *state.State) error {
	env, err := sysctl.ReadEnv(s.Runner)
	if err != nil {
		return err
	}
	f, ok, err := sysctl.ConfigFile(s.Runner, sysctl.Desired(st, env))
	if err != nil {
		return err
	}
	c := change.Change{State: st}
	if ok {
		c.Parts = []change.Part{{Kind: sysctl.Kind(), Files: []change.File{f}}}
	} else {
		// Nothing to write: the state still changes.
		return s.Store.Save(st)
	}
	_, err = s.Changes.Apply(ctx, c)
	return err
}
