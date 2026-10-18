package ssh

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/system"
)

// PasswdPath is the account database.
const PasswdPath = "/etc/passwd"

// Account is a local user that can log in over SSH.
type Account struct {
	Name     string
	UID, GID int
	Home     string
	Shell    string
}

// SSHDir is the account's ~/.ssh.
func (a Account) SSHDir() string { return path.Join(a.Home, ".ssh") }

// AuthorizedKeysPath is the account's ~/.ssh/authorized_keys.
func (a Account) AuthorizedKeysPath() string { return path.Join(a.SSHDir(), "authorized_keys") }

// ParsePasswd reads /etc/passwd.
func ParsePasswd(data []byte) []Account {
	var out []Account
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, ":")
		if len(f) != 7 {
			continue
		}
		uid, err1 := strconv.Atoi(f[2])
		gid, err2 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, Account{Name: f[0], UID: uid, GID: gid, Home: f[5], Shell: f[6]})
	}
	return out
}

// CanLogIn reports whether the account is root or a regular user with a
// login shell.
func (a Account) CanLogIn() bool {
	if a.UID != 0 && a.UID < 1000 || a.UID == 65534 {
		return false
	}
	base := path.Base(a.Shell)
	return a.Shell != "" && base != "nologin" && base != "false"
}

// ErrNoAccount means there is no such user.
var ErrNoAccount = errors.New("no such user")

// LookupAccount finds a user in /etc/passwd.
func LookupAccount(r system.Runner, name string) (Account, error) {
	data, err := r.ReadFile(PasswdPath)
	if err != nil {
		return Account{}, err
	}
	for _, a := range ParsePasswd(data) {
		if a.Name == name {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("%w: %s", ErrNoAccount, name)
}

// LoginAccounts lists root and the regular users that can log in.
func LoginAccounts(r system.Runner) ([]Account, error) {
	data, err := r.ReadFile(PasswdPath)
	if err != nil {
		return nil, err
	}
	var out []Account
	for _, a := range ParsePasswd(data) {
		if a.CanLogIn() {
			out = append(out, a)
		}
	}
	return out, nil
}
