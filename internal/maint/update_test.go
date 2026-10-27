package maint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/xray"
)

type fakeFetcher map[string][]byte

func (f fakeFetcher) Fetch(_ context.Context, url string, _ int64) ([]byte, error) {
	if d, ok := f[url]; ok {
		return d, nil
	}
	return nil, errors.New("404 " + url)
}

func (f fakeFetcher) Redirect(_ context.Context, url string) (string, error) {
	if d, ok := f[url]; ok {
		return string(d), nil
	}
	return "", errors.New("404 " + url)
}

func release(tag string, bin []byte) fakeFetcher {
	sum := sha256.Sum256(bin)
	return fakeFetcher{
		xray.LatestPageURL(Repo):              []byte("https://github.com/" + Repo + "/releases/tag/" + tag),
		AssetURL(tag, "litenode-linux-amd64"): bin,
		AssetURL(tag, "sha256sums.txt"):       []byte("0000000000000000000000000000000000000000000000000000000000000000  litenode-linux-arm64\n" + hex.EncodeToString(sum[:]) + "  litenode-linux-amd64\n"),
	}
}

func TestUpdateAvailable(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"v0.0.2", "v0.1.0", true}, {"v0.1.0", "v0.1.0", false}, {"v0.1.1", "v0.1.0", false}, {"dev", "v0.1.0", true},
	} {
		if got := UpdateAvailable(c.cur, c.latest); got != c.want {
			t.Errorf("UpdateAvailable(%s, %s) = %v", c.cur, c.latest, got)
		}
	}
}

func TestParseSums(t *testing.T) {
	h := "ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if got, err := ParseSums([]byte(h+" *litenode-linux-amd64\n"), "litenode-linux-amd64"); err != nil || got != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := ParseSums([]byte(h+"  other\n"), "litenode-linux-amd64"); err == nil {
		t.Error("missing entry accepted")
	}
	if _, err := ParseSums([]byte("xyz  litenode-linux-amd64\n"), "litenode-linux-amd64"); err == nil {
		t.Error("bad digest accepted")
	}
}

func TestUpdate(t *testing.T) {
	f := systemtest.New()
	f.Files[BinPath] = []byte("old")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "LiteNode v0.1.0 (linux/amd64)\n"}}, newPath, "version")
	u := &Updater{Runner: f, Fetcher: release("v0.1.0", []byte("new")), Arch: "amd64"}
	ctx := context.Background()
	tag, err := u.Latest(ctx)
	if err != nil || tag != "v0.1.0" {
		t.Fatalf("latest %q, %v", tag, err)
	}
	if err := u.Update(ctx, tag); err != nil {
		t.Fatal(err)
	}
	if string(f.Files[BinPath]) != "new" || string(f.Files[BakPath]) != "old" || f.Modes[BakPath] != 0o755 {
		t.Errorf("bin %q bak %q", f.Files[BinPath], f.Files[BakPath])
	}
	if _, ok := f.Files[newPath]; ok {
		t.Error(".new left behind")
	}
}

func TestUpdateFailedChecksChangeNothing(t *testing.T) {
	ctx := context.Background()
	// Tampered binary: the digest does not match.
	f := systemtest.New()
	f.Files[BinPath] = []byte("old")
	fetch := release("v0.1.0", []byte("new"))
	fetch[AssetURL("v0.1.0", "litenode-linux-amd64")] = []byte("evil")
	if err := (&Updater{Runner: f, Fetcher: fetch, Arch: "amd64"}).Update(ctx, "v0.1.0"); !errors.Is(err, ErrDigestMismatch) {
		t.Errorf("err = %v", err)
	}
	if len(f.Mutations()) != 0 {
		t.Errorf("mutations after digest mismatch: %v", f.Mutations())
	}
	// The binary does not report the version: removed, nothing replaced.
	f = systemtest.New()
	f.Files[BinPath] = []byte("old")
	f.Respond(systemtest.Response{Err: errors.New("exec format error")}, newPath, "version")
	if err := (&Updater{Runner: f, Fetcher: release("v0.1.0", []byte("new")), Arch: "amd64"}).Update(ctx, "v0.1.0"); !errors.Is(err, ErrBadBinary) {
		t.Errorf("err = %v", err)
	}
	if string(f.Files[BinPath]) != "old" || f.Files[BakPath] != nil || f.Files[newPath] != nil {
		t.Errorf("files changed: bin %q", f.Files[BinPath])
	}
	// No release for the architecture.
	if err := (&Updater{Runner: f, Fetcher: fetch, Arch: "riscv64"}).Update(ctx, "v0.1.0"); err == nil {
		t.Error("riscv64 accepted")
	}
}
