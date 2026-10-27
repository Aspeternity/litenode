//go:build vps

// Integration test for the test VPS only. It installs, updates and removes
// Xray on the real system, so it is excluded from normal builds:
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o xray-vps.test ./internal/xray
//	./xray-vps.test -test.v -test.run 'TestVPS' -test.timeout 15m   (as root, on the test VPS)
package xray

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/node/security"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// overrideFetcher serves some URLs from memory and the rest from the network.
type overrideFetcher struct {
	real     Fetcher
	override map[string][]byte
}

func (f overrideFetcher) Fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	if data, ok := f.override[url]; ok {
		return data, nil
	}
	return f.real.Fetch(ctx, url, limit)
}

func (f overrideFetcher) Redirect(ctx context.Context, url string) (string, error) {
	if data, ok := f.override[url]; ok {
		return string(data), nil
	}
	return f.real.Redirect(ctx, url)
}

// testConfig listens on a privileged port to prove that the non-root
// service can bind it (P-002).
const testConfig = `{
  "log": {"loglevel": "warning"},
  "inbounds": [{"listen": "127.0.0.1", "port": 443, "protocol": "dokodemo-door",
                "settings": {"address": "127.0.0.1", "port": 1}}],
  "outbounds": [{"protocol": "freedom"}]
}
`

func TestVPS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	ctx := context.Background()
	r := system.NewReal(system.NewOpLog(system.DefaultLogPath))
	real := NewHTTPFetcher()
	fetch := overrideFetcher{real: real, override: map[string][]byte{}}
	in := NewInstaller(r, fetch, runtime.GOARCH)

	_, statErr := os.Stat(state.ConfigDir)
	createdConfigDir := errors.Is(statErr, os.ErrNotExist)
	in.Uninstall(ctx) // leftovers from an earlier run
	t.Cleanup(func() {
		in.Uninstall(ctx)
		if createdConfigDir {
			os.RemoveAll(state.ConfigDir)
		}
	})

	latest, prev := stableVersions(t, real)
	t.Logf("latest stable %s, previous stable %s", latest, prev)

	step(t, "install default version", func() error {
		want, err := in.DefaultVersion(ctx)
		if err != nil {
			return err
		}
		v, err := in.Install(ctx, "")
		if err != nil {
			return err
		}
		if v != want {
			return fmt.Errorf("installed %s, want %s", v, want)
		}
		t.Logf("    default version %s (D-054)", v)
		if _, err := user.Lookup(User); err != nil {
			return fmt.Errorf("user %s missing: %w", User, err)
		}
		return nil
	})

	store := state.NewStore(r)
	m := change.NewManager(r, store)
	kind := &change.Kind{
		Name: "xray",
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			_, err := r.Query(ctx, BinPath, "run", "-test", "-c", paths[ConfigPath])
			return err
		},
		// Apply must act on what is on disk now: after a restore there may
		// be no config at all, and then the service is stopped.
		Apply: func(ctx context.Context, r system.Runner) error {
			if _, err := r.ReadFile(ConfigPath); err != nil {
				return system.Systemctl(ctx, r, system.ServiceStop, UnitName)
			}
			return in.restartAndCheck(ctx)
		},
	}
	step(t, "start service as non-root on port 443", func() error {
		_, err := m.Apply(ctx, change.Change{Parts: []change.Part{{Kind: kind, Files: []change.File{
			{Path: ConfigPath, Data: []byte(testConfig), Mode: state.GroupFileMode, Group: User},
		}}}})
		if err != nil {
			return err
		}
		return checkRunning(ctx, r)
	})

	step(t, "update to "+prev, func() error {
		v, err := in.Update(ctx, prev)
		if err != nil {
			return err
		}
		if got, _ := in.Installed(ctx); got != prev || v != prev {
			return fmt.Errorf("installed %s after update, want %s", got, prev)
		}
		return checkRunning(ctx, r)
	})

	// Broken "new versions" served in place of the latest release: one
	// rejects the config, one passes the config test but dies when run.
	latestNum := strings.TrimPrefix(latest, "v")
	for name, program := range map[string]string{
		"rejects config": "#!/bin/sh\n[ \"$1\" = version ] && echo 'Xray " + latestNum + " (Xray, fake)' && exit 0\nexit 23\n",
		"dies on start": "#!/bin/sh\n[ \"$1\" = version ] && echo 'Xray " + latestNum + " (Xray, fake)' && exit 0\n" +
			"for a in \"$@\"; do [ \"$a\" = -test ] && exit 0; done\nexit 1\n",
	} {
		step(t, "failed update reverts: "+name, func() error {
			serveFake(t, fetch.override, latest, program)
			if _, err := in.Update(ctx, latest); err == nil {
				return errors.New("update with a broken program succeeded")
			} else {
				t.Logf("    update error (expected): %v", err)
			}
			if got, _ := in.Installed(ctx); got != prev {
				return fmt.Errorf("installed %s after failed update, want %s", got, prev)
			}
			if _, err := os.Stat(BinPath + ".bak"); !errors.Is(err, os.ErrNotExist) {
				return errors.New("xray.bak left behind")
			}
			return checkRunning(ctx, r)
		})
	}

	step(t, "permissions under "+state.ConfigDir, func() error {
		bad, err := state.AuditPermissions(state.ConfigDir)
		if err != nil {
			return err
		}
		if len(bad) > 0 {
			return fmt.Errorf("readable by others: %s", strings.Join(bad, "; "))
		}
		out, err := r.Query(ctx, "stat", "-c", "%U:%G %a", ConfigPath)
		if err != nil {
			return err
		}
		if got := strings.TrimSpace(out.Stdout); got != "root:"+User+" 640" {
			return fmt.Errorf("xray.json is %s, want root:%s 640", got, User)
		}
		return nil
	})

	step(t, "uninstall", func() error {
		if err := in.Uninstall(ctx); err != nil {
			return err
		}
		for _, p := range []string{BinPath, UnitPath, ConfigPath, LibDir} {
			if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s still exists", p)
			}
		}
		if _, err := user.Lookup(User); err == nil {
			return fmt.Errorf("user %s still exists", User)
		}
		if active, _ := system.ServiceActive(ctx, r, UnitName); active {
			return errors.New("service still active")
		}
		return nil
	})
}

func step(t *testing.T, name string, fn func() error) {
	t.Helper()
	if err := fn(); err != nil {
		out, _ := system.NewReal(nil).Query(context.Background(),
			"journalctl", "-u", UnitName, "--no-pager", "-n", "15", "-o", "cat")
		t.Fatalf("FAIL %s: %v\njournal:\n%s", name, err, out.Stdout)
	}
	t.Logf("ok   %s", name)
}

// checkRunning verifies the service is active, runs as the dedicated user
// and listens on 127.0.0.1:443.
func checkRunning(ctx context.Context, r system.Runner) error {
	if active, err := system.ServiceActive(ctx, r, UnitName); err != nil || !active {
		return fmt.Errorf("service not active (%v)", err)
	}
	pid, err := r.Query(ctx, "systemctl", "show", "-p", "MainPID", "--value", UnitName)
	if err != nil {
		return err
	}
	owner, err := r.Query(ctx, "ps", "-o", "user=", "-p", strings.TrimSpace(pid.Stdout))
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(owner.Stdout); got != User {
		return fmt.Errorf("xray runs as %q, want %s", got, User)
	}
	for range 10 {
		out, _ := r.Query(ctx, "ss", "-ltnH", "sport = :443")
		if strings.Contains(out.Stdout, "127.0.0.1:443") {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("nothing listening on 127.0.0.1:443")
}

func serveFake(t *testing.T, override map[string][]byte, version, program string) {
	t.Helper()
	fx := fakeFetcher{}
	publish(t, fx, version, program)
	asset, _ := AssetName(runtime.GOARCH)
	url := DownloadURL(version, asset)
	override[url] = fx[DownloadURL(version, "Xray-linux-64.zip")]
	override[url+".dgst"] = fx[DownloadURL(version, "Xray-linux-64.zip")+".dgst"]
}

// stableVersions returns the latest and the previous stable releases.
func stableVersions(t *testing.T, f Fetcher) (latest, prev string) {
	t.Helper()
	data, err := f.Fetch(context.Background(), "https://api.github.com/repos/"+Repo+"/releases?per_page=30", 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	var rels []struct {
		Tag        string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err := json.Unmarshal(data, &rels); err != nil {
		t.Fatal(err)
	}
	var stable []string
	for _, r := range rels {
		if !r.Prerelease && !r.Draft {
			stable = append(stable, r.Tag)
		}
	}
	if len(stable) < 2 {
		t.Fatalf("need two stable releases, got %v", stable)
	}
	return stable[0], stable[1]
}

// TestVPSGeneratedConfig checks that GenerateConfig's output passes
// "xray run -test" and runs as a service (M1-03).
func TestVPSGeneratedConfig(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	ctx := context.Background()
	r := system.NewReal(system.NewOpLog(system.DefaultLogPath))
	in := NewInstaller(r, NewHTTPFetcher(), runtime.GOARCH)
	_, statErr := os.Stat(state.ConfigDir)
	createdConfigDir := errors.Is(statErr, os.ErrNotExist)
	in.Uninstall(ctx)
	t.Cleanup(func() {
		in.Uninstall(ctx)
		if createdConfigDir {
			os.RemoveAll(state.ConfigDir)
		}
	})

	step(t, "install latest", func() error {
		_, err := in.Install(ctx, "")
		return err
	})
	reg, err := builtin.Registry()
	if err != nil {
		t.Fatal(err)
	}
	data, err := GenerateConfig(reg, goldenNodes(), "")
	if err != nil {
		t.Fatal(err)
	}
	m := change.NewManager(r, state.NewStore(r))
	kind := &change.Kind{
		Name: "xray",
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			res, err := r.Query(ctx, BinPath, "run", "-test", "-c", paths[ConfigPath])
			t.Logf("    xray run -test: %s", strings.TrimSpace(res.Stdout))
			return err
		},
		Apply: func(ctx context.Context, r system.Runner) error { return in.restartAndCheck(ctx) },
	}
	step(t, "generated config passes xray run -test and runs", func() error {
		_, err := m.Apply(ctx, change.Change{Parts: []change.Part{{Kind: kind, Files: []change.File{
			{Path: ConfigPath, Data: data, Mode: state.GroupFileMode, Group: User},
		}}}})
		return err
	})
	step(t, "listening on 443 and 2053 (IPv4 and IPv6)", func() error {
		for range 10 {
			out, _ := r.Query(ctx, "ss", "-ltnH")
			if strings.Contains(out.Stdout, "*:443") && strings.Contains(out.Stdout, "*:2053") {
				return nil
			}
			time.Sleep(300 * time.Millisecond)
		}
		out, _ := r.Query(ctx, "ss", "-ltnH")
		return fmt.Errorf("not listening on all addresses:\n%s", out.Stdout)
	})
}

// TestVPSKeysMatchXray checks LiteNode's x25519 keys against the real
// "xray x25519 -i <private key>" (M1-04, D-047).
func TestVPSKeysMatchXray(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	in := NewInstaller(r, NewHTTPFetcher(), runtime.GOARCH)
	_, statErr := os.Stat(state.ConfigDir)
	createdConfigDir := errors.Is(statErr, os.ErrNotExist)
	in.Uninstall(ctx)
	t.Cleanup(func() {
		in.Uninstall(ctx)
		if createdConfigDir {
			os.RemoveAll(state.ConfigDir)
		}
	})
	step(t, "install latest", func() error {
		_, err := in.Install(ctx, "")
		return err
	})
	step(t, "20 generated key pairs match xray x25519 -i", func() error {
		for range 20 {
			priv, pub, err := security.NewX25519(cryptorand.Reader)
			if err != nil {
				return err
			}
			out, err := r.Query(ctx, BinPath, "x25519", "-i", priv)
			if err != nil {
				return err
			}
			var gotPriv, gotPub string
			for line := range strings.Lines(out.Stdout) {
				k, v, _ := strings.Cut(strings.TrimSpace(line), ": ")
				switch k {
				case "PrivateKey":
					gotPriv = v
				case "Password (PublicKey)", "Public key":
					gotPub = v
				}
			}
			if gotPriv != priv || gotPub != pub {
				return fmt.Errorf("xray says %q / %q, LiteNode %q / %q", gotPriv, gotPub, priv, pub)
			}
		}
		return nil
	})
}
