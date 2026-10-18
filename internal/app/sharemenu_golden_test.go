package app

import (
	"os"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

func clientMenusUI(t *testing.T) *nodeUI {
	t.Helper()
	f := systemtest.New()
	st := state.New()
	st.Server.ShareAddresses = []string{"203.0.113.10", "2001:db8::10", "la.example.com"}
	st.Nodes = []node.Node{
		{ID: 1, Remark: "LA-Reality", Port: 443, Enabled: true, Clients: []node.Client{
			{ID: 1, Remark: "me", Enabled: true}, {ID: 2, Remark: "gf", Enabled: true}}},
		{ID: 2, Remark: "LA-8443", Port: 8443, Enabled: true, Clients: []node.Client{
			{ID: 3, Remark: "old", Enabled: false}}},
	}
	store := state.NewStore(f)
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	return &nodeUI{s: &ui.Session{}, svc: &nodemgr.Service{Store: store}}
}

func checkGolden(t *testing.T, name string, m ui.Menu) {
	t.Helper()
	want, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	m.Path = []string{text.MainMenuName}
	if got := m.Render(); got != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestRenderClientMenus compares against the DESIGN section 9.4 and 9.5
// examples.
func TestRenderClientMenus(t *testing.T) {
	u := clientMenusUI(t)
	checkGolden(t, "clients_menu.golden", u.clientsMenu())
	checkGolden(t, "share_menu.golden", u.shareMenu())
}
