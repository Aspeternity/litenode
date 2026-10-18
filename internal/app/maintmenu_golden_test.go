package app

import (
	"os"
	"testing"
)

// TestRenderMaintMenu compares against the DESIGN section 16 example.
func TestRenderMaintMenu(t *testing.T) {
	want, err := os.ReadFile("testdata/maint_menu.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := (&maintUI{}).menu().Render(); got != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
