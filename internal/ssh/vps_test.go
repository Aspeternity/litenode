//go:build vps

// Integration test for the test VPS only (M2-01). It changes the real sshd
// configuration, so a systemd timer removes LiteNode's drop-in after three
// minutes in case the test dies halfway. Existing SSH sessions survive.
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o ssh-vps.test ./internal/ssh
//	./ssh-vps.test -test.v -test.run TestVPS      (as root)
package ssh

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/xray"
)

const safetyUnit = "litenode-ssh-vps-test"

func vpsSetup(t *testing.T) (context.Context, system.Runner, *state.Store) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	if _, err := os.Stat(ConfigPath); err == nil {
		t.Fatalf("%s already exists; not touching a real LiteNode setup", ConfigPath)
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	r.Exec(ctx, "systemctl", "stop", safetyUnit+".timer")
	r.Exec(ctx, "systemctl", "reset-failed", safetyUnit+".service")
	if _, err := r.Exec(ctx, "systemd-run", "--unit="+safetyUnit, "--on-active=180",
		"/bin/sh", "-c", "rm -f "+ConfigPath+"; systemctl reload "+Unit); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Remove(ConfigPath)
		r.Exec(ctx, "systemctl", "reload", Unit)
		r.Exec(ctx, "systemctl", "stop", safetyUnit+".timer")
	})
	dir := t.TempDir()
	store := &state.Store{Runner: r, Path: filepath.Join(dir, "state.json"), BackupDir: filepath.Join(dir, "backups"), Now: state.NewStore(r).Now}
	return ctx, r, store
}

func vpsApply(ctx context.Context, r system.Runner, store *state.Store, s state.SSH) error {
	st := state.New()
	st.SSH = s
	_, err := change.NewManager(r, store).Apply(ctx, change.Change{
		Parts: []change.Part{{Kind: Kind(store), Files: []change.File{ConfigFile(s)}}},
		State: st,
	})
	return err
}

// TestVPS: with 50-cloud-init.conf saying "PasswordAuthentication yes",
// LiteNode's "no" still takes effect according to sshd -T.
func TestVPS(t *testing.T) {
	ctx, r, store := vpsSetup(t)
	if data, err := os.ReadFile("/etc/ssh/sshd_config.d/50-cloud-init.conf"); err != nil ||
		!regexp.MustCompile(`(?m)^PasswordAuthentication yes`).Match(data) {
		t.Fatalf("expected 50-cloud-init.conf with PasswordAuthentication yes: %v", err)
	}
	before, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: %+v", before)

	off := false
	if err := vpsApply(ctx, r, store, state.SSH{PasswordAuth: &off}); err != nil {
		t.Fatal(err)
	}
	e, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("password off: %+v", e)
	if e.PasswordAuth || e.KbdInteractive {
		t.Errorf("password login still on: %+v", e)
	}

	if err := vpsApply(ctx, r, store, state.SSH{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drop-in not removed: %v", err)
	}
	after, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if after.PasswordAuth != before.PasswordAuth || after.KbdInteractive != before.KbdInteractive {
		t.Errorf("not back to before: %+v", after)
	}
}

// TestVPSPortAccumulates: when the main config has its own Port line
// (Debian test VPS), sshd listens on both ports, so LiteNode's port check
// fails and the change is undone. Skipped where the main config sets no
// port.
func TestVPSPortAccumulates(t *testing.T) {
	main, err := os.ReadFile(MainConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^Port \d+`).Match(main) {
		t.Skip("main config sets no Port")
	}
	ctx, r, store := vpsSetup(t)
	before, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	err = vpsApply(ctx, r, store, state.SSH{Port: 22222})
	t.Logf("apply Port 22222: %v", err)
	if !errors.Is(err, ErrNotEffective) {
		t.Errorf("want ErrNotEffective, got %v", err)
	}
	after, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Ports) != len(before.Ports) || after.Ports[0] != before.Ports[0] {
		t.Errorf("ports %v, want %v", after.Ports, before.Ports)
	}
	if _, err := os.Stat(ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drop-in not removed: %v", err)
	}
}

// confirmSetup uses the real state store and binary, as the menu does, so
// that "litenode internal rollback" (run by systemd) finds the change.
func confirmSetup(t *testing.T) (context.Context, system.Runner, *Service) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	if _, err := os.Stat(ConfigPath); err == nil {
		t.Fatalf("%s already exists", ConfigPath)
	}
	r := system.NewReal(nil)
	store := state.NewStore(r)
	return context.Background(), r, &Service{Store: store, Confirmer: change.NewConfirmer(change.NewManager(r, store), "/usr/local/bin/litenode")}
}

func passwordAuth(t *testing.T, ctx context.Context, r system.Runner) bool {
	t.Helper()
	e, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	return e.PasswordAuth
}

// TestVPSUnconfirmed (M2-02): password login is turned off and not
// confirmed; after 60 seconds the timer restores it.
func TestVPSUnconfirmed(t *testing.T) {
	ctx, r, svc := confirmSetup(t)
	off := false
	if err := svc.Change(ctx, func(s *state.SSH) { s.PasswordAuth = &off }); err != nil {
		t.Fatal(err)
	}
	if passwordAuth(t, ctx, r) {
		t.Fatal("password login still on")
	}
	t.Log("password login off; not confirming, waiting 70 seconds")
	time.Sleep(70 * time.Second)
	if !passwordAuth(t, ctx, r) {
		t.Error("password login not restored")
	}
	if _, err := os.Stat(ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drop-in not removed: %v", err)
	}
	if _, err := os.Stat(change.DefaultPendingDir + "/pending"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("marker left: %v", err)
	}
}

// TestVPSHold (M2-02) turns password login off and waits without
// confirming, for checks where the session is cut or the server reboots.
// Run only on purpose: LN_HOLD=1.
func TestVPSHold(t *testing.T) {
	if os.Getenv("LN_HOLD") != "1" {
		t.Skip("set LN_HOLD=1")
	}
	ctx, _, svc := confirmSetup(t)
	off := false
	if err := svc.Change(ctx, func(s *state.SSH) { s.PasswordAuth = &off }); err != nil {
		t.Fatal(err)
	}
	t.Log("password login off; holding")
	os.WriteFile("/tmp/ln-hold-armed", nil, 0o600)
	time.Sleep(10 * time.Minute)
}

func keySetup(t *testing.T) (context.Context, system.Runner, *Service) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	r := system.NewReal(nil)
	store := state.NewStore(r)
	return context.Background(), r, &Service{Store: store, Confirmer: change.NewConfirmer(change.NewManager(r, store), "/usr/local/bin/litenode"),
		Fetcher: xray.NewHTTPFetcher(), Rand: rand.Reader}
}

// TestVPSGitHubImport (M2-03) imports a GitHub user's keys for root:
// LN_GITHUB=<user>. Logging in with the matching private key is checked
// from the development machine; LN_GITHUB_REMOVE=1 removes them again.
func TestVPSGitHubImport(t *testing.T) {
	user := os.Getenv("LN_GITHUB")
	if user == "" {
		t.Skip("set LN_GITHUB")
	}
	ctx, r, svc := keySetup(t)
	root, err := LookupAccount(r, "root")
	if err != nil {
		t.Fatal(err)
	}
	keys, skipped, err := svc.GitHubKeys(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		t.Logf("%s %s %s", k.ShortType(), k.Fingerprint(), k.Comment)
	}
	t.Logf("skipped %d", skipped)
	if os.Getenv("LN_GITHUB_REMOVE") == "1" {
		for _, k := range keys {
			if err := svc.DeleteKey(ctx, root, k.Fingerprint()); err != nil {
				t.Fatal(err)
			}
			if err := svc.Confirmer.Disarm(ctx); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	n, err := svc.AddKeys(ctx, root, keys)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("added %d", n)
}

// TestVPSGeneratedKeyLogin (M2-03): a key pair generated for a new
// regular user is valid for ssh-keygen and logs that user in.
func TestVPSGeneratedKeyLogin(t *testing.T) {
	ctx, r, svc := keySetup(t)
	const name = "lntestkey"
	if _, err := r.Exec(ctx, "useradd", "-m", "-s", "/bin/bash", name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Exec(ctx, "pkill", "-KILL", "-u", name)
		r.Exec(ctx, "userdel", "-r", "-f", name)
	})
	a, err := LookupAccount(r, name)
	if err != nil {
		t.Fatal(err)
	}
	k, priv, err := svc.GenerateKey(ctx, a, "litenode-test")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "id")
	if err := os.WriteFile(keyFile, priv, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := r.Query(ctx, "ssh-keygen", "-y", "-f", keyFile)
	if err != nil {
		t.Fatalf("ssh-keygen rejects the private key: %v", err)
	}
	if got := strings.Fields(res.Stdout); len(got) < 2 || got[0]+" "+got[1] != strings.Join(strings.Fields(k.Line())[:2], " ") {
		t.Errorf("ssh-keygen -y = %q, want %q", res.Stdout, k.Line())
	}
	out, err := r.Query(ctx, "stat", "-c", "%U %a", a.SSHDir(), a.AuthorizedKeysPath())
	t.Logf("ownership:\n%s", out.Stdout)
	if err != nil || out.Stdout != name+" 700\n"+name+" 600\n" {
		t.Errorf("ownership/modes: %q %v", out.Stdout, err)
	}
	e, err := ReadEffective(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	login, err := r.Query(ctx, "ssh", "-i", keyFile, "-p", strconv.Itoa(e.Ports[0]), "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null", "-o", "IdentitiesOnly=yes", name+"@127.0.0.1", "id", "-un")
	t.Logf("login: %q %v", login.Stdout, err)
	if err != nil || strings.TrimSpace(login.Stdout) != name {
		t.Errorf("login with the generated key failed: %v %s", err, login.Stderr)
	}
}
