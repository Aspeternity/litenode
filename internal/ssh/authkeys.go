package ssh

import (
	"context"
	"fmt"
	"io/fs"
	"strconv"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/system"
)

// AuthKeysKind is the "authkeys" configuration kind: authorized_keys
// files. LiteNode writes them as root; Apply then gives ~/.ssh and
// authorized_keys back to their user with modes 700 and 600, which sshd
// requires (StrictModes). It does so for every account that can log in,
// so it also repairs files after a restore.
func AuthKeysKind() *change.Kind {
	return &change.Kind{
		Name: "authkeys",
		Apply: func(ctx context.Context, r system.Runner) error {
			accounts, err := LoginAccounts(r)
			if err != nil {
				return err
			}
			for _, a := range accounts {
				if err := fixKeyPermissions(ctx, r, a); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// exists reports whether path is a readable file or directory.
func exists(r system.Runner, path string) bool {
	if _, err := r.ReadFile(path); err == nil {
		return true
	}
	_, err := r.ReadDir(path)
	return err == nil
}

// fixKeyPermissions sets the owner and modes of ~/.ssh and
// authorized_keys if they exist.
func fixKeyPermissions(ctx context.Context, r system.Runner, a Account) error {
	owner := strconv.Itoa(a.UID) + ":" + strconv.Itoa(a.GID)
	for _, p := range []struct {
		path string
		mode fs.FileMode
	}{{a.SSHDir(), 0o700}, {a.AuthorizedKeysPath(), 0o600}} {
		if !exists(r, p.path) {
			continue
		}
		if err := r.Chmod(p.path, p.mode); err != nil {
			return fmt.Errorf("chmod %s: %w", p.path, err)
		}
		if _, err := r.Exec(ctx, "chown", owner, p.path); err != nil {
			return fmt.Errorf("chown %s: %w", p.path, err)
		}
	}
	return nil
}
