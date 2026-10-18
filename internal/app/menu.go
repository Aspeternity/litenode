package app

import (
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
)

// mainMenu builds the main menu (DESIGN section 8).
func mainMenu(s *ui.Session, version string, info system.Info, st *state.State, xrayState, sshState string, ufwToo bool, nodes, forwards, xrayMenu, sshMenu, firewallMenu, sysoptMenu, maintMenu func() error) ui.Menu {
	var warnings []string
	if st.Xray.Stopped && enabledNodes(st) > 0 {
		warnings = append(warnings, text.XrayManualWarning)
	}
	if ufwToo {
		warnings = append(warnings, text.FirewallUFWBoth)
	}
	return ui.Menu{
		Title: text.MainTitle(version),
		Status: ui.MainStatusLines(ui.MainStatus{
			Xray:     xrayState,
			Nodes:    len(st.Nodes),
			Forwards: len(st.Forwards),
			SSH:      sshState,
			Firewall: firewallMainStatus(s, st.Firewall.Enabled),
			System:   info,
			Warnings: warnings,
		}, s.Colors),
		Grouped:   true,
		ExitLabel: text.Exit,
		Items: []ui.Item{
			{Label: text.MenuNodes, Group: 1, Page: true, Action: nodes},
			{Label: text.MenuForwards, Group: 1, Page: true, Action: forwards},
			{Label: text.MenuXray, Group: 1, Page: true, Action: xrayMenu},
			{Label: text.MenuSSH, Group: 2, Page: true, Action: sshMenu},
			{Label: text.MenuFirewall, Group: 2, Page: true, Action: firewallMenu},
			{Label: text.MenuSysopt, Group: 2, Page: true, Action: sysoptMenu},
			{Label: text.MenuMaint, Hint: text.MenuMaintHint, Group: 3, Page: true, Action: maintMenu},
		},
	}
}

func enabledNodes(st *state.State) int {
	n := 0
	for _, x := range st.Nodes {
		if x.Enabled {
			n++
		}
	}
	return n
}

// xrayStatus renders the Xray state for the main menu (D-045).
func xrayStatus(s *ui.Session, installed, active bool, st *state.State) string {
	switch {
	case !installed:
		return s.Colors.Off(text.XrayNotInstalled)
	case active:
		return s.Colors.Good(text.XrayRunning)
	case st.Xray.Stopped:
		return s.Colors.Warn(text.XrayStoppedByUser)
	}
	return s.Colors.Off(text.XrayStopped)
}
