// Package xray installs, updates and uninstalls Xray and generates its
// configuration.
package xray

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Aspeternity/litenode/internal/state"
)

// Repo is the upstream repository of Xray releases.
const Repo = "XTLS/Xray-core"

// File locations (P-001).
const (
	LibDir     = "/usr/local/lib/litenode"
	BinPath    = LibDir + "/xray"
	ConfigPath = state.ConfigDir + "/xray.json"
	UnitName   = "litenode-xray.service"
	UnitPath   = "/etc/systemd/system/" + UnitName
	// User is the dedicated system user Xray runs as (P-002).
	User = "litenode-xray"
)

// ErrDigestMismatch means the downloaded archive does not match the
// sha256 published with the release.
var ErrDigestMismatch = errors.New("xray archive sha256 mismatch")

// AssetName returns the release archive for a Go architecture.
func AssetName(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "Xray-linux-64.zip", nil
	case "arm64":
		return "Xray-linux-arm64-v8a.zip", nil
	}
	return "", fmt.Errorf("no xray release for architecture %s", goarch)
}

// DownloadURL returns the URL of a release asset.
func DownloadURL(version, asset string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", Repo, version, asset)
}

var versionRE = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// NormalizeVersion accepts "26.3.27" or "v26.3.27" and returns "v26.3.27".
func NormalizeVersion(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "v") {
		s = "v" + s
	}
	if !versionRE.MatchString(s) {
		return "", fmt.Errorf("invalid xray version %q", s)
	}
	return s, nil
}

// ParseDigest returns the sha256 hex digest from a release .dgst file,
// which lists several digests as "SHA2-256= <hex>" lines.
func ParseDigest(data []byte) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		name, value, ok := strings.Cut(sc.Text(), "=")
		if !ok || strings.TrimSpace(name) != "SHA2-256" {
			continue
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if b, err := hex.DecodeString(value); err != nil || len(b) != 32 {
			return "", fmt.Errorf("invalid SHA2-256 value %q in digest file", value)
		}
		return value, nil
	}
	return "", errors.New("no SHA2-256 line in digest file")
}

// ParseVersionOutput extracts the version from "xray version" output, whose
// first line looks like "Xray 26.3.27 (Xray, Penetrates Everything.) ...".
func ParseVersionOutput(out string) (string, error) {
	first, _, _ := strings.Cut(out, "\n")
	f := strings.Fields(first)
	if len(f) < 2 || f[0] != "Xray" {
		return "", fmt.Errorf("unexpected xray version output %q", first)
	}
	return NormalizeVersion(f[1])
}
