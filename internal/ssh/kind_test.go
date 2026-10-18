package ssh

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const effectiveDefault = "port 22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"

func sshSystem(effective string) *systemtest.Fake {
	f := systemtest.New()
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
		Parts: []change.Part{{Kind: Kind(store), Files: []change.File{ConfigFile(s)}}},
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
	if string(f.Files[ConfigPath]) != string(Generate(keysOnly)) {
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
