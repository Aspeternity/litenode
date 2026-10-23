package ssh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/Aspeternity/litenode/internal/state"
)

// Files read to find who can log in.
const (
	GroupPath  = "/etc/group"
	ShadowPath = "/etc/shadow"
)

// Admins lists root and the sudo users that can log in, with their keys
// and whether they have a usable password.
func (s *Service) Admins(ctx context.Context) ([]Admin, error) {
	r := s.runner()
	accounts, err := LoginAccounts(r)
	if err != nil {
		return nil, err
	}
	group, err := r.ReadFile(GroupPath)
	if err != nil {
		return nil, err
	}
	sudoGID, members, _ := ParseGroupMembers(group, SudoGroup)
	shadow, err := r.ReadFile(ShadowPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	passwords := ParseShadow(shadow)
	var out []Admin
	for _, a := range accounts {
		root := a.UID == 0
		if !root && !slices.Contains(members, a.Name) && a.GID != sudoGID {
			continue
		}
		keys, err := s.Keys(a)
		if err != nil {
			return nil, err
		}
		out = append(out, Admin{Name: a.Name, Root: root, Keys: len(keys), Password: passwords[a.Name]})
	}
	return out, nil
}

// changePolicy checks the interlocks for the policy fn produces from the
// one in effect, then applies the change under confirm-or-rollback.
func (s *Service) changePolicy(ctx context.Context, fnPolicy func(*Policy), fnState func(*state.SSH)) error {
	return s.changePolicyPassword(ctx, fnPolicy, fnState, "", "")
}

// changePolicyPassword is changePolicy that first sets the password of
// admin name (none when name is ""). The interlocks count the new
// password; it is set outside the rollback, so rejecting the change
// restores only the SSH settings (FIX-04).
func (s *Service) changePolicyPassword(ctx context.Context, fnPolicy func(*Policy), fnState func(*state.SSH), name, password string) error {
	e, err := ReadEffective(ctx, s.runner())
	if err != nil {
		return err
	}
	old := PolicyOf(e)
	new := old
	fnPolicy(&new)
	admins, err := s.Admins(ctx)
	if err != nil {
		return err
	}
	if name != "" {
		if err := CheckPassword(password); err != nil {
			return err
		}
		i := slices.IndexFunc(admins, func(a Admin) bool { return a.Name == name })
		if i < 0 {
			return fmt.Errorf("%w: %s", ErrNotAdmin, name)
		}
		admins[i].Password = true
	}
	if err := CheckLockout(old, new, admins); err != nil {
		return err
	}
	if name != "" {
		if _, err := s.runner().ExecInput(ctx, []byte(name+":"+password+"\n"), "chpasswd"); err != nil {
			return fmt.Errorf("chpasswd %s: %w", name, err)
		}
	}
	return s.Change(ctx, fnState)
}

// SetPubkeyAuth turns key login on or off.
func (s *Service) SetPubkeyAuth(ctx context.Context, on bool) error {
	return s.changePolicy(ctx, func(p *Policy) { p.Pubkey = on }, func(c *state.SSH) { c.PubkeyAuth = &on })
}

// SetPasswordAuth turns password login on or off.
func (s *Service) SetPasswordAuth(ctx context.Context, on bool) error {
	return s.changePolicy(ctx, func(p *Policy) { p.Password = on }, func(c *state.SSH) { c.PasswordAuth = &on })
}

// SetLoginMethod sets key and password login together, as one change
// (D-090: key only, password only, or both).
func (s *Service) SetLoginMethod(ctx context.Context, pubkey, password bool) error {
	return s.SetLoginMethodPassword(ctx, pubkey, password, "", "")
}

// SetLoginMethodPassword is SetLoginMethod that first sets the password of
// admin name, for a method with password login (FIX-04, D-126); name ""
// keeps the passwords as they are.
func (s *Service) SetLoginMethodPassword(ctx context.Context, pubkey, password bool, name, newPassword string) error {
	return s.changePolicyPassword(ctx, func(p *Policy) { p.Pubkey, p.Password = pubkey, password },
		func(c *state.SSH) { c.PubkeyAuth, c.PasswordAuth = &pubkey, &password }, name, newPassword)
}

// ErrRootPolicy means an unknown root login policy.
var ErrRootPolicy = errors.New("unknown root login policy")

// SetRootLogin sets the root login policy: RootYes, RootKeysOnly or
// RootNo.
func (s *Service) SetRootLogin(ctx context.Context, policy string) error {
	if policy != RootYes && policy != RootKeysOnly && policy != RootNo {
		return ErrRootPolicy
	}
	return s.changePolicy(ctx, func(p *Policy) { p.Root = policy }, func(c *state.SSH) { c.PermitRootLogin = policy })
}
