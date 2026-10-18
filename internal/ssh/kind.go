package ssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// ConfigFile is the change of the drop-in for s; it is removed when s
// manages nothing.
func ConfigFile(s state.SSH) change.File {
	data := Generate(s)
	if data == nil {
		return change.File{Path: ConfigPath, Remove: true}
	}
	return change.File{Path: ConfigPath, Data: data, Mode: state.PrivateFileMode}
}

// ReadEffective runs "sshd -T".
func ReadEffective(ctx context.Context, r system.Runner) (Effective, error) {
	res, err := r.Query(ctx, SshdPath, "-T")
	if err != nil {
		return Effective{}, fmt.Errorf("sshd -T: %w (%s)", err, strings.TrimSpace(res.Stderr))
	}
	return ParseEffective(res.Stdout)
}

// Kind is the "sshd" configuration kind. sshd cannot check a file
// elsewhere, so the drop-in is validated in place with "sshd -t" before
// sshd reloads. Apply reloads a running sshd and then checks with
// "sshd -T" that sshd uses the settings in the state on disk, so it also
// verifies a restored configuration.
func Kind(store *state.Store) *change.Kind {
	return &change.Kind{
		Name:            "sshd",
		ValidateInPlace: true,
		Validate: func(ctx context.Context, r system.Runner, _ map[string]string) error {
			main, err := r.ReadFile(MainConfigPath)
			if err != nil {
				return err
			}
			if err := CheckInclude(main); err != nil {
				return err
			}
			if res, err := r.Query(ctx, SshdPath, "-t"); err != nil {
				return fmt.Errorf("sshd -t: %w (%s)", err, strings.TrimSpace(res.Stderr))
			}
			return nil
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			st, err := store.Load()
			if err != nil {
				return err
			}
			// Under socket activation (Ubuntu) sshd may not be running; it
			// reads the new configuration when it starts.
			active, err := system.ServiceActive(ctx, r, Unit)
			if err != nil {
				return err
			}
			if active {
				if err := system.Systemctl(ctx, r, system.ServiceReload, Unit); err != nil {
					return err
				}
			}
			e, err := ReadEffective(ctx, r)
			if err != nil {
				return err
			}
			return Check(st.SSH, e)
		},
	}
}
