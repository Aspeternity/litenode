package ssh

import (
	"context"
	"errors"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestCheckLockout(t *testing.T) {
	open := Policy{Pubkey: true, Password: true, Root: RootYes}
	rootKey := Admin{Name: "root", Root: true, Keys: 1}
	rootPass := Admin{Name: "root", Root: true, Password: true}
	sudoKey := Admin{Name: "alice", Keys: 1}
	sudoPass := Admin{Name: "alice", Password: true}
	with := func(p Policy, f func(*Policy)) Policy { f(&p); return p }
	for _, tc := range []struct {
		name     string
		old, new Policy
		admins   []Admin
		want     error
	}{
		// Turning password login off (DESIGN: key login on and a key).
		{"password off with root key", open, with(open, func(p *Policy) { p.Password = false }), []Admin{rootKey}, nil},
		{"password off, key login off", Policy{Password: true, Root: RootYes}, Policy{Root: RootYes}, []Admin{rootKey}, ErrNeedKeyLogin},
		{"password off, no keys", open, with(open, func(p *Policy) { p.Password = false }), []Admin{rootPass, sudoPass}, ErrNeedKey},
		{"password off, root key but root forbidden", with(open, func(p *Policy) { p.Root = RootNo }),
			Policy{Pubkey: true, Root: RootNo}, []Admin{rootKey, sudoPass}, ErrNeedKey},
		{"password off, sudo user key", with(open, func(p *Policy) { p.Root = RootNo }),
			Policy{Pubkey: true, Root: RootNo}, []Admin{rootKey, sudoKey}, nil},
		// Turning key login off (DESIGN: password login on).
		{"key off, password off", Policy{Pubkey: true, Root: RootYes}, Policy{Root: RootYes}, []Admin{rootKey, rootPass}, ErrNeedPasswordLogin},
		{"key off, password on", open, with(open, func(p *Policy) { p.Pubkey = false }), []Admin{rootPass}, nil},
		{"key off, password on but nobody has one", open, with(open, func(p *Policy) { p.Pubkey = false }), []Admin{rootKey}, ErrLockout},
		// Forbidding root (P-003: a sudo user that can log in).
		{"root no, no sudo user", open, with(open, func(p *Policy) { p.Root = RootNo }), []Admin{rootKey, rootPass}, ErrNeedSudoUser},
		{"root no, sudo user without credentials", open, with(open, func(p *Policy) { p.Root = RootNo }), []Admin{rootPass, {Name: "bob"}}, ErrNeedSudoUser},
		{"root no, sudo user with password", open, with(open, func(p *Policy) { p.Root = RootNo }), []Admin{rootPass, sudoPass}, nil},
		{"root no, sudo key but key login off", Policy{Password: true, Root: RootYes}, Policy{Password: true, Root: RootNo}, []Admin{sudoKey}, ErrNeedSudoUser},
		// Root keys only: root needs a key, or a sudo user remains.
		{"root keys-only without root key", open, with(open, func(p *Policy) { p.Root = RootKeysOnly }), []Admin{rootPass}, ErrLockout},
		{"root keys-only with root key", open, with(open, func(p *Policy) { p.Root = RootKeysOnly }), []Admin{rootKey}, nil},
		{"root keys-only, sudo user remains", open, with(open, func(p *Policy) { p.Root = RootKeysOnly }), []Admin{rootPass, sudoPass}, nil},
		// Unrelated changes on a usable system are fine.
		{"no change", open, open, []Admin{rootPass}, nil},
	} {
		if err := CheckLockout(tc.old, tc.new, tc.admins); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestShadowAndGroup(t *testing.T) {
	sh := ParseShadow([]byte("root:$y$j9T$abc:19000:0:99999:7:::\nalice:!$y$xyz:19000::::::\nbob:*:19000::::::\ncarol::19000::::::\n"))
	if !sh["root"] || sh["alice"] || sh["bob"] || sh["carol"] {
		t.Errorf("shadow = %v", sh)
	}
	gid, members, ok := ParseGroupMembers([]byte("root:x:0:\nsudo:x:27:alice,bob\n"), "sudo")
	if !ok || gid != 27 || len(members) != 2 || members[1] != "bob" {
		t.Errorf("sudo = %d %v %v", gid, members, ok)
	}
	if _, _, ok := ParseGroupMembers([]byte("root:x:0:\n"), "sudo"); ok {
		t.Error("missing group found")
	}
}

func loginService(t *testing.T, effective string) (*systemtest.Fake, *Service) {
	t.Helper()
	f, svc := keyService(t, effective)
	f.Files[GroupPath] = []byte("root:x:0:\nsudo:x:27:alice\nalice:x:1000:\n")
	f.Files[ShadowPath] = []byte("root:$y$hash:19000:0:99999:7:::\nalice:!:19000::::::\nsvc:*:1::::::\n")
	return f, svc
}

func TestAdmins(t *testing.T) {
	f, svc := loginService(t, effectiveDefault)
	k, _ := ParseKey(readPub(t, "ed25519"))
	svc.AddKeys(context.Background(), alice(t, f), []Key{k})
	admins, err := svc.Admins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 2 || admins[0] != (Admin{Name: "root", Root: true, Password: true}) || admins[1] != (Admin{Name: "alice", Keys: 1}) {
		t.Errorf("admins = %+v", admins)
	}
}

func TestSetPasswordAuthInterlock(t *testing.T) {
	ctx := context.Background()
	// Root has only a password: turning password login off is refused
	// before anything changes.
	f, svc := loginService(t, effectiveDefault)
	if err := svc.SetPasswordAuth(ctx, false); !errors.Is(err, ErrNeedKey) {
		t.Fatalf("got %v", err)
	}
	for _, m := range f.Mutations() {
		if m.Path != PrivsepDir {
			t.Errorf("refused change touched the system: %+v", m)
		}
	}
	// With a key for root it goes ahead under confirm-or-rollback.
	f, svc = loginService(t, "port 22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin yes\n")
	f.Files["/root/.ssh/authorized_keys"] = []byte(readPub(t, "ed25519") + "\n")
	if err := svc.SetPasswordAuth(ctx, false); err != nil {
		t.Fatal(err)
	}
	st, _ := svc.Store.Load()
	if st.SSH.PasswordAuth == nil || *st.SSH.PasswordAuth {
		t.Errorf("state = %+v", st.SSH)
	}
	if _, ok := f.Files["/etc/litenode/rollback/pending"]; !ok {
		t.Error("no rollback armed")
	}
}

func TestSetRootLogin(t *testing.T) {
	ctx := context.Background()
	f, svc := loginService(t, effectiveDefault)
	if err := svc.SetRootLogin(ctx, "maybe"); !errors.Is(err, ErrRootPolicy) {
		t.Errorf("bad policy: %v", err)
	}
	// alice is a sudo user without key or password: forbidding root is
	// refused (P-003).
	if err := svc.SetRootLogin(ctx, RootNo); !errors.Is(err, ErrNeedSudoUser) {
		t.Errorf("root no: %v", err)
	}
	k, _ := ParseKey(readPub(t, "ed25519"))
	svc.AddKeys(ctx, alice(t, f), []Key{k})
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "port 22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin no\n"}}, SshdPath, "-T")
	if err := svc.SetRootLogin(ctx, RootNo); err != nil {
		t.Fatalf("root no with a sudo user key: %v", err)
	}
	if st, _ := svc.Store.Load(); st.SSH.PermitRootLogin != RootNo {
		t.Errorf("state = %+v", st.SSH)
	}
}

func TestSetLoginMethod(t *testing.T) {
	ctx := context.Background()
	// Key only without any key is refused (D-090).
	_, svc := loginService(t, effectiveDefault)
	if err := svc.SetLoginMethod(ctx, true, false); !errors.Is(err, ErrNeedKey) {
		t.Fatalf("key only without a key = %v, want ErrNeedKey", err)
	}
	// Password only: both settings are written in one step (the fake sshd
	// reports the result).
	_, svc = loginService(t, "port 22\npubkeyauthentication no\npasswordauthentication yes\npermitrootlogin yes\n")
	if err := svc.SetLoginMethod(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	st, _ := svc.Store.Load()
	if st.SSH.PubkeyAuth == nil || *st.SSH.PubkeyAuth || st.SSH.PasswordAuth == nil || !*st.SSH.PasswordAuth {
		t.Errorf("state = %+v", st.SSH)
	}
}
