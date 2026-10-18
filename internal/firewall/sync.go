package firewall

import (
	"bytes"
	"context"
	"fmt"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// SyncKind is the "firewall" configuration kind, part of the changes of
// the features whose ports the firewall opens: nodes, the SSH port and
// uninstalling Xray (M3-04). It has no files: while the firewall is on,
// Apply writes and loads nftables.nft for the state on disk, so after a
// restore the rules follow the restored state, like ufw.Kind. It reads
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
			if !st.Firewall.Enabled {
				return nil
			}
			ports, err := sshPorts(ctx, r, st)
			if err != nil {
				return err
			}
			f, err := nft.ConfigFile(Config(st, ports))
			if err != nil {
				return err
			}
			if old, err := r.ReadFile(f.Path); err == nil && bytes.Equal(old, f.Data) {
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
