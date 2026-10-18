package ssh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Errors of the account operations.
var (
	ErrNotAdmin   = errors.New("not root or a sudo user")
	ErrDeleteRoot = errors.New("root cannot be deleted")
	ErrUserInUse  = errors.New("user has running processes")
	// ErrDeleteLockout: after deleting the user neither root nor another
	// sudo user could log in.
	ErrDeleteLockout = errors.New("nobody could log in after deleting the user")
)

// PasswordRestoreName is the file, under the state directory, holding the
// password hash a password change restores (D-100). It exists only while
// a change is being made or restored.
const PasswordRestoreName = "password-restore"

func passwordRestorePath(store *state.Store) string {
	return filepath.Join(filepath.Dir(store.Path), PasswordRestoreName)
}

// ShadowField returns the password field of name in /etc/shadow.
func ShadowField(data []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, ":")
		if len(f) >= 2 && f[0] == name {
			return f[1], true
		}
	}
	return "", false
}

// PasswordKind is the "password" configuration kind. Its file holds
// "name:hash" lines; Apply gives each user that hash with "chpasswd -e"
// (the hash goes through standard input) where it differs, then removes
// the file. A password change backs up the file holding the old hash,
// so restoring the backup puts the old password back (D-100).
func PasswordKind(store *state.Store) *change.Kind {
	path := passwordRestorePath(store)
	return &change.Kind{
		Name: "password",
		Apply: func(ctx context.Context, r system.Runner) error {
			data, err := r.ReadFile(path)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			shadow, err := r.ReadFile(ShadowPath)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				name, hash, ok := strings.Cut(line, ":")
				if !ok || name == "" {
					continue
				}
				if cur, found := ShadowField(shadow, name); found && cur == hash {
					continue
				}
				if _, err := r.ExecInput(ctx, []byte(line+"\n"), "chpasswd", "-e"); err != nil {
					return fmt.Errorf("chpasswd -e %s: %w", name, err)
				}
			}
			return r.Remove(path)
		},
	}
}

// ChangePassword sets the password of root or a sudo user under
// confirm-or-rollback: if the user does not keep it, the old password
// hash is restored (D-096). The password goes to chpasswd through
// standard input only.
func (s *Service) ChangePassword(ctx context.Context, name, password string) error {
	if err := CheckPassword(password); err != nil {
		return err
	}
	admins, err := s.Admins(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(admins, func(a Admin) bool { return a.Name == name }) {
		return fmt.Errorf("%w: %s", ErrNotAdmin, name)
	}
	r := s.runner()
	shadow, err := r.ReadFile(ShadowPath)
	if err != nil {
		return err
	}
	old, ok := ShadowField(shadow, name)
	if !ok {
		return fmt.Errorf("%w: %s in %s", ErrNoAccount, name, ShadowPath)
	}
	// The change backs up the restore file as it is now, with the old
	// hash; the change itself leaves the password alone (Apply finds the
	// hash unchanged) and the new one is set once the rollback is armed.
	kind := PasswordKind(s.Store)
	restore := []byte(name + ":" + old + "\n")
	path := passwordRestorePath(s.Store)
	if err := r.WriteFile(path, restore, state.PrivateFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := s.Confirmer.Apply(ctx, change.Change{Parts: []change.Part{{Kind: kind,
		Files: []change.File{{Path: path, Data: restore, Mode: state.PrivateFileMode}}}}}); err != nil {
		r.Remove(path) // left behind if the change failed before its restore
		return err
	}
	if _, err := r.ExecInput(ctx, []byte(name+":"+password+"\n"), "chpasswd"); err != nil {
		err = fmt.Errorf("chpasswd %s: %w", name, err)
		if rerr := s.Confirmer.Revert(ctx, change.Registry{kind.Name: kind}); rerr != nil {
			return errors.Join(err, rerr)
		}
		return err
	}
	return nil
}

// SudoUsers lists the sudo users that can log in (the admins but root).
func (s *Service) SudoUsers(ctx context.Context) ([]Admin, error) {
	admins, err := s.Admins(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(admins, func(a Admin) bool { return a.Root }), nil
}

// DeleteSudoUser removes a sudo user with its home directory and mail
// spool (userdel --remove). It is refused for root, for a user with
// running processes (e.g. logged in, which includes the session running
// LiteNode through sudo), and when afterwards neither root nor another
// sudo user could log in under the policy sshd uses (D-096).
func (s *Service) DeleteSudoUser(ctx context.Context, name string) error {
	admins, err := s.Admins(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(admins, func(a Admin) bool { return a.Name == name })
	switch {
	case i < 0:
		return fmt.Errorf("%w: %s", ErrNotAdmin, name)
	case admins[i].Root:
		return ErrDeleteRoot
	}
	r := s.runner()
	e, err := ReadEffective(ctx, r)
	if err != nil {
		return err
	}
	p := PolicyOf(e)
	if CheckLockout(p, p, slices.Delete(slices.Clone(admins), i, i+1)) != nil {
		return ErrDeleteLockout
	}
	// pgrep exits 0 when it finds a process.
	if _, err := r.Query(ctx, "pgrep", "-u", name); err == nil {
		return fmt.Errorf("%w: %s", ErrUserInUse, name)
	}
	if _, err := r.Exec(ctx, "userdel", "--remove", "--", name); err != nil {
		return fmt.Errorf("userdel %s: %w", name, err)
	}
	return nil
}
