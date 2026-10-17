package app

import (
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// mainMenu builds the main menu (DESIGN section 8). Submenus are
// placeholders until their tasks are done.
func mainMenu(s *ui.Session, version string, info system.Info, st *state.State) ui.Menu {
	todo := func() error {
		s.Println(text.InDevelopment)
		return nil
	}
	return ui.Menu{
		Title: text.MainTitle(version),
		Status: ui.MainStatusLines(ui.MainStatus{
			XrayInstalled: st.Xray.Version != "",
			System:        info,
		}, s.Colors),
		Grouped:   true,
		ExitLabel: text.Exit,
		Items: []ui.Item{
			{Label: text.MenuNodes, Group: 1, Action: todo},
			{Label: text.MenuForwards, Group: 1, Action: todo},
			{Label: text.MenuXray, Group: 1, Action: todo},
			{Label: text.MenuSSH, Group: 2, Action: todo},
			{Label: text.MenuFirewall, Group: 2, Action: todo},
			{Label: text.MenuSysopt, Group: 2, Action: todo},
			{Label: text.MenuMaint, Hint: text.MenuMaintHint, Group: 3, Action: todo},
		},
	}
}
