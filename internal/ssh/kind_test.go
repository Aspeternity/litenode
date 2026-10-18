package ssh

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const effectiveDefault = "port 22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"

// procTCP lists port 22222 (0x56CE) as listening.
const procTCP = "  sl  local_address rem_address   st\n   0: 00000000:56CE 00000000:0000 0A\n"

func init() { Sleep = func(time.Duration) {} }

func sshSystem(effective string) *systemtest.Fake {
	f := systemtest.New()
	f.Files["/proc/net/tcp"] = []byte(procTCP)
	// Debian: ssh.socket masked, ssh.service running.
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", SocketUnit)
	f.Files[MainConfigPath] = []byte("Include /etc/ssh/sshd_config.d/*.conf\nKbdInteractiveAuthentication no\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: effective}}, SshdPath, "-T")
	return f
}

// apply changes the SSH settings to s through the change flow.
func apply(t *testing.T, f *systemtest.Fake, s state.SSH) error {
	t.Helper()
	store := state.NewStore(f)
	st := state.New()
	st.SSH = s
	_, err := change.NewManager(f, store).Apply(context.Background(), change.Change{
		Parts: []change.Part{{Kind: Kind(store), Files: []change.File{ConfigFile(s, true)}}},
		State: st,
	})
	return err
}

var keysOnly = state.SSH{Port: 22222, PasswordAuth: ptr(false), PermitRootLogin: RootKeysOnly}

func TestKindApplies(t *testing.T) {
	f := sshSystem("port 22222\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin prohibit-password\n")
	if err := apply(t, f, keysOnly); err != nil {
		t.Fatal(err)
	}
	if string(f.Files[ConfigPath]) != string(Generate(keysOnly, true)) {
		t.Errorf("drop-in = %q", f.Files[ConfigPath])
	}
	var order []string
	for _, c := range f.Calls() {
		switch {
		case c.Op == systemtest.OpWrite && c.Path == ConfigPath:
			order = append(order, "write")
			if c.Perm != state.PrivateFileMode {
				t.Errorf("drop-in mode %v", c.Perm)
			}
		case c.Command == SshdPath+" -t":
			order = append(order, "test")
		case c.Command == "systemctl reload "+Unit:
			order = append(order, "reload")
		case c.Command == SshdPath+" -T":
			order = append(order, "effective")
		}
	}
	if want := []string{"write", "test", "reload", "effective"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	if st, _ := state.NewStore(f).Load(); st.SSH.Port != 22222 {
		t.Errorf("state not saved: %+v", st.SSH)
	}
}

// restored checks that a failed change left no drop-in and no SSH state.
func restored(t *testing.T, f *systemtest.Fake, err error, want error) {
	t.Helper()
	var ce *change.Error
	if !errors.As(err, &ce) || !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if ce.RestoreErr != nil {
		t.Errorf("restore failed: %v", ce.RestoreErr)
	}
	if _, ok := f.Files[ConfigPath]; ok {
		t.Error("drop-in left behind")
	}
	if st, _ := state.NewStore(f).Load(); st.SSH.Port != 0 {
		t.Errorf("state not restored: %+v", st.SSH)
	}
}

func TestKindNotEffectiveRestores(t *testing.T) {
	// sshd still listens on 22 as well: another file sets Port too.
	f := sshSystem("port 22222\nport 22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin prohibit-password\n")
	restored(t, f, apply(t, f, keysOnly), ErrNotEffective)
}

func TestKindSshdTestFails(t *testing.T) {
	f := sshSystem(effectiveDefault)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 255, Stderr: "bad configuration"}}, SshdPath, "-t")
	err := apply(t, f, keysOnly)
	var ce *system.CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v", err)
	}
	restored(t, f, err, ce)
	// The restore may reload sshd, but only after the bad drop-in is gone.
	present := false
	for _, c := range f.Calls() {
		switch {
		case c.Op == systemtest.OpWrite && c.Path == ConfigPath:
			present = true
		case c.Op == systemtest.OpRemove && c.Path == ConfigPath:
			present = false
		case c.Command == "systemctl reload "+Unit && present:
			t.Error("sshd reloaded with the bad configuration in place")
		}
	}
}

func TestKindNoInclude(t *testing.T) {
	f := sshSystem(effectiveDefault)
	f.Files[MainConfigPath] = []byte("Port 22\n")
	restored(t, f, apply(t, f, keysOnly), ErrNoInclude)
}

func TestKindSocketActivatedNotRunning(t *testing.T) {
	f := sshSystem("port 22222\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin prohibit-password\n")
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", Unit)
	if err := apply(t, f, keysOnly); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.Execs(), "systemctl reload "+Unit) {
		t.Error("reloaded an sshd that is not running")
	}
}

func TestKindRemovesDropInWhenNothingManaged(t *testing.T) {
	f := sshSystem(effectiveDefault)
	f.Files[ConfigPath] = []byte("Port 2222\n")
	if err := apply(t, f, state.SSH{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[ConfigPath]; ok {
		t.Error("drop-in not removed")
	}
}

func TestPrivsepDirCreatedWhenMissing(t *testing.T) {
	// Socket-activated sshd that never started: /run/sshd is missing and
	// sshd -t / -T would refuse to run.
	f := sshSystem("port 22222\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin prohibit-password\n")
	if err := apply(t, f, keysOnly); err != nil {
		t.Fatal(err)
	}
	created := -1
	for i, c := range f.Calls() {
		if c.Op == systemtest.OpMkdir && c.Path == PrivsepDir && created < 0 {
			created = i
		}
		if c.Command == SshdPath+" -t" && (created < 0 || created > i) {
			t.Fatal("sshd -t ran before /run/sshd was created")
		}
	}
	if created < 0 {
		t.Error("/run/sshd not created")
	}
	// Once it exists it is left alone.
	f2 := sshSystem(effectiveDefault)
	f2.Dirs[PrivsepDir] = true
	if _, err := ReadEffective(context.Background(), f2); err != nil {
		t.Fatal(err)
	}
	for _, c := range f2.Calls() {
		if c.Op == systemtest.OpMkdir || c.Op == systemtest.OpChmod {
			t.Errorf("existing /run/sshd touched: %+v", c)
		}
	}
}

const effective22222 = "port 22222\nlistenaddress 0.0.0.0:22222\nlistenaddress [::]:22222\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin prohibit-password\n"

// ubuntu is sshSystem with socket activation and the socket listening on
// socketListen.
func ubuntu(effective, socketListen string) *systemtest.Fake {
	f := sshSystem(effective)
	f.Respond(systemtest.Response{}, "systemctl", "is-active", "--quiet", SocketUnit)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: socketListen}}, "systemctl", "show", "--property=Listen", "--value", SocketUnit)
	return f
}

func TestKindSocketRestartsOnPortChange(t *testing.T) {
	f := ubuntu(effective22222, "[::]:22 (Stream)\n0.0.0.0:22 (Stream)\n")
	if err := apply(t, f, keysOnly); err != nil {
		t.Fatal(err)
	}
	// The socket's ports are read before daemon-reload, which would load
	// the new ones (found on Ubuntu 24.04).
	show, daemonReload := -1, -1
	for i, c := range f.Calls() {
		switch c.Command {
		case "systemctl show --property=Listen --value " + SocketUnit:
			show = i
		case "systemctl daemon-reload":
			daemonReload = i
		}
	}
	if show < 0 || show > daemonReload {
		t.Errorf("socket ports read after daemon-reload")
	}
	execs := f.Execs()
	reloadAt := slices.Index(execs, "systemctl daemon-reload")
	restartAt := slices.Index(execs, "systemctl restart "+SocketUnit)
	if reloadAt < 0 || restartAt < reloadAt {
		t.Errorf("want daemon-reload then restart ssh.socket: %v", execs)
	}
	if slices.Contains(execs, "systemctl reload "+Unit) {
		t.Error("ssh.service reloaded although the socket restart restarts it")
	}
}

func TestKindSocketSamePortsReloads(t *testing.T) {
	f := ubuntu(effective22222, "[::]:22222 (Stream)\n0.0.0.0:22222 (Stream)\n")
	if err := apply(t, f, keysOnly); err != nil {
		t.Fatal(err)
	}
	execs := f.Execs()
	if slices.Contains(execs, "systemctl restart "+SocketUnit) || !slices.Contains(execs, "systemctl reload "+Unit) {
		t.Errorf("execs = %v", execs)
	}
}

func TestKindNothingListeningRestores(t *testing.T) {
	f := sshSystem(effective22222)
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address rem_address   st\n")
	restored(t, f, apply(t, f, keysOnly), ErrNotEffective)
}

func TestParseEffectiveListenAddress(t *testing.T) {
	// Debian with "Port 61234" in the main config and LiteNode's
	// ListenAddress lines: sshd listens only on 2222.
	e, err := ParseEffective("port 2222\nport 61234\nlistenaddress [::]:2222\nlistenaddress 0.0.0.0:2222\npubkeyauthentication yes\npasswordauthentication yes\npermitrootlogin yes\n")
	if err != nil || !slices.Equal(e.Ports, []int{2222}) {
		t.Errorf("ports = %v, %v", e.Ports, err)
	}
}
