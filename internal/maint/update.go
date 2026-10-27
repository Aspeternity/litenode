package maint

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/xray"
)

// Repo is the LiteNode repository on GitHub.
const Repo = "Aspeternity/litenode"

// Installed program locations (DESIGN section 3).
const (
	BinPath  = "/usr/local/bin/litenode"
	LinkPath = "/usr/local/bin/lnode"
	// BakPath keeps the previous program after an update.
	BakPath = BinPath + ".bak"
	// newPath holds the downloaded program until it passed the checks.
	newPath = BinPath + ".new"
)

// Download size limits.
const (
	maxBinary = 64 << 20
	maxSums   = 64 << 10
)

// Errors of Update.
var (
	ErrDigestMismatch = errors.New("litenode binary sha256 mismatch")
	ErrBadBinary      = errors.New("downloaded litenode does not run")
)

// AssetName is the release file for a Go architecture.
func AssetName(goarch string) (string, error) {
	switch goarch {
	case "amd64", "arm64":
		return "litenode-linux-" + goarch, nil
	}
	return "", fmt.Errorf("no litenode release for architecture %s", goarch)
}

// AssetURL is the download URL of a release file.
func AssetURL(tag, name string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", Repo, tag, name)
}

// ParseSums finds the sha256 of name in a sha256sums.txt ("<hex>  <name>"
// or "<hex> *<name>").
func ParseSums(data []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			if _, err := hex.DecodeString(f[0]); err != nil || len(f[0]) != 64 {
				return "", fmt.Errorf("invalid sha256 for %s", name)
			}
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("sha256sums.txt has no entry for %s", name)
}

// UpdateAvailable reports whether latest is newer than current. A build
// without a version ("dev") can always update.
func UpdateAvailable(current, latest string) bool {
	if !strings.HasPrefix(current, "v") {
		return true
	}
	return xray.CompareVersions(latest, current) > 0
}

// Updater replaces the installed program with a release (DESIGN section
// 3, update).
type Updater struct {
	Runner  system.Runner
	Fetcher xray.Fetcher
	Arch    string
}

// Latest returns the tag of the latest release.
func (u *Updater) Latest(ctx context.Context) (string, error) {
	return xray.LatestTag(ctx, u.Fetcher, Repo)
}

// Update downloads release tag, checks its sha256 against the release's
// sha256sums.txt and that it runs and reports tag, then keeps the current
// program as litenode.bak and atomically puts the new one in place. A
// failed check changes nothing.
func (u *Updater) Update(ctx context.Context, tag string) error {
	name, err := AssetName(u.Arch)
	if err != nil {
		return err
	}
	bin, err := u.Fetcher.Fetch(ctx, AssetURL(tag, name), maxBinary)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	sums, err := u.Fetcher.Fetch(ctx, AssetURL(tag, "sha256sums.txt"), maxSums)
	if err != nil {
		return fmt.Errorf("download sha256sums.txt: %w", err)
	}
	want, err := ParseSums(sums, name)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(bin); hex.EncodeToString(got[:]) != want {
		return ErrDigestMismatch
	}
	r := u.Runner
	if err := r.WriteFile(newPath, bin, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", newPath, err)
	}
	res, err := r.Query(ctx, newPath, "version")
	if err != nil || !strings.HasPrefix(res.Stdout, "LiteNode "+tag+" ") {
		_ = r.Remove(newPath)
		return fmt.Errorf("%w: %q", ErrBadBinary, strings.TrimSpace(res.Stdout))
	}
	old, err := r.ReadFile(BinPath)
	if err != nil {
		_ = r.Remove(newPath)
		return fmt.Errorf("read %s: %w", BinPath, err)
	}
	if err := r.WriteFile(BakPath, old, 0o755); err != nil {
		_ = r.Remove(newPath)
		return fmt.Errorf("keep %s: %w", BakPath, err)
	}
	if err := r.Rename(newPath, BinPath); err != nil {
		_ = r.Remove(newPath)
		return fmt.Errorf("replace %s: %w", BinPath, err)
	}
	return nil
}
