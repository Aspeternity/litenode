package ssh

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const passwd = "root:x:0:0:root:/root:/bin/bash\n" +
	"daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin\n" +
	"alice:x:1000:1000:Alice,,,:/home/alice:/bin/bash\n" +
	"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin\n" +
	"svc:x:1001:1001::/home/svc:/bin/false\n"

func TestAccounts(t *testing.T) {
	var names []string
	for _, a := range ParsePasswd([]byte(passwd)) {
		if a.CanLogIn() {
			names = append(names, a.Name)
		}
	}
	if !slices.Equal(names, []string{"root", "alice"}) {
		t.Errorf("login accounts = %v", names)
	}
	f := systemtest.New()
	f.Files[PasswdPath] = []byte(passwd)
	a, err := LookupAccount(f, "alice")
	if err != nil || a.UID != 1000 || a.AuthorizedKeysPath() != "/home/alice/.ssh/authorized_keys" {
		t.Errorf("alice = %+v, %v", a, err)
	}
	if _, err := LookupAccount(f, "bob"); !errors.Is(err, ErrNoAccount) {
		t.Errorf("bob: %v", err)
	}
}

type fakeFetcher map[string]string

func (f fakeFetcher) Fetch(_ context.Context, url string, _ int64) ([]byte, error) {
	if s, ok := f[url]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("HTTP 404")
}

func keyService(t *testing.T, effective string) (*systemtest.Fake, *Service) {
	t.Helper()
	f := sshSystem(effective)
	f.Files[PasswdPath] = []byte(passwd)
	store := state.NewStore(f)
	return f, &Service{Store: store, Confirmer: change.NewConfirmer(change.NewManager(f, store), "/usr/local/bin/litenode"),
		Fetcher: fakeFetcher{}, Rand: rand.Reader}
}

func alice(t *testing.T, f *systemtest.Fake) Account {
	t.Helper()
	a, err := LookupAccount(f, "alice")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAddKeysCreatesDirAndFixesOwnership(t *testing.T) {
	f, svc := keyService(t, effectiveDefault)
	a := alice(t, f)
	k, _ := ParseKey(readPub(t, "ed25519"))
	n, err := svc.AddKeys(context.Background(), a, []Key{k})
	if err != nil || n != 1 {
		t.Fatalf("added %d, %v", n, err)
	}
	if got := string(f.Files[a.AuthorizedKeysPath()]); got != k.Line()+"\n" {
		t.Errorf("authorized_keys = %q", got)
	}
	execs := f.Execs()
	for _, want := range []string{"chown 1000:1000 /home/alice/.ssh", "chown 1000:1000 /home/alice/.ssh/authorized_keys"} {
		if !slices.Contains(execs, want) {
			t.Errorf("missing %q in %v", want, execs)
		}
	}
	modes := map[string]bool{}
	for _, c := range f.Calls() {
		if c.Op == systemtest.OpChmod {
			modes[c.Path+" "+c.Perm.String()] = true
		}
	}
	if !modes["/home/alice/.ssh -rwx------"] || !modes["/home/alice/.ssh/authorized_keys -rw-------"] {
		t.Errorf("chmod calls = %v", modes)
	}
	// Adding the same key again changes nothing.
	before := len(f.Mutations())
	if n, err := svc.AddKeys(context.Background(), a, []Key{k}); n != 0 || err != nil || len(f.Mutations()) != before {
		t.Errorf("re-add: %d %v", n, err)
	}
	// No confirm-or-rollback for adding.
	if _, ok := f.Files[change.DefaultPendingDir+"/pending"]; ok {
		t.Error("adding a key armed a rollback")
	}
}

func TestDeleteKey(t *testing.T) {
	ed, _ := ParseKey(readPub(t, "ed25519"))
	rsa, _ := ParseKey(readPub(t, "rsa"))
	ctx := context.Background()

	// Password login off: the last key cannot go.
	f, svc := keyService(t, "port 22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin yes\n")
	a := alice(t, f)
	svc.AddKeys(ctx, a, []Key{ed, rsa})
	if err := svc.DeleteKey(ctx, a, rsa.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[change.DefaultPendingDir+"/pending"]; !ok {
		t.Error("deleting a key did not arm a rollback")
	}
	svc.Confirmer.Disarm(ctx)
	if err := svc.DeleteKey(ctx, a, ed.Fingerprint()); !errors.Is(err, ErrLastKey) {
		t.Errorf("last key with password off: %v", err)
	}
	if err := svc.DeleteKey(ctx, a, "SHA256:nope"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("unknown key: %v", err)
	}

	// Password login on: the last key may go.
	f, svc = keyService(t, effectiveDefault)
	a = alice(t, f)
	svc.AddKeys(ctx, a, []Key{ed})
	if err := svc.DeleteKey(ctx, a, ed.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	if keys, _ := svc.Keys(a); len(keys) != 0 {
		t.Errorf("keys left: %v", keys)
	}
}

func TestGitHubKeys(t *testing.T) {
	_, svc := keyService(t, effectiveDefault)
	ed := strings.Fields(readPub(t, "ed25519"))
	svc.Fetcher = fakeFetcher{GitHubKeysURL("octo-cat"): ed[0] + " " + ed[1] + "\n" + readPub(t, "rsa1024") + "\n\n" + readPub(t, "rsa") + "\n"}
	keys, skipped, err := svc.GitHubKeys(context.Background(), "octo-cat")
	if err != nil || len(keys) != 2 || skipped != 1 {
		t.Fatalf("keys=%d skipped=%d err=%v", len(keys), skipped, err)
	}
	if keys[0].Comment != "github:octo-cat" || keys[1].Comment != "test-rsa" {
		t.Errorf("comments %q %q", keys[0].Comment, keys[1].Comment)
	}
	for _, bad := range []string{"", "-x", "x-", "a/b", "a b", strings.Repeat("a", 40), "../etc"} {
		if _, _, err := svc.GitHubKeys(context.Background(), bad); !errors.Is(err, ErrGitHubUser) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	svc.Fetcher = fakeFetcher{GitHubKeysURL("empty"): "\n"}
	if _, _, err := svc.GitHubKeys(context.Background(), "empty"); !errors.Is(err, ErrGitHubNone) {
		t.Errorf("no keys: %v", err)
	}
}

func TestGenerateKeyAuthorizes(t *testing.T) {
	f, svc := keyService(t, effectiveDefault)
	a := alice(t, f)
	k, priv, err := svc.GenerateKey(context.Background(), a, "litenode@vps")
	if err != nil || len(priv) == 0 {
		t.Fatal(err)
	}
	if keys, _ := svc.Keys(a); len(keys) != 1 || keys[0].Fingerprint() != k.Fingerprint() {
		t.Errorf("keys = %v", keys)
	}
	for p, data := range f.Files {
		if strings.Contains(string(data), "PRIVATE KEY") {
			t.Errorf("private key written to %s", p)
		}
	}
}

func TestAuthKeysKindRepairsAfterRestore(t *testing.T) {
	f := systemtest.New()
	f.Files[PasswdPath] = []byte(passwd)
	f.Files["/root/.ssh/authorized_keys"] = []byte("x\n")
	if err := AuthKeysKind().Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// Root's existing file is fixed; alice has none, so nothing for her.
	execs := strings.Join(f.Execs(), "\n")
	if !strings.Contains(execs, "chown 0:0 /root/.ssh/authorized_keys") || strings.Contains(execs, "alice") {
		t.Errorf("execs:\n%s", execs)
	}
}
