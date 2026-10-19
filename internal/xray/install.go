package xray

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

const (
	maxArchiveSize = 200 << 20
	maxDigestSize  = 64 << 10
	// maxAPISize bounds GitHub API responses, which include the full
	// release notes and can be large.
	maxAPISize = 4 << 20
	// restartSettle is how long to wait after a restart before checking
	// that Xray is still running; a bad config makes it exit at once.
	restartSettle = 2 * time.Second
)

// Installer installs, updates and uninstalls Xray.
//
// The Xray program is not handled by the change package: it is large, so
// instead of keeping ten backups an update keeps the old program as
// xray.bak only until the new one is known to work.
type Installer struct {
	Runner  system.Runner
	Fetcher Fetcher
	// Arch is the Go architecture (runtime.GOARCH).
	Arch  string
	Sleep func(time.Duration)
}

// NewInstaller returns an Installer for the running architecture.
func NewInstaller(r system.Runner, f Fetcher, arch string) *Installer {
	return &Installer{Runner: r, Fetcher: f, Arch: arch, Sleep: time.Sleep}
}

// Latest returns the latest stable Xray version.
func (in *Installer) Latest(ctx context.Context) (string, error) {
	data, err := in.Fetcher.Fetch(ctx, LatestReleaseURL, maxAPISize)
	if err != nil {
		return "", fmt.Errorf("query latest xray release: %w", err)
	}
	return ParseLatestRelease(data)
}

// Installed returns the version of the installed Xray program, or false
// if it is missing or does not run.
func (in *Installer) Installed(ctx context.Context) (string, bool) {
	return in.versionOf(ctx, BinPath)
}

func (in *Installer) versionOf(ctx context.Context, path string) (string, bool) {
	res, err := in.Runner.Query(ctx, path, "version")
	if err != nil {
		return "", false
	}
	v, err := ParseVersionOutput(res.Stdout)
	return v, err == nil
}

// resolve turns "" into the default version (D-054) and normalizes the
// rest.
func (in *Installer) resolve(ctx context.Context, version string) (string, error) {
	if version == "" {
		return in.DefaultVersion(ctx)
	}
	return NormalizeVersion(version)
}

// download fetches the release archive, verifies it against the published
// sha256 and returns the xray program from it. Other files in the archive
// (geoip.dat, geosite.dat) are not used (D-008).
func (in *Installer) download(ctx context.Context, version string) ([]byte, error) {
	asset, err := AssetName(in.Arch)
	if err != nil {
		return nil, err
	}
	url := DownloadURL(version, asset)
	archive, err := in.Fetcher.Fetch(ctx, url, maxArchiveSize)
	if err != nil {
		return nil, err
	}
	dgst, err := in.Fetcher.Fetch(ctx, url+".dgst", maxDigestSize)
	if err != nil {
		return nil, err
	}
	want, err := ParseDigest(dgst)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != want {
		return nil, ErrDigestMismatch
	}
	return extractXray(archive)
}

func extractXray(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open xray archive: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "xray" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open xray in archive: %w", err)
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxArchiveSize+1))
		if err != nil {
			return nil, fmt.Errorf("extract xray: %w", err)
		}
		if len(data) > maxArchiveSize {
			return nil, errors.New("xray in archive is too large")
		}
		return data, nil
	}
	return nil, errors.New("xray program not found in archive")
}

// DownloadError means querying or downloading a release failed; nothing
// was changed.
type DownloadError struct{ Err error }

func (e *DownloadError) Error() string { return "download xray: " + e.Err.Error() }
func (e *DownloadError) Unwrap() error { return e.Err }

// Install downloads Xray (DefaultVersion if version is empty),
// creates its user and systemd unit, and returns the installed version.
// It does not start the service: Xray only runs while there are enabled
// nodes. On failure everything it created is removed.
func (in *Installer) Install(ctx context.Context, version string) (v string, err error) {
	if v, err = in.resolve(ctx, version); err != nil {
		return "", err
	}
	bin, err := in.download(ctx, v)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			in.removeFiles(ctx)
		}
	}()
	if err := in.ensureUser(ctx); err != nil {
		return "", err
	}
	if err := in.Runner.MkdirAll(LibDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", LibDir, err)
	}
	if err := in.Runner.WriteFile(BinPath, bin, 0o755); err != nil {
		return "", fmt.Errorf("install xray: %w", err)
	}
	if got, ok := in.Installed(ctx); !ok || got != v {
		return "", fmt.Errorf("installed xray reports version %q, want %s", got, v)
	}
	if err := in.Runner.WriteFile(UnitPath, []byte(Unit()), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", UnitPath, err)
	}
	if err := system.DaemonReload(ctx, in.Runner); err != nil {
		return "", err
	}
	// Xray reads its config from the config directory, which also holds
	// the root-only state file: allow traversal but not listing.
	if err := system.EnsureDir(in.Runner, state.ConfigDir, state.ConfigDirMode); err != nil {
		return "", err
	}
	return v, nil
}

func (in *Installer) ensureUser(ctx context.Context) error {
	if _, err := in.Runner.Query(ctx, "id", "-u", User); err == nil {
		return nil
	}
	_, err := in.Runner.Exec(ctx, "useradd", "--system", "--user-group",
		"--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", User)
	if err != nil {
		return fmt.Errorf("create user %s: %w", User, err)
	}
	return nil
}

// Update replaces Xray with another version (DefaultVersion if version is
// empty) and returns it. The new program must report the right version
// and accept the current config, and a running service must keep running
// after a restart; otherwise the old program is put back.
func (in *Installer) Update(ctx context.Context, version string) (string, error) {
	old, ok := in.Installed(ctx)
	if !ok {
		return "", errors.New("xray is not installed")
	}
	v, err := in.resolve(ctx, version)
	if err != nil {
		return "", &DownloadError{Err: err}
	}
	bin, err := in.download(ctx, v)
	if err != nil {
		return "", &DownloadError{Err: err}
	}
	running, err := system.ServiceActive(ctx, in.Runner, UnitName)
	if err != nil {
		return "", err
	}

	bak := BinPath + ".bak"
	if err := in.Runner.Rename(BinPath, bak); err != nil {
		return "", fmt.Errorf("back up xray %s: %w", old, err)
	}
	if err := in.tryNew(ctx, bin, v, running); err != nil {
		if rerr := in.revert(ctx, bak, running); rerr != nil {
			return "", fmt.Errorf("update to %s failed: %w; reverting to %s also failed: %v", v, err, old, rerr)
		}
		return "", fmt.Errorf("update to %s failed, reverted to %s: %w", v, old, err)
	}
	in.Runner.Remove(bak)
	return v, nil
}

func (in *Installer) tryNew(ctx context.Context, bin []byte, v string, running bool) error {
	if err := in.Runner.WriteFile(BinPath, bin, 0o755); err != nil {
		return fmt.Errorf("install xray: %w", err)
	}
	if got, ok := in.Installed(ctx); !ok || got != v {
		return fmt.Errorf("new xray reports version %q, want %s", got, v)
	}
	if _, err := in.Runner.ReadFile(ConfigPath); err == nil {
		if _, err := in.Runner.Query(ctx, BinPath, "run", "-test", "-c", ConfigPath); err != nil {
			return fmt.Errorf("new xray rejects the current config: %w", err)
		}
	}
	if running {
		return in.restartAndCheck(ctx)
	}
	return nil
}

func (in *Installer) revert(ctx context.Context, bak string, running bool) error {
	if err := in.Runner.Rename(bak, BinPath); err != nil {
		return fmt.Errorf("restore xray: %w", err)
	}
	if !running {
		return nil
	}
	// A new program that crashes is restarted by systemd (Restart=on-failure)
	// until the start rate limit is hit; clear that so the old one can start.
	in.Runner.Exec(ctx, "systemctl", "reset-failed", UnitName)
	return in.restartAndCheck(ctx)
}

// Restart restarts the service and checks that it keeps running.
func (in *Installer) Restart(ctx context.Context) error {
	return in.restartAndCheck(ctx)
}

func (in *Installer) restartAndCheck(ctx context.Context) error {
	if err := system.Systemctl(ctx, in.Runner, system.ServiceRestart, UnitName); err != nil {
		return err
	}
	in.Sleep(restartSettle)
	active, err := system.ServiceActive(ctx, in.Runner, UnitName)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("%s is not running after restart", UnitName)
	}
	return nil
}

// Uninstall stops Xray and removes the program, unit, config and user.
// It keeps going after errors so that as much as possible is removed.
// Deleting nodes and clients from the state is the caller's job (D-020).
func (in *Installer) Uninstall(ctx context.Context) error {
	var errs []error
	if _, err := in.Runner.ReadFile(UnitPath); err == nil {
		if err := system.Systemctl(ctx, in.Runner, system.ServiceStop, UnitName); err != nil {
			errs = append(errs, err)
		}
		if err := system.Systemctl(ctx, in.Runner, system.ServiceDisable, UnitName); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, in.removeFiles(ctx))
	if err := in.Runner.Remove(ConfigPath); err != nil {
		errs = append(errs, fmt.Errorf("remove %s: %w", ConfigPath, err))
	}
	if _, err := in.Runner.Query(ctx, "id", "-u", User); err == nil {
		if _, err := in.Runner.Exec(ctx, "userdel", User); err != nil {
			errs = append(errs, fmt.Errorf("delete user %s: %w", User, err))
		}
	}
	return errors.Join(errs...)
}

// removeFiles deletes the program and unit and reloads systemd.
func (in *Installer) removeFiles(ctx context.Context) error {
	var errs []error
	for _, p := range []string{BinPath, BinPath + ".bak", UnitPath} {
		if err := in.Runner.Remove(p); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", p, err))
		}
	}
	// Remove the directory only if nothing else is in it.
	if names, err := in.Runner.ReadDir(LibDir); err == nil && len(names) == 0 {
		in.Runner.Remove(LibDir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	if err := system.DaemonReload(ctx, in.Runner); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
