package sysopt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
)

// GaiPath configures the address order of getaddrinfo, which programs
// such as curl and apt use to pick IPv4 or IPv6.
const GaiPath = "/etc/gai.conf"

const (
	gaiBegin = "# BEGIN LiteNode: prefer IPv4 (managed by LiteNode)"
	gaiLine  = "precedence ::ffff:0:0/96  100"
	gaiEnd   = "# END LiteNode"
	// mappedPrefix is the prefix of IPv4 addresses in the precedence table.
	mappedPrefix = "::ffff:0:0/96"
	// ipv6Precedence is the default precedence of global IPv6 addresses
	// (RFC 6724): IPv4 wins above it.
	ipv6Precedence = 40
)

// Preference is the address family preferred for outgoing connections.
type Preference struct {
	// IPv4 is true when IPv4 is preferred.
	IPv4 bool
	// Managed is true when LiteNode's block sets the preference.
	Managed bool
	// Foreign is true when another line of gai.conf prefers IPv4, so
	// removing LiteNode's block keeps IPv4 first.
	Foreign bool
}

// ParseGai reads the preference from gai.conf.
func ParseGai(data []byte) Preference {
	var p Preference
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == gaiBegin:
			inBlock = true
			continue
		case t == gaiEnd:
			inBlock = false
			continue
		}
		f := strings.Fields(t)
		if len(f) < 3 || f[0] != "precedence" || f[1] != mappedPrefix {
			continue
		}
		if n, err := strconv.Atoi(f[2]); err == nil && n > ipv6Precedence {
			if inBlock {
				p.Managed = true
			} else {
				p.Foreign = true
			}
		}
	}
	p.IPv4 = p.Managed || p.Foreign
	return p
}

// GaiWithIPv4 returns gai.conf with or without LiteNode's block. Other
// lines are kept as they are.
func GaiWithIPv4(data []byte, ipv4 bool) []byte {
	var lines []string
	inBlock := false
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		switch t := strings.TrimSpace(line); {
		case t == gaiBegin:
			inBlock = true
		case t == gaiEnd:
			inBlock = false
		case !inBlock:
			lines = append(lines, line)
		}
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if ipv4 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, gaiBegin, gaiLine, gaiEnd)
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// PreferenceStatus reads /etc/gai.conf; a missing file is the default
// (IPv6 first).
func (s *Service) PreferenceStatus() (Preference, error) {
	data, err := s.Runner.ReadFile(GaiPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Preference{}, fmt.Errorf("read %s: %w", GaiPath, err)
	}
	return ParseGai(data), nil
}

// SetPreference writes or removes LiteNode's block. glibc reloads
// gai.conf when it changes, so nothing needs restarting. A file left
// empty is removed.
func (s *Service) SetPreference(ctx context.Context, ipv4 bool) error {
	data, err := s.Runner.ReadFile(GaiPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", GaiPath, err)
	}
	f := change.File{Path: GaiPath, Data: GaiWithIPv4(data, ipv4), Mode: 0o644}
	if f.Data == nil {
		if data == nil {
			return nil
		}
		f = change.File{Path: GaiPath, Remove: true}
	}
	_, err = s.Changes.Apply(ctx, change.Change{Parts: []change.Part{{Kind: &change.Kind{Name: "gai"}, Files: []change.File{f}}}})
	return err
}
