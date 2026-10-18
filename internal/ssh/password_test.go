package ssh

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const newPassword = "N3w-Secret-Pass"

// fakeChpasswd makes chpasswd change the shadow file, as the real one
// does: plain passwords become "$fake$<password>", "-e" stores the hash.
func fakeChpasswd(f *systemtest.Fake) {
	set := func(encrypted bool) func() {
		return func() {
			calls := f.Calls()
			line := strings.TrimSpace(string(calls[len(calls)-1].Data))
			name, pass, _ := strings.Cut(line, ":")
			if !encrypted {
				pass = "$fake$" + pass
			}
			var out []string
			for _, l := range strings.Split(strings.TrimRight(string(f.Files[ShadowPath]), "\n"), "\n") {
				fields := strings.Split(l, ":")
				if fields[0] == name {
					fields[1] = pass
				}
				out = append(out, strings.Join(fields, ":"))
			}
			f.Files[ShadowPath] = []byte(strings.Join(out, "\n") + "\n")
		}
	}
	f.OnCommand("chpasswd", set(false))
	f.OnCommand("chpasswd -e", set(true))
}

func shadowOf(f *systemtest.Fake, name string) string {
	field, _ := ShadowField(f.Files[ShadowPath], name)
	return field
}

// TestChangePasswordTimeoutRestoresHash: the password reaches chpasswd
// only through standard input, the change is armed, and the rollback the
// timer runs puts the old hash back (D-096).
func TestChangePasswordTimeoutRestoresHash(t *testing.T) {
	ctx := context.Background()
	f, svc := loginService(t, effectiveDefault)
	fakeChpasswd(f)
	if err := svc.ChangePassword(ctx, "root", newPassword); err != nil {
		t.Fatal(err)
	}
	if got := shadowOf(f, "root"); got != "$fake$"+newPassword {
		t.Fatalf("root hash = %q", got)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c.Command, newPassword) || strings.Contains(c.Path, newPassword) {
			t.Errorf("password in a command line or path: %+v", c)
		}
		if c.Op == systemtest.OpWrite && strings.Contains(string(c.Data), newPassword) {
			t.Errorf("password written to %s", c.Path)
		}
	}
	if _, ok := f.Files["/etc/litenode/"+PasswordRestoreName]; ok {
		t.Error("restore file left behind")
	}
	if _, ok := f.Files[change.DefaultPendingDir+"/pending"]; !ok {
		t.Fatal("rollback not armed")
	}
	reg := change.Registry{}
	for _, k := range svc.Kinds() {
		reg.Register(k)
	}
	if done, err := svc.Confirmer.Rollback(ctx, reg); err != nil || !done {
		t.Fatalf("rollback: %v %v", done, err)
	}
	if got := shadowOf(f, "root"); got != "$y$hash" {
		t.Errorf("after rollback root hash = %q, want the old one", got)
	}
	if got := shadowOf(f, "alice"); got != "!" {
		t.Errorf("alice changed: %q", got)
	}
}

// TestChangePasswordKept: confirming keeps the new password.
func TestChangePasswordKept(t *testing.T) {
	ctx := context.Background()
	f, svc := loginService(t, effectiveDefault)
	fakeChpasswd(f)
	if err := svc.ChangePassword(ctx, "alice", newPassword); err != nil {
		t.Fatal(err)
	}
	if err := svc.Confirmer.Disarm(ctx); err != nil {
		t.Fatal(err)
	}
	if done, _ := svc.Confirmer.Rollback(ctx, change.Registry{}); done {
		t.Error("rolled back after confirming")
	}
	if got := shadowOf(f, "alice"); got != "$fake$"+newPassword {
		t.Errorf("alice hash = %q", got)
	}
}

func TestChangePasswordRefused(t *testing.T) {
	ctx := context.Background()
	f, svc := loginService(t, effectiveDefault)
	if err := svc.ChangePassword(ctx, "svc", newPassword); !errors.Is(err, ErrNotAdmin) {
		t.Errorf("svc: %v", err)
	}
	if err := svc.ChangePassword(ctx, "root", "short"); !errors.Is(err, ErrPasswordShort) {
		t.Errorf("short: %v", err)
	}
	for _, m := range f.Mutations() {
		t.Errorf("refused change touched the system: %+v", m)
	}
}

// TestChangePasswordChpasswdFails: the armed change is undone at once.
func TestChangePasswordChpasswdFails(t *testing.T) {
	ctx := context.Background()
	f, svc := loginService(t, effectiveDefault)
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "chpasswd")
	if err := svc.ChangePassword(ctx, "root", newPassword); err == nil {
		t.Fatal("no error")
	}
	if _, ok := f.Files[change.DefaultPendingDir+"/pending"]; ok {
		t.Error("rollback still pending")
	}
	if got := shadowOf(f, "root"); got != "$y$hash" {
		t.Errorf("root hash = %q", got)
	}
}

func TestDeleteSudoUser(t *testing.T) {
	ctx := context.Background()
	notRunning := systemtest.Response{Result: system.Result{ExitCode: 1}}

	// Root can still log in with its password.
	f, svc := loginService(t, effectiveDefault)
	f.Respond(notRunning, "pgrep", "-u", "alice")
	if err := svc.DeleteSudoUser(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if ex := f.Execs(); len(ex) == 0 || ex[len(ex)-1] != "userdel --remove -- alice" {
		t.Errorf("execs = %v", ex)
	}

	if err := svc.DeleteSudoUser(ctx, "root"); !errors.Is(err, ErrDeleteRoot) {
		t.Errorf("root: %v", err)
	}
	if err := svc.DeleteSudoUser(ctx, "svc"); !errors.Is(err, ErrNotAdmin) {
		t.Errorf("svc: %v", err)
	}

	// Root login forbidden: alice is the only way in.
	f, svc = loginService(t, "port 22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin no\n")
	f.Respond(notRunning, "pgrep", "-u", "alice")
	f.Files[ShadowPath] = []byte("root:$y$hash:1::::::\nalice:$y$a:1::::::\n")
	if err := svc.DeleteSudoUser(ctx, "alice"); !errors.Is(err, ErrDeleteLockout) || errors.Is(err, ErrLockout) {
		t.Errorf("lockout: %v", err)
	}

	// A logged-in user is not deleted.
	f, svc = loginService(t, effectiveDefault)
	if err := svc.DeleteSudoUser(ctx, "alice"); !errors.Is(err, ErrUserInUse) {
		t.Errorf("in use: %v", err)
	}
	for _, e := range f.Execs() {
		if strings.HasPrefix(e, "userdel") {
			t.Errorf("deleted anyway: %v", f.Execs())
		}
	}
}
