package xray

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

// fakeFetcher serves URLs from memory.
type fakeFetcher map[string][]byte

func (f fakeFetcher) Fetch(_ context.Context, url string, limit int64) ([]byte, error) {
	data, ok := f[url]
	if !ok {
		return nil, fmt.Errorf("404 %s", url)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", url, limit)
	}
	return data, nil
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// publish adds a release with a correct digest to the fetcher.
func publish(t *testing.T, f fakeFetcher, version, program string) {
	t.Helper()
	archive := makeZip(t, map[string]string{"xray": program, "geoip.dat": "geo", "LICENSE": "MPL"})
	sum := sha256.Sum256(archive)
	url := DownloadURL(version, "Xray-linux-64.zip")
	f[url] = archive
	f[url+".dgst"] = []byte("MD5= x\nSHA2-256= " + hex.EncodeToString(sum[:]) + "\n")
}

type fixture struct {
	r  *systemtest.Fake
	f  fakeFetcher
	in *Installer
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{r: systemtest.New(), f: fakeFetcher{}}
	fx.in = &Installer{Runner: fx.r, Fetcher: fx.f, Arch: "amd64", Sleep: func(time.Duration) {}}
	fx.f[LatestReleaseURL] = []byte(`{"tag_name": "v26.3.27"}`)
	publish(t, fx.f, "v26.3.27", "program-26.3.27")
	publish(t, fx.f, "v26.2.6", "program-26.2.6")
	// "xray version" reports whatever version was last written.
	fx.reportVersion("")
	return fx
}

// reportVersion makes "xray version" answer as version ("" = not installed).
func (fx *fixture) reportVersion(v string) {
	if v == "" {
		fx.r.Respond(systemtest.Response{Err: errors.New("no such file")}, BinPath, "version")
		return
	}
	fx.r.Respond(systemtest.Response{Result: system.Result{
		Stdout: "Xray " + strings.TrimPrefix(v, "v") + " (Xray, Penetrates Everything.) abc (go1.26 linux/amd64)\n"}},
		BinPath, "version")
}

func TestInstallLatest(t *testing.T) {
	fx := newFixture(t)
	fx.r.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "id", "-u", User)
	fx.reportVersion("v26.3.27")

	v, err := fx.in.Install(context.Background(), "")
	if err != nil || v != "v26.3.27" {
		t.Fatalf("Install = (%q, %v)", v, err)
	}
	if string(fx.r.Files[BinPath]) != "program-26.3.27" || fx.r.Modes[BinPath] != 0o755 {
		t.Errorf("program not installed correctly: %q %04o", fx.r.Files[BinPath], fx.r.Modes[BinPath])
	}
	if string(fx.r.Files[UnitPath]) != Unit() {
		t.Error("unit file not written")
	}
	for p := range fx.r.Files {
		if strings.HasSuffix(p, ".dat") {
			t.Errorf("geo file installed: %s (D-008)", p)
		}
	}
	if fx.r.Modes["/etc/litenode"] != 0o711 {
		t.Errorf("/etc/litenode mode = %04o, want 0711", fx.r.Modes["/etc/litenode"])
	}
	want := []string{
		"useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin litenode-xray",
		"systemctl daemon-reload",
	}
	if got := fx.r.Execs(); !slices.Equal(got, want) {
		t.Errorf("execs = %q, want %q", got, want)
	}
}

func TestInstallSpecificVersionExistingUser(t *testing.T) {
	fx := newFixture(t)
	fx.reportVersion("v26.2.6")
	v, err := fx.in.Install(context.Background(), "26.2.6")
	if err != nil || v != "v26.2.6" || string(fx.r.Files[BinPath]) != "program-26.2.6" {
		t.Fatalf("Install = (%q, %v)", v, err)
	}
	if slices.ContainsFunc(fx.r.Execs(), func(s string) bool { return strings.HasPrefix(s, "useradd") }) {
		t.Error("useradd ran for an existing user")
	}
}

func TestInstallDigestMismatch(t *testing.T) {
	fx := newFixture(t)
	url := DownloadURL("v26.3.27", "Xray-linux-64.zip")
	fx.f[url] = append(fx.f[url], "tampered"...)
	if _, err := fx.in.Install(context.Background(), ""); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("err = %v, want ErrDigestMismatch", err)
	}
	if m := fx.r.Mutations(); len(m) != 0 {
		t.Errorf("mismatch must not touch the system: %v", m)
	}
}

func TestInstallBrokenProgramCleansUp(t *testing.T) {
	fx := newFixture(t)
	// The program never reports a version (e.g. wrong architecture).
	if _, err := fx.in.Install(context.Background(), ""); err == nil {
		t.Fatal("want error")
	}
	for _, p := range []string{BinPath, UnitPath} {
		if _, ok := fx.r.Files[p]; ok {
			t.Errorf("%s left behind", p)
		}
	}
}

func TestUpdate(t *testing.T) {
	fx := newFixture(t)
	fx.r.Files[BinPath] = []byte("program-26.3.27")
	fx.r.Files[ConfigPath] = []byte("{}")
	// The fake answers "xray version" the same way every time, so it
	// reports the target version both before and after the update.
	v := "v26.2.6"
	fx.reportVersion(v)

	got, err := fx.in.Update(context.Background(), "v26.2.6")
	if err != nil || got != v {
		t.Fatalf("Update = (%q, %v)", got, err)
	}
	if string(fx.r.Files[BinPath]) != "program-26.2.6" {
		t.Errorf("program = %q", fx.r.Files[BinPath])
	}
	if _, ok := fx.r.Files[BinPath+".bak"]; ok {
		t.Error("backup left after successful update")
	}
	// The service was running: it is restarted and checked.
	execs := strings.Join(fx.r.Execs(), "\n")
	if !strings.Contains(execs, "systemctl restart litenode-xray.service") {
		t.Errorf("service not restarted: %s", execs)
	}
}

func TestUpdateRevertsWhenConfigRejected(t *testing.T) {
	fx := newFixture(t)
	fx.r.Files[BinPath] = []byte("program-26.3.27")
	fx.r.Files[ConfigPath] = []byte("{}")
	fx.reportVersion("v26.2.6")
	fx.r.Respond(systemtest.Response{Result: system.Result{ExitCode: 23, Stderr: "unknown field"}},
		BinPath, "run", "-test", "-c", ConfigPath)

	if _, err := fx.in.Update(context.Background(), "v26.2.6"); err == nil {
		t.Fatal("want error")
	}
	if string(fx.r.Files[BinPath]) != "program-26.3.27" {
		t.Errorf("old program not restored: %q", fx.r.Files[BinPath])
	}
	if _, ok := fx.r.Files[BinPath+".bak"]; ok {
		t.Error("backup left behind")
	}
}

func TestUpdateRevertsWhenServiceDies(t *testing.T) {
	fx := newFixture(t)
	fx.r.Files[BinPath] = []byte("program-26.3.27")
	fx.reportVersion("v26.2.6")
	// Running before the update, dead after every restart.
	calls := 0
	fx.r.Respond(systemtest.Response{}, "systemctl", "is-active", "--quiet", UnitName)
	in := fx.in
	in.Sleep = func(time.Duration) {
		calls++
		fx.r.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", UnitName)
	}
	_, err := in.Update(context.Background(), "v26.2.6")
	if err == nil || !strings.Contains(err.Error(), "reverting") {
		t.Fatalf("err = %v, want update and revert failure reported", err)
	}
	if string(fx.r.Files[BinPath]) != "program-26.3.27" {
		t.Errorf("old program not restored: %q", fx.r.Files[BinPath])
	}
	if !slices.Contains(fx.r.Execs(), "systemctl reset-failed litenode-xray.service") {
		t.Error("start rate limit not reset before restarting the old program")
	}
	if n := strings.Count(strings.Join(fx.r.Execs(), "\n"), "systemctl restart"); n != 2 || calls != 2 {
		t.Errorf("restarts = %d (sleeps %d), want 2: new program, then old", n, calls)
	}
}

func TestUpdateNotInstalled(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.in.Update(context.Background(), ""); err == nil {
		t.Error("want error when xray is not installed")
	}
}

func TestUninstall(t *testing.T) {
	fx := newFixture(t)
	for _, p := range []string{BinPath, UnitPath, ConfigPath} {
		fx.r.Files[p] = []byte("x")
	}
	if err := fx.in.Uninstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{BinPath, UnitPath, ConfigPath, LibDir} {
		if _, ok := fx.r.Files[p]; ok {
			t.Errorf("%s not removed", p)
		}
	}
	want := []string{
		"systemctl stop litenode-xray.service",
		"systemctl disable litenode-xray.service",
		"systemctl daemon-reload",
		"userdel litenode-xray",
	}
	if got := fx.r.Execs(); !slices.Equal(got, want) {
		t.Errorf("execs = %q, want %q", got, want)
	}
}

func TestLatestWithLongReleaseNotes(t *testing.T) {
	fx := newFixture(t)
	// Real API responses carry the full release notes (well over 64 KiB).
	notes := strings.Repeat("x", 300<<10)
	fx.f[LatestReleaseURL] = []byte(`{"tag_name": "v26.3.27", "body": "` + notes + `"}`)
	if v, err := fx.in.Latest(context.Background()); err != nil || v != "v26.3.27" {
		t.Errorf("Latest = (%q, %v)", v, err)
	}
}

func TestUpdateDownloadFailed(t *testing.T) {
	fx := newFixture(t)
	fx.r.Files[BinPath] = []byte("program-26.3.27")
	fx.reportVersion("v26.3.27")
	_, err := fx.in.Update(context.Background(), "v9.9.9")
	var de *DownloadError
	if !errors.As(err, &de) {
		t.Fatalf("Update = %v, want a DownloadError", err)
	}
	if string(fx.r.Files[BinPath]) != "program-26.3.27" {
		t.Errorf("program changed: %q", fx.r.Files[BinPath])
	}
}
