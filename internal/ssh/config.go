package ssh

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/state"
)

const (
	// MainConfigPath is sshd's main configuration, which LiteNode never
	// changes.
	MainConfigPath = "/etc/ssh/sshd_config"
	// ConfigPath is LiteNode's drop-in. sshd uses the first value it reads
	// for most settings, and cloud images ship 50-cloud-init.conf, so the
	// name sorts first (DESIGN section 13).
	ConfigPath = "/etc/ssh/sshd_config.d/00-litenode.conf"
	// SshdPath is the sshd binary on Debian and Ubuntu.
	SshdPath = "/usr/sbin/sshd"
	// Unit is the sshd service on Debian and Ubuntu (sshd.service is an
	// alias on Debian).
	Unit = "ssh.service"
)

// Root login policies (PermitRootLogin).
const (
	RootYes      = "yes"
	RootKeysOnly = "prohibit-password"
	RootNo       = "no"
)

// managedKeys are the sshd keywords LiteNode writes, lowercase.
var managedKeys = []string{"port", "listenaddress", "pubkeyauthentication", "passwordauthentication", "kbdinteractiveauthentication", "challengeresponseauthentication", "permitrootlogin"}

// Generate returns the drop-in for s, or nil when s manages nothing (the
// file is then removed). Turning password login off also turns off
// keyboard-interactive login, which otherwise still asks for the password
// through PAM (D-061).
//
// The port is also given as ListenAddress lines with that port: sshd
// listens on every Port line of every file, but ignores Port when each
// ListenAddress names its own port, so a Port in the main config (some
// providers add one) does not keep the old port open (D-065). ipv6 adds
// the IPv6 wildcard; without IPv6 it would break Ubuntu's ssh.socket.
func Generate(s state.SSH, ipv6 bool) []byte {
	var lines []string
	if s.Port > 0 {
		p := strconv.Itoa(s.Port)
		lines = append(lines, "Port "+p, "ListenAddress 0.0.0.0:"+p)
		if ipv6 {
			lines = append(lines, "ListenAddress [::]:"+p)
		}
	}
	if s.PubkeyAuth != nil {
		lines = append(lines, "PubkeyAuthentication "+yesNo(*s.PubkeyAuth))
	}
	if s.PasswordAuth != nil {
		lines = append(lines, "PasswordAuthentication "+yesNo(*s.PasswordAuth))
		if !*s.PasswordAuth {
			lines = append(lines, "KbdInteractiveAuthentication no")
		}
	}
	if s.PermitRootLogin != "" {
		lines = append(lines, "PermitRootLogin "+s.PermitRootLogin)
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte("# Managed by LiteNode; changes here are overwritten.\n" +
		"# sshd uses the first value it reads, and this file is read first.\n" +
		strings.Join(lines, "\n") + "\n")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Errors of CheckInclude.
var (
	// ErrNoInclude means the main config does not include
	// sshd_config.d/*.conf, so LiteNode's file would be ignored.
	ErrNoInclude = errors.New("sshd_config does not include sshd_config.d/*.conf")
	// ErrIncludeLate means a setting LiteNode manages comes before the
	// Include line (or the Include is inside a Match block) and wins.
	ErrIncludeLate = errors.New("sshd_config sets a managed option before including sshd_config.d")
)

// CheckInclude checks that the main config reads LiteNode's drop-in before
// any setting LiteNode manages. LiteNode does not modify the main config
// itself (DESIGN section 13); the caller tells the user what to fix.
func CheckInclude(main []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(main))
	var early []string
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		key := strings.ToLower(fields[0])
		if i := strings.IndexByte(key, '='); i >= 0 {
			key = key[:i]
		}
		switch {
		case key == "match":
			return fmt.Errorf("%w: Match block before Include", errFirst(early, ErrIncludeLate, ErrNoInclude))
		case key == "include" && includes(fields[1:], ConfigPath):
			if len(early) > 0 {
				return fmt.Errorf("%w: %s", ErrIncludeLate, strings.Join(early, ", "))
			}
			return nil
		case isManaged(key):
			early = append(early, fields[0])
		}
	}
	return ErrNoInclude
}

func errFirst(early []string, late, none error) error {
	if len(early) > 0 {
		return late
	}
	return none
}

func isManaged(key string) bool {
	for _, k := range managedKeys {
		if k == key {
			return true
		}
	}
	return false
}

// includes reports whether one of sshd's Include patterns matches file.
// Relative patterns are relative to /etc/ssh.
func includes(patterns []string, file string) bool {
	for _, p := range patterns {
		p = strings.Trim(p, `"`)
		if !strings.HasPrefix(p, "/") {
			p = "/etc/ssh/" + p
		}
		if ok, _ := path.Match(p, file); ok {
			return true
		}
	}
	return false
}
