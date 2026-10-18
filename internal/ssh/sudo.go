package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Errors of CreateSudoUser.
var (
	ErrUserName      = errors.New("invalid user name")
	ErrUserExists    = errors.New("user already exists")
	ErrPasswordShort = errors.New("password too short")
	ErrPasswordChars = errors.New("password contains a colon, newline or control character")
)

// MinPasswordLen is the shortest password accepted for a new user.
const MinPasswordLen = 8

// userName is the portable form useradd accepts by default on Debian
// and Ubuntu.
var userName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// ValidUserName checks a new user name.
func ValidUserName(name string) bool { return userName.MatchString(name) }

// CheckPassword checks a password for chpasswd: long enough, and nothing
// that would break its "name:password" lines.
func CheckPassword(p string) error {
	if utf8.RuneCountInString(p) < MinPasswordLen {
		return ErrPasswordShort
	}
	for _, c := range p {
		if c == ':' || c < 0x20 || c == 0x7f {
			return ErrPasswordChars
		}
	}
	return nil
}

// passwordAlphabet leaves out look-alikes (0/O, 1/l/I).
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GeneratePassword returns a random 16-character password (about 93
// bits), shown to the user once.
func GeneratePassword(rand io.Reader) (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := 0; i < 16; i++ {
		n, err := randInt(rand, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(passwordAlphabet[n])
	}
	return b.String(), nil
}

// randInt is crypto/rand.Int for any reader.
func randInt(r io.Reader, max *big.Int) (int, error) {
	var buf [1]byte
	for {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		// Rejection sampling keeps the choice uniform.
		limit := 256 - 256%int(max.Int64())
		if int(buf[0]) < limit {
			return int(buf[0]) % int(max.Int64()), nil
		}
	}
}

// CreateSudoUser creates a user in the sudo group with a password (sudo
// asks for it) and, optionally, public keys (DESIGN section 13). It adds
// a way in and cannot lock anyone out, so it runs without
// confirm-or-rollback; if a step fails, the user is removed again.
func (s *Service) CreateSudoUser(ctx context.Context, name, password string, keys []Key) error {
	if !ValidUserName(name) {
		return ErrUserName
	}
	if err := CheckPassword(password); err != nil {
		return err
	}
	r := s.runner()
	if _, err := LookupAccount(r, name); err == nil {
		return ErrUserExists
	} else if !errors.Is(err, ErrNoAccount) {
		return err
	}
	if _, err := r.Exec(ctx, "useradd", "--create-home", "--shell", "/bin/bash", "--groups", SudoGroup, "--", name); err != nil {
		return fmt.Errorf("useradd %s: %w", name, err)
	}
	undo := func(err error) error {
		r.Exec(ctx, "userdel", "--remove", "--", name)
		return err
	}
	if _, err := r.ExecInput(ctx, []byte(name+":"+password+"\n"), "chpasswd"); err != nil {
		return undo(fmt.Errorf("chpasswd %s: %w", name, err))
	}
	if len(keys) > 0 {
		a, err := LookupAccount(r, name)
		if err != nil {
			return undo(err)
		}
		if _, err := s.AddKeys(ctx, a, keys); err != nil {
			return undo(err)
		}
	}
	return nil
}
