package app

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/text"
)

// TestRenderMaintMenu compares against the DESIGN section 16 example.
func TestRenderMaintMenu(t *testing.T) {
	want, err := os.ReadFile("testdata/maint_menu.golden")
	if err != nil {
		t.Fatal(err)
	}
	m := (&maintUI{}).menu()
	m.Path = []string{text.MainMenuName}
	if got := m.Render(); got != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestMenusHaveNoBracketHints walks the main menu and every page under it:
// menu items carry no bracketed descriptions of what is inside (D-090);
// what to know before acting is in the confirmation instead.
func TestMenusHaveNoBracketHints(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	var in []string
	for i := 1; i <= 9; i++ {
		in = append(in, strconv.Itoa(i), "0")
	}
	// Pages one level further down: share > host address, SSH > keys,
	// Xray > logs.
	in = append(in, "3", "3", "0", "0", "6", "1", "0", "0", "5", "4", "0", "0", "0")
	r := runNodes(t, f, strings.Join(in, "\n")+"\n")
	item := regexp.MustCompile(`^ +\d+\. .*\(`)
	for _, line := range strings.Split(r.stdout, "\n") {
		if item.MatchString(line) && !strings.Contains(line, "当前") {
			t.Errorf("bracket hint: %q", line)
		}
	}
	if !strings.Contains(r.stdout, " › 配置主机地址 ══") || !strings.Contains(r.stdout, " › 密钥管理 ══") || !strings.Contains(r.stdout, " › 查看日志 ══") {
		t.Errorf("pages not visited:\n%s", r.stdout)
	}
}
