package change

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const (
	confPath  = "/etc/litenode/xray.json"
	nftPath   = "/etc/litenode/nftables.nft"
	statePath = "/etc/litenode/state.json"
	backupDir = "/etc/litenode/backups"
)

type fixture struct {
	f     *systemtest.Fake
	m     *Manager
	clock time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fx := &fixture{f: systemtest.New(), clock: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	store := &state.Store{Runner: fx.f, Path: statePath, BackupDir: backupDir, Now: time.Now}
	fx.m = &Manager{Runner: fx.f, Store: store, BackupDir: backupDir, Keep: DefaultKeep,
		Now: func() time.Time { fx.clock = fx.clock.Add(time.Second); return fx.clock }}
	fx.f.Files[confPath] = []byte("old-config")
	fx.f.Files[nftPath] = []byte("old-rules")
	fx.f.Files[statePath] = []byte(`{"schema_version": 1, "next_id": 1, "server": {"share_address": "old"}}`)
	return fx
}

func newState(addr string) *state.State {
	st := state.New()
	st.Server.ShareAddress = addr
	return st
}

// xrayKind validates and applies by running fake commands so tests can
// make either step fail through canned responses.
func xrayKind(inPlace bool) *Kind {
	return &Kind{
		Name:            "xray",
		ValidateInPlace: inPlace,
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			_, err := r.Query(ctx, "xray", "run", "-test", "-c", paths[confPath])
			return err
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			_, err := r.Exec(ctx, "systemctl", "restart", "litenode-xray.service")
			return err
		},
	}
}

func nftKind() *Kind {
	return &Kind{
		Name: "nft",
		Validate: func(ctx context.Context, r system.Runner, paths map[string]string) error {
			_, err := r.Query(ctx, "nft", "-c", "-f", paths[nftPath])
			return err
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			_, err := r.Exec(ctx, "nft", "-f", nftPath)
			return err
		},
	}
}

// ufwKind has no files, only Apply (reconciliation).
func ufwKind() *Kind {
	return &Kind{Name: "ufw", Apply: func(ctx context.Context, r system.Runner) error {
		_, err := r.Exec(ctx, "ufw-reconcile")
		return err
	}}
}

func xrayPart(data string) Part {
	return Part{Kind: xrayKind(false), Files: []File{{Path: confPath, Data: []byte(data), Mode: 0o600}}}
}

func nftPart(data string) Part {
	return Part{Kind: nftKind(), Files: []File{{Path: nftPath, Data: []byte(data), Mode: 0o600}}}
}

// threeKinds is "add a node": xray + nft + ufw.
func threeKinds() Change {
	return Change{Parts: []Part{xrayPart("new-config"), nftPart("new-rules"), {Kind: ufwKind()}}, State: newState("new")}
}

func (fx *fixture) assertOriginal(t *testing.T) {
	t.Helper()
	if got := string(fx.f.Files[confPath]); got != "old-config" {
		t.Errorf("xray config = %q, want restored", got)
	}
	if got := string(fx.f.Files[nftPath]); got != "old-rules" {
		t.Errorf("nft rules = %q, want restored", got)
	}
	if got := string(fx.f.Files[statePath]); !strings.Contains(got, `"old"`) {
		t.Errorf("state = %q, want restored original", got)
	}
	for p := range fx.f.Files {
		if strings.Contains(p, stagedMarker) {
			t.Errorf("staged file left behind: %s", p)
		}
	}
}

func commands(f *systemtest.Fake) []string {
	var cmds []string
	for _, c := range f.Calls() {
		if c.Command != "" {
			cmds = append(cmds, c.Command)
		}
	}
	return cmds
}

func TestApplyMultiKindSuccess(t *testing.T) {
	fx := newFixture(t)
	id, err := fx.m.Apply(context.Background(), threeKinds())
	if err != nil {
		t.Fatal(err)
	}
	if string(fx.f.Files[confPath]) != "new-config" || string(fx.f.Files[nftPath]) != "new-rules" ||
		!strings.Contains(string(fx.f.Files[statePath]), `"new"`) {
		t.Error("change not applied")
	}
	// Everything is validated before anything is applied, then kinds are
	// applied in order.
	want := []string{
		"xray run -test -c /etc/litenode/xray.litenode-new.json",
		"nft -c -f /etc/litenode/nftables.litenode-new.nft",
		"systemctl restart litenode-xray.service",
		"nft -f /etc/litenode/nftables.nft",
		"ufw-reconcile",
	}
	if got := commands(fx.f); !slices.Equal(got, want) {
		t.Errorf("commands =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.HasPrefix(id, "xray+nft+ufw-20261018-120001") {
		t.Errorf("backup id = %q", id)
	}
	b, err := fx.m.LoadBackup(id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Kinds, []string{"xray", "nft", "ufw"}) || len(b.Files) != 2 || !b.StateExisted {
		t.Errorf("backup = %+v", b)
	}
	if string(fx.f.Files[b.Dir+"/xray.json"]) != "old-config" || string(fx.f.Files[b.Dir+"/nftables.nft"]) != "old-rules" {
		t.Error("backup does not hold the original files")
	}
}

func TestValidateFailureInAnyKindRestoresAll(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inPlace bool
		fail    []string
	}{
		{"first kind", false, []string{"xray", "run", "-test", "-c", "/etc/litenode/xray.litenode-new.json"}},
		{"first kind in place", true, []string{"xray", "run", "-test", "-c", confPath}},
		{"second kind", false, []string{"nft", "-c", "-f", "/etc/litenode/nftables.litenode-new.nft"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			fx.f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, tc.fail[0], tc.fail[1:]...)
			c := threeKinds()
			c.Parts[0].Kind = xrayKind(tc.inPlace)
			_, err := fx.m.Apply(context.Background(), c)
			var ce *Error
			if !errors.As(err, &ce) || ce.Stage != StageValidate || ce.RestoreErr != nil {
				t.Fatalf("err = %v, want validate error with clean restore", err)
			}
			fx.assertOriginal(t)
		})
	}
}

func TestApplyFailureInLaterKindRestoresAndReappliesAll(t *testing.T) {
	fx := newFixture(t)
	fx.f.Respond(systemtest.Response{Err: errors.New("ufw broke")}, "ufw-reconcile")

	_, err := fx.m.Apply(context.Background(), threeKinds())
	var ce *Error
	if !errors.As(err, &ce) || ce.Stage != StageApply || ce.Kind != "ufw" {
		t.Fatalf("err = %v, want ufw apply error", err)
	}
	fx.assertOriginal(t)
	// The xray and nft changes had already taken effect: after the restore
	// every kind is applied again, in order, with the old configuration.
	cmds := commands(fx.f)
	tail := cmds[len(cmds)-3:]
	want := []string{"systemctl restart litenode-xray.service", "nft -f /etc/litenode/nftables.nft", "ufw-reconcile"}
	if !slices.Equal(tail, want) {
		t.Errorf("reapply = %q, want %q", tail, want)
	}
}

func TestApplyFailureRemovesNewFiles(t *testing.T) {
	fx := newFixture(t)
	delete(fx.f.Files, confPath)
	delete(fx.f.Files, statePath)
	fx.f.WriteErr[statePath] = errors.New("disk full")

	_, err := fx.m.Apply(context.Background(), threeKinds())
	if err == nil {
		t.Fatal("want error")
	}
	if _, ok := fx.f.Files[confPath]; ok {
		t.Error("file created by the failed change was not removed")
	}
	if _, ok := fx.f.Files[statePath]; ok {
		t.Error("state file created by the failed change was not removed")
	}
	if string(fx.f.Files[nftPath]) != "old-rules" {
		t.Error("existing file of another kind not restored")
	}
}

func TestRemoveFileRestored(t *testing.T) {
	fx := newFixture(t)
	fx.f.Respond(systemtest.Response{Err: errors.New("boom")}, "systemctl", "restart", "litenode-xray.service")
	_, err := fx.m.Apply(context.Background(), Change{Parts: []Part{
		{Kind: xrayKind(false), Files: []File{{Path: confPath, Remove: true, Mode: 0o600}}},
	}})
	if err == nil {
		t.Fatal("want error")
	}
	if string(fx.f.Files[confPath]) != "old-config" {
		t.Error("removed file was not restored")
	}
}

func TestApplyKindWithoutFiles(t *testing.T) {
	fx := newFixture(t)
	id, err := fx.m.Apply(context.Background(), Change{Parts: []Part{{Kind: ufwKind()}}, State: newState("new")})
	if err != nil {
		t.Fatal(err)
	}
	if got := commands(fx.f); !slices.Equal(got, []string{"ufw-reconcile"}) {
		t.Errorf("commands = %q", got)
	}
	if !strings.HasPrefix(id, "ufw-") {
		t.Errorf("backup id = %q", id)
	}
}

func TestMalformedChangesRejected(t *testing.T) {
	fx := newFixture(t)
	for name, c := range map[string]Change{
		"no parts":       {},
		"nil kind":       {Parts: []Part{{}}},
		"duplicate kind": {Parts: []Part{xrayPart("a"), xrayPart("b")}},
		"bad kind name":  {Parts: []Part{{Kind: &Kind{Name: "x-ray"}}}},
	} {
		if _, err := fx.m.Apply(context.Background(), c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if m := fx.f.Mutations(); len(m) != 0 {
		t.Errorf("rejected changes touched the system: %v", m)
	}
}

func TestBackupRetentionPerKind(t *testing.T) {
	fx := newFixture(t)
	fx.m.Keep = 2
	apply := func(parts ...Part) string {
		t.Helper()
		id, err := fx.m.Apply(context.Background(), Change{Parts: parts})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// Oldest first: A(xray+nft), B(xray), C(xray), D(nft).
	a := apply(xrayPart("1"), nftPart("1"))
	b := apply(xrayPart("2"))
	c := apply(xrayPart("3"))
	// After C, xray's newest two are C and B; A is still nft's newest.
	if got, _ := fx.m.Backups("nft"); !slices.Contains(got, a) {
		t.Errorf("A removed although it is the newest nft backup: %v", got)
	}
	d := apply(nftPart("2"))
	e := apply(nftPart("3"))
	// Now nft's newest two are E and D, xray's are C and B: A goes.
	if got, _ := fx.m.Backups("xray"); !slices.Equal(got, []string{b, c}) {
		t.Errorf("xray backups = %v, want %v", got, []string{b, c})
	}
	if got, _ := fx.m.Backups("nft"); !slices.Equal(got, []string{d, e}) {
		t.Errorf("nft backups = %v, want %v", got, []string{d, e})
	}
	for p := range fx.f.Files {
		if strings.Contains(p, a) {
			t.Errorf("pruned backup file still present: %s", p)
		}
	}
	// State migration backups are not change backups and are kept.
	fx.f.Files[backupDir+"/state-v0-20200101-000000.json"] = []byte("{}")
	apply(xrayPart("4"))
	if _, ok := fx.f.Files[backupDir+"/state-v0-20200101-000000.json"]; !ok {
		t.Error("migration backup pruned")
	}
}

func TestParseBackupID(t *testing.T) {
	for id, want := range map[string][]string{
		"xray-20261018-120000.000":         {"xray"},
		"xray+nft+ufw-20261018-120000.000": {"xray", "nft", "ufw"},
		"state-v0-20261018-120000.json":    nil,
		"xray":                             nil,
		"Xray-20261018-120000.000":         nil,
	} {
		kinds, _, ok := parseBackupID(id)
		if ok != (want != nil) || !slices.Equal(kinds, want) {
			t.Errorf("parseBackupID(%q) = %v, %v", id, kinds, ok)
		}
	}
}

func TestStagedPathKeepsExtension(t *testing.T) {
	for in, want := range map[string]string{
		"/etc/litenode/xray.json":                 "/etc/litenode/xray.litenode-new.json",
		"/etc/litenode/nftables.nft":              "/etc/litenode/nftables.litenode-new.nft",
		"/etc/ssh/sshd_config.d/00-litenode.conf": "/etc/ssh/sshd_config.d/00-litenode.litenode-new.conf",
		"/etc/litenode/noext":                     "/etc/litenode/noext.litenode-new",
	} {
		if got := StagedPath(in); got != want {
			t.Errorf("StagedPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	e := &Error{Stage: StageApply, Kind: "nft", Err: errors.New("x"), RestoreErr: fmt.Errorf("y")}
	if got := e.Error(); got != "nft apply failed: x; restore also failed: y" {
		t.Errorf("got %q", got)
	}
}
