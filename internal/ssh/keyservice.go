package ssh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Fetcher downloads a URL into memory (xray.HTTPFetcher).
type Fetcher interface {
	Fetch(ctx context.Context, url string, limit int64) ([]byte, error)
}

// Errors of the key operations.
var (
	// ErrLastKey: password login is off, so deleting the account's last
	// key would lock it out (DESIGN section 13).
	ErrLastKey     = errors.New("cannot delete the last key while password login is off")
	ErrKeyNotFound = errors.New("key not found")
	ErrGitHubUser  = errors.New("invalid GitHub user name")
	ErrGitHubNone  = errors.New("GitHub user has no usable keys")
)

func (s *Service) runner() system.Runner { return s.Confirmer.Manager.Runner }

// Keys lists the account's authorized keys.
func (s *Service) Keys(a Account) ([]Key, error) {
	ak, err := s.readAuthorizedKeys(a)
	if err != nil {
		return nil, err
	}
	return ak.Keys(), nil
}

func (s *Service) readAuthorizedKeys(a Account) (*AuthorizedKeys, error) {
	data, err := s.runner().ReadFile(a.AuthorizedKeysPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return ParseAuthorizedKeys(data), nil
}

// keysChange is the change writing the account's authorized_keys.
func keysChange(a Account, ak *AuthorizedKeys) change.Change {
	return change.Change{Parts: []change.Part{{Kind: AuthKeysKind(), Files: []change.File{
		{Path: a.AuthorizedKeysPath(), Data: ak.Bytes(), Mode: state.PrivateFileMode},
	}}}}
}

// AddKeys adds the keys the account does not have yet and returns how
// many were added. Adding a key cannot lock anyone out, so it is applied
// without confirm-or-rollback.
func (s *Service) AddKeys(ctx context.Context, a Account, keys []Key) (int, error) {
	ak, err := s.readAuthorizedKeys(a)
	if err != nil {
		return 0, err
	}
	n := ak.Add(keys...)
	if n == 0 {
		return 0, nil
	}
	r := s.runner()
	if !exists(r, a.SSHDir()) {
		if err := system.EnsureDir(r, a.SSHDir(), 0o700); err != nil {
			return 0, err
		}
		if _, err := r.Exec(ctx, "chown", strconv.Itoa(a.UID)+":"+strconv.Itoa(a.GID), a.SSHDir()); err != nil {
			return 0, fmt.Errorf("chown %s: %w", a.SSHDir(), err)
		}
	}
	if _, err := s.Confirmer.Manager.Apply(ctx, keysChange(a, ak)); err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteKey removes a key under confirm-or-rollback: the user may be
// logged in with it. While password login is off the account's last key
// cannot be deleted.
func (s *Service) DeleteKey(ctx context.Context, a Account, fingerprint string) error {
	ak, err := s.readAuthorizedKeys(a)
	if err != nil {
		return err
	}
	keys := ak.Keys()
	if !ak.Remove(fingerprint) {
		return ErrKeyNotFound
	}
	if len(keys) == 1 {
		e, err := ReadEffective(ctx, s.runner())
		if err != nil {
			return err
		}
		if !e.PasswordAuth {
			return ErrLastKey
		}
	}
	_, err = s.Confirmer.Apply(ctx, keysChange(a, ak))
	return err
}

// GenerateKey creates an ed25519 key pair, authorizes the public key for
// the account and returns the private key, which is not kept anywhere.
func (s *Service) GenerateKey(ctx context.Context, a Account, comment string) (Key, []byte, error) {
	k, priv, err := GenerateKeyPair(s.Rand, comment)
	if err != nil {
		return Key{}, nil, err
	}
	if _, err := s.AddKeys(ctx, a, []Key{k}); err != nil {
		return Key{}, nil, err
	}
	return k, priv, nil
}

var githubUser = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// GitHubKeysURL is where GitHub publishes a user's public keys.
func GitHubKeysURL(user string) string { return "https://github.com/" + user + ".keys" }

// GitHubKeys downloads a GitHub user's public keys for the user to choose
// from. Keys LiteNode does not accept (e.g. short RSA) are skipped and
// counted.
func (s *Service) GitHubKeys(ctx context.Context, user string) ([]Key, int, error) {
	if !githubUser.MatchString(user) {
		return nil, 0, ErrGitHubUser
	}
	data, err := s.Fetcher.Fetch(ctx, GitHubKeysURL(user), 64<<10)
	if err != nil {
		return nil, 0, err
	}
	var keys []Key
	skipped := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		k, err := ParseKey(line)
		if err != nil {
			skipped++
			continue
		}
		if k.Comment == "" {
			k.Comment = "github:" + user
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, skipped, ErrGitHubNone
	}
	return keys, skipped, nil
}
