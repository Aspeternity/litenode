package firewall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// SyncKind is the "firewall" configuration kind, part of the changes of
// the features whose rules live in the LiteNode table: nodes, the SSH
// port and uninstalling Xray open firewall ports (M3-04), forwards add
// DNAT rules (M4-01). It has no files: Apply writes and loads
// nftables.nft for the state on disk, so after a restore the rules follow
// the restored state, like ufw.Kind. While the firewall is on it reads
// the SSH port from sshd -T when the state does not manage it, so in an
// SSH change it comes after the sshd kind.
func SyncKind(store *state.Store) *change.Kind {
	return &change.Kind{
		Name: "firewall",
		Apply: func(ctx context.Context, r system.Runner) error {
			st, err := store.Load()
			if err != nil {
				return err
			}
			var ports []int
			if st.Firewall.Enabled {
				if ports, err = sshPorts(ctx, r, st); err != nil {
					return err
				}
			}
			c := Config(st, ports)
			f, err := nft.ConfigFile(c)
			if err != nil {
				return err
			}
			old, err := r.ReadFile(f.Path)
			switch {
			case err == nil && bytes.Equal(old, f.Data):
				return nil
			case errors.Is(err, fs.ErrNotExist) && c.Empty():
				// Never had a table: nothing to remove.
				return nil
			}
			if res, err := r.ExecInput(ctx, f.Data, nft.NftPath, "-c", "-f", "-"); err != nil {
				return fmt.Errorf("nft rejects the rules: %w (%s)", err, bytes.TrimSpace([]byte(res.Stderr)))
			}
			if err := r.WriteFile(f.Path, f.Data, f.Mode); err != nil {
				return fmt.Errorf("write %s: %w", f.Path, err)
			}
			return nft.Kind().Apply(ctx, r)
		},
	}
}

// sshPorts are the SSH ports the firewall keeps open for st now.
func sshPorts(ctx context.Context, r system.Runner, st *state.State) ([]int, error) {
	if st.SSH.Port > 0 {
		return SSHPorts(st, nil), nil
	}
	e, err := ssh.ReadEffective(ctx, r)
	if err != nil {
		return nil, err
	}
	return SSHPorts(st, e.Ports), nil
}
