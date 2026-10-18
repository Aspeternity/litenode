package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/state"
)

// Effective is what sshd actually uses, from "sshd -T" (DESIGN section 13:
// the menu shows these, not what LiteNode wrote).
type Effective struct {
	// Ports are the ports sshd listens on: those of its ListenAddress
	// values, or of its Port values when it reports no ListenAddress.
	// sshd listens on every Port line of every file, so there may be
	// several.
	Ports           []int
	PubkeyAuth      bool
	PasswordAuth    bool
	KbdInteractive  bool
	PermitRootLogin string
}

// ParseEffective reads "sshd -T" output.
func ParseEffective(out string) (Effective, error) {
	var e Effective
	var listen []int
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		seen[key] = true
		switch key {
		case "port":
			p, err := strconv.Atoi(value)
			if err != nil {
				return Effective{}, fmt.Errorf("sshd -T port %q: %w", value, err)
			}
			if !slices.Contains(e.Ports, p) {
				e.Ports = append(e.Ports, p)
			}
		case "listenaddress":
			i := strings.LastIndexByte(value, ':')
			p, err := strconv.Atoi(value[i+1:])
			if i < 0 || err != nil {
				return Effective{}, fmt.Errorf("sshd -T listenaddress %q", value)
			}
			if !slices.Contains(listen, p) {
				listen = append(listen, p)
			}
		case "pubkeyauthentication":
			e.PubkeyAuth = value == "yes"
		case "passwordauthentication":
			e.PasswordAuth = value == "yes"
		case "kbdinteractiveauthentication":
			e.KbdInteractive = value == "yes"
		case "permitrootlogin":
			if value == "without-password" { // older name of prohibit-password
				value = RootKeysOnly
			}
			e.PermitRootLogin = value
		}
	}
	for _, k := range []string{"port", "pubkeyauthentication", "passwordauthentication", "permitrootlogin"} {
		if !seen[k] {
			return Effective{}, fmt.Errorf("sshd -T output has no %s", k)
		}
	}
	if len(listen) > 0 {
		e.Ports = listen
	}
	return e, nil
}

// ErrNotEffective means sshd does not use a setting LiteNode wrote,
// usually because another file sets it first.
var ErrNotEffective = errors.New("sshd does not use LiteNode's settings")

// Check compares the managed settings in s with what sshd uses and lists
// each difference.
func Check(s state.SSH, e Effective) error {
	var diff []string
	if s.Port > 0 && !slices.Equal(e.Ports, []int{s.Port}) {
		diff = append(diff, fmt.Sprintf("port %d (sshd: %v)", s.Port, e.Ports))
	}
	if s.PubkeyAuth != nil && *s.PubkeyAuth != e.PubkeyAuth {
		diff = append(diff, fmt.Sprintf("pubkeyauthentication %s", yesNo(*s.PubkeyAuth)))
	}
	if s.PasswordAuth != nil {
		if *s.PasswordAuth != e.PasswordAuth {
			diff = append(diff, fmt.Sprintf("passwordauthentication %s", yesNo(*s.PasswordAuth)))
		}
		if !*s.PasswordAuth && e.KbdInteractive {
			diff = append(diff, "kbdinteractiveauthentication no")
		}
	}
	if s.PermitRootLogin != "" && s.PermitRootLogin != e.PermitRootLogin {
		diff = append(diff, fmt.Sprintf("permitrootlogin %s (sshd: %s)", s.PermitRootLogin, e.PermitRootLogin))
	}
	if len(diff) > 0 {
		return fmt.Errorf("%w: %s", ErrNotEffective, strings.Join(diff, "; "))
	}
	return nil
}
