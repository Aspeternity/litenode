package nft

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

var on = Config{Firewall: &Firewall{Rules: []Rule{{Ports: "22", Proto: "tcp", Comment: "litenode:ssh"}}}}

func apply(t *testing.T, f *systemtest.Fake, c Config) error {
	t.Helper()
	file, err := ConfigFile(c)
	if err != nil {
		t.Fatal(err)
	}
	_, err = change.NewManager(f, state.NewStore(f)).Apply(context.Background(), change.Change{Parts: []change.Part{{Kind: Kind(), Files: []change.File{file}}}})
	return err
}

func TestKindLoadsAndEnablesUnit(t *testing.T) {
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "systemctl", "is-enabled", "--quiet", UnitName)
	if err := apply(t, f, on); err != nil {
		t.Fatal(err)
	}
	var checked bool
	for _, c := range f.Calls() {
		if c.Command == NftPath+" -c -f /etc/litenode/nftables.litenode-new.nft" {
			checked = true
		}
	}
	execs := f.Execs()
	if !checked || !slices.Contains(execs, NftPath+" -f "+ConfigPath) || !slices.Contains(execs, "systemctl enable "+UnitName) || string(f.Files[UnitPath]) != unit {
		t.Errorf("checked=%v execs=%v", checked, execs)
	}
	// Turning everything off removes the table and disables the unit.
	f.Respond(systemtest.Response{}, "systemctl", "is-enabled", "--quiet", UnitName)
	if err := apply(t, f, Config{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Execs(), "systemctl disable "+UnitName) || hasTable(f.Files[ConfigPath]) {
		t.Errorf("execs = %v", f.Execs())
	}
}

func TestKindRejectedRulesNotLoaded(t *testing.T) {
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1, Stderr: "syntax error"}}, NftPath, "-c", "-f", "/etc/litenode/nftables.litenode-new.nft")
	err := apply(t, f, on)
	var ce *change.Error
	if !errors.As(err, &ce) || ce.Stage != change.StageValidate {
		t.Fatalf("err = %v", err)
	}
	// The restore re-applies "no table", but the rejected file is never
	// loaded.
	if slices.Contains(f.Execs(), NftPath+" -f "+ConfigPath) {
		t.Errorf("loaded anyway: %v", f.Execs())
	}
}

func TestKindRestoredToNoFileRemovesTable(t *testing.T) {
	// After a restore to before the first rules there is no file: the
	// table must go.
	f := systemtest.New()
	if err := Kind().Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	var input string
	for _, c := range f.Calls() {
		if c.Command == NftPath+" -f -" {
			input = string(c.Data)
		}
	}
	if input == "" || hasTable([]byte(input)) {
		t.Errorf("removal script = %q", input)
	}
}
