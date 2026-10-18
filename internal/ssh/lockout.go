package ssh

import (
	"errors"
	"strconv"
	"strings"
)

// Policy is the login configuration sshd uses (or would use after a
// change).
type Policy struct {
	Pubkey, Password bool
	Root             string // RootYes, RootKeysOnly or RootNo
}

// PolicyOf is the policy in effect.
func PolicyOf(e Effective) Policy {
	return Policy{Pubkey: e.PubkeyAuth, Password: e.PasswordAuth, Root: e.PermitRootLogin}
}

// Admin is root or a sudo user, with what it could log in with.
type Admin struct {
	Name string
	Root bool
	Keys int
	// Password: the account has a usable password (not locked or empty).
	Password bool
}

// CanLogIn reports whether the admin can log in over SSH under p.
func (a Admin) CanLogIn(p Policy) bool {
	key := p.Pubkey && a.Keys > 0
	pass := p.Password && a.Password
	if a.Root {
		switch p.Root {
		case RootNo:
			return false
		case RootKeysOnly:
			return key
		}
	}
	return key || pass
}

// Interlock errors (DESIGN section 13, D-064). Each names what must be
// done first.
var (
	// ErrNeedPasswordLogin: key login cannot be turned off while password
	// login is off.
	ErrNeedPasswordLogin = errors.New("turn on password login before turning off key login")
	// ErrNeedKeyLogin: password login cannot be turned off while key
	// login is off.
	ErrNeedKeyLogin = errors.New("turn on key login before turning off password login")
	// ErrNeedKey: password login cannot be turned off while no admin
	// could log in with a key.
	ErrNeedKey = errors.New("add a key before turning off password login")
	// ErrNeedSudoUser: root login cannot be forbidden while no sudo user
	// can log in (P-003).
	ErrNeedSudoUser = errors.New("create a sudo user that can log in before forbidding root login")
	// ErrLockout: after the change neither root nor a sudo user could log
	// in.
	ErrLockout = errors.New("nobody could log in after this change")
)

// CheckLockout checks a policy change against the interlocks: the
// specific rules of DESIGN section 13 first, then that root or a sudo
// user can still log in at all.
func CheckLockout(old, new Policy, admins []Admin) error {
	anyone := func(filter func(Admin) bool) bool {
		for _, a := range admins {
			if filter(a) && a.CanLogIn(new) {
				return true
			}
		}
		return false
	}
	if old.Pubkey && !new.Pubkey && !new.Password {
		return ErrNeedPasswordLogin
	}
	if old.Password && !new.Password {
		if !new.Pubkey {
			return ErrNeedKeyLogin
		}
		if !anyone(func(Admin) bool { return true }) {
			return ErrNeedKey
		}
	}
	if old.Root != RootNo && new.Root == RootNo && !anyone(func(a Admin) bool { return !a.Root }) {
		return ErrNeedSudoUser
	}
	if !anyone(func(Admin) bool { return true }) {
		return ErrLockout
	}
	return nil
}

// UsablePassword reports whether a password field of /etc/shadow allows
// a password login: not empty, not locked ("!..."), not disabled ("*").
func UsablePassword(field string) bool {
	return field != "" && !strings.HasPrefix(field, "!") && !strings.HasPrefix(field, "*")
}

// ParseShadow maps user names to whether they have a usable password.
func ParseShadow(data []byte) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, ":")
		if len(f) >= 2 && f[0] != "" {
			out[f[0]] = UsablePassword(f[1])
		}
	}
	return out
}

// SudoGroup is the group whose members may use sudo on Debian and Ubuntu.
const SudoGroup = "sudo"

// ParseGroupMembers returns the gid and members of a group in /etc/group.
func ParseGroupMembers(data []byte, group string) (int, []string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, ":")
		if len(f) != 4 || f[0] != group {
			continue
		}
		gid, err := strconv.Atoi(f[2])
		if err != nil {
			return 0, nil, false
		}
		var members []string
		for _, m := range strings.Split(f[3], ",") {
			if m != "" {
				members = append(members, m)
			}
		}
		return gid, members, true
	}
	return 0, nil, false
}
