package system_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestDryRunPrintsMutationsWithoutExecuting(t *testing.T) {
	inner := systemtest.New()
	var out strings.Builder
	d := system.NewDryRun(inner, &out)
	ctx := context.Background()

	if _, err := d.Exec(ctx, "systemctl", "restart", "litenode-xray.service"); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteFile("/etc/litenode/xray.json", []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove("/etc/litenode/old.json"); err != nil {
		t.Fatal(err)
	}
	if err := d.MkdirAll("/etc/litenode/backups", 0o700); err != nil {
		t.Fatal(err)
	}

	if calls := inner.Calls(); len(calls) != 0 {
		t.Errorf("dry-run reached the inner runner: %v", calls)
	}
	want := []string{
		"systemctl restart litenode-xray.service",
		"/etc/litenode/xray.json",
		"0600",
		"/etc/litenode/old.json",
		"/etc/litenode/backups",
		"0700",
	}
	for _, w := range want {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output missing %q:\n%s", w, out.String())
		}
	}
	if n := strings.Count(out.String(), "[dry-run]"); n != 4 {
		t.Errorf("got %d dry-run lines, want 4:\n%s", n, out.String())
	}
}

func TestDryRunReadsThroughInner(t *testing.T) {
	inner := systemtest.New()
	inner.Files["/etc/os-release"] = []byte("ID=debian\n")
	inner.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")
	var out strings.Builder
	d := system.NewDryRun(inner, &out)

	data, err := d.ReadFile("/etc/os-release")
	if err != nil || string(data) != "ID=debian\n" {
		t.Errorf("ReadFile = (%q, %v)", data, err)
	}
	res, err := d.Query(context.Background(), "systemd-detect-virt")
	if err != nil || res.Stdout != "kvm\n" {
		t.Errorf("Query = (%+v, %v)", res, err)
	}
	if out.Len() != 0 {
		t.Errorf("read-only operations printed output: %q", out.String())
	}
}
