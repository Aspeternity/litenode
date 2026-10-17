package xray

import (
	"context"
	"fmt"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Running reports whether, according to the state, Xray should run: there
// is an enabled node with an enabled client and the user has not stopped
// it (D-035, D-045).
func Running(st *state.State) bool {
	if st.Xray.Stopped {
		return false
	}
	for _, n := range st.Nodes {
		if n.Enabled && hasEnabledClient(n) {
			return true
		}
	}
	return false
}

// Kind is the "xray" configuration kind: xray.json, validated with
// "xray run -test". Apply acts on the state on disk, so it also does the
// right thing after a restore: it runs Xray when nodes call for it and
// stops it (and disables start at boot) otherwise.
func Kind(store *state.Store, in *Installer) *change.Kind {
	return &change.Kind{
		Name: "xray",
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			res, err := r.Query(ctx, BinPath, "run", "-test", "-c", paths[ConfigPath])
			if err != nil {
				return fmt.Errorf("xray rejects the config: %w (%s)", err, lastLine(res.Stdout))
			}
			return nil
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			st, err := store.Load()
			if err != nil {
				return err
			}
			if !Running(st) {
				if _, ok := in.Installed(ctx); !ok {
					return nil // nothing installed, nothing to stop
				}
				if err := system.Systemctl(ctx, r, system.ServiceStop, UnitName); err != nil {
					return err
				}
				return system.Systemctl(ctx, r, system.ServiceDisable, UnitName)
			}
			if err := system.Systemctl(ctx, r, system.ServiceEnable, UnitName); err != nil {
				return err
			}
			return in.Restart(ctx)
		},
	}
}

// ConfigFile is the xray.json change for the given nodes: readable by the
// service group only (D-034).
func ConfigFile(reg *node.Registry, nodes []node.Node) (change.File, error) {
	data, err := GenerateConfig(reg, nodes)
	if err != nil {
		return change.File{}, err
	}
	return change.File{Path: ConfigPath, Data: data, Mode: state.GroupFileMode, Group: User}, nil
}

func lastLine(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return s[i+1:]
		}
	}
	return s
}
