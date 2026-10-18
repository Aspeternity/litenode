package nft

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Paths and units (DESIGN sections 3 and 11).
const (
	ConfigPath = state.ConfigDir + "/nftables.nft"
	NftPath    = "/usr/sbin/nft"
	UnitName   = "litenode-nft.service"
	UnitPath   = "/etc/systemd/system/" + UnitName
)

// unit loads the table at boot, before the network comes up, like
// Debian's nftables.service. It does not touch /etc/nftables.conf.
const unit = `[Unit]
Description=LiteNode nftables rules
Wants=network-pre.target
Before=network-pre.target shutdown.target
Conflicts=shutdown.target
DefaultDependencies=no

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=` + NftPath + ` -f ` + ConfigPath + `

[Install]
WantedBy=sysinit.target
`

// ConfigFile is the change of nftables.nft for c.
func ConfigFile(c Config) (change.File, error) {
	data, err := Generate(c)
	if err != nil {
		return change.File{}, err
	}
	return change.File{Path: ConfigPath, Data: data, Mode: state.PrivateFileMode}, nil
}

// hasTable reports whether a generated file creates the table (rather
// than only removing it).
func hasTable(data []byte) bool {
	return bytes.Contains(data, []byte("table "+Table+" {"))
}

// Kind is the "nft" configuration kind: nftables.nft, checked with
// "nft -c -f". Apply loads the file on disk, which replaces or removes the
// table, and enables litenode-nft.service only while there is a table, so
// it also does the right thing after a restore.
func Kind() *change.Kind {
	return &change.Kind{
		Name: "nft",
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			if res, err := r.Query(ctx, NftPath, "-c", "-f", paths[ConfigPath]); err != nil {
				return fmt.Errorf("nft rejects the rules: %w (%s)", err, bytes.TrimSpace([]byte(res.Stderr)))
			}
			return nil
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			data, err := r.ReadFile(ConfigPath)
			if errors.Is(err, fs.ErrNotExist) {
				// Restored to before the first rules: remove the table.
				data, _ = Generate(Config{})
				if _, err := r.ExecInput(ctx, data, NftPath, "-f", "-"); err != nil {
					return fmt.Errorf("remove table: %w", err)
				}
				return disableUnit(ctx, r)
			}
			if err != nil {
				return err
			}
			if _, err := r.Exec(ctx, NftPath, "-f", ConfigPath); err != nil {
				return fmt.Errorf("load %s: %w", ConfigPath, err)
			}
			if hasTable(data) {
				return enableUnit(ctx, r)
			}
			return disableUnit(ctx, r)
		},
	}
}

func enableUnit(ctx context.Context, r system.Runner) error {
	if got, err := r.ReadFile(UnitPath); err != nil || string(got) != unit {
		if err := r.WriteFile(UnitPath, []byte(unit), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", UnitPath, err)
		}
		if err := system.DaemonReload(ctx, r); err != nil {
			return err
		}
	}
	if on, _ := system.ServiceEnabled(ctx, r, UnitName); on {
		return nil
	}
	return system.Systemctl(ctx, r, system.ServiceEnable, UnitName)
}

func disableUnit(ctx context.Context, r system.Runner) error {
	if on, _ := system.ServiceEnabled(ctx, r, UnitName); !on {
		return nil
	}
	return system.Systemctl(ctx, r, system.ServiceDisable, UnitName)
}
