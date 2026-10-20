package sysopt

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system"
)

// FallbackCongestion is the congestion control set when BBR is turned
// off and no other sysctl file sets one: the Linux default.
const FallbackCongestion = "cubic"

const (
	congestionKey  = "net.ipv4.tcp_congestion_control"
	congestionPath = "/proc/sys/net/ipv4/tcp_congestion_control"
)

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
// Turning it off removes the lines, which no longer apply until reboot,
// so the congestion control is set to what the other sysctl files give
// at boot, or cubic when none sets it. keptBy is the file that keeps BBR
// on after turning it off.
func (s *Service) SetBBR(ctx context.Context, on bool) (keptBy string, err error) {
	st, err := s.Store.Load()
	if err != nil {
		return "", err
	}
	st.Sysopt.BBR = on
	if err := s.applySysctl(ctx, st); err != nil {
		return "", err
	}
	if on {
		return "", nil
	}
	want, file, ok, err := sysctl.BootValue(s.Runner, congestionKey)
	if err != nil {
		return "", err
	}
	if !ok {
		want, file = FallbackCongestion, ""
	}
	if data, err := s.Runner.ReadFile(congestionPath); err != nil || strings.TrimSpace(string(data)) != want {
		key := congestionKey + "=" + want
		if _, err := s.Runner.Exec(ctx, "sysctl", "-w", key); err != nil {
			return "", fmt.Errorf("set %s: %w", key, err)
		}
	}
	if want == "bbr" {
		return file, nil
	}
	return "", nil
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
