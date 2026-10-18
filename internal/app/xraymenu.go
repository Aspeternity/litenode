package app

import (
	"errors"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
	"github.com/Aspeternity/litenode/internal/xray"
)

// xrayMenu is the Xray service menu (DESIGN section 12).
func (u *nodeUI) xrayMenu() ui.Menu {
	st := u.svc.XrayStatus(u.ctx)
	if !st.Installed {
		return ui.Menu{Heading: text.MenuXray, Status: []string{text.XrayNotInstalledNote}}
	}
	state := u.s.Colors.Off(text.XrayStopped)
	switch {
	case st.Active:
		state = u.s.Colors.Good(text.XrayRunning)
	case st.StoppedByUser:
		state = u.s.Colors.Warn(text.XrayStoppedByUser)
	}
	return ui.Menu{
		Heading: text.MenuXray,
		Status: []string{
			text.XrayStatusLine(st.Version, state, int64(st.Uptime.Seconds()), st.Memory),
			text.StatusOutboundIP(outboundLabel(u.state().Xray.OutboundIP)),
		},
		Items: []ui.Item{
			{Label: text.XrayStart, Action: u.xrayStart},
			{Label: text.XrayStop, Action: u.xrayStop},
			{Label: text.XrayRestart, Action: u.xrayRestart},
			{Label: text.XrayLogs, Action: u.xrayLogs},
			{Label: text.XrayOutboundIP, Hint: text.XrayOutboundIPHint, Action: u.xrayOutboundIP},
			{Label: text.XrayUpdate, Hint: text.XrayUpdateHint, Action: u.xrayUpdate},
			{Label: text.XrayUninstall, Hint: text.XrayUninstallHint, Action: u.xrayUninstall},
		},
	}
}

func (u *nodeUI) xrayStart() error {
	err := u.svc.StartXray(u.ctx)
	if errors.Is(err, nodemgr.ErrNoEnabledNodes) {
		u.s.End(text.XrayNoNodes)
		return nil
	}
	if err == nil {
		u.s.Done(text.XrayStarted)
	}
	return err
}

func (u *nodeUI) xrayStop() error {
	if err := u.svc.StopXray(u.ctx); err != nil {
		return err
	}
	u.s.Done(text.XrayStoppedMsg)
	return nil
}

func (u *nodeUI) xrayRestart() error {
	err := u.svc.RestartXray(u.ctx)
	switch {
	case errors.Is(err, nodemgr.ErrNoEnabledNodes):
		u.s.End(text.XrayNoNodes)
		return nil
	case err != nil && u.state().Xray.Stopped:
		u.s.End(u.s.Colors.Warn(text.XrayStoppedByUserErr))
		return nil
	case err == nil:
		u.s.Done(text.XrayRestarted)
	}
	return err
}

func (u *nodeUI) xrayLogs() error {
	logs, err := u.svc.XrayLogs(u.ctx, 50)
	if err != nil {
		return err
	}
	u.s.End(text.XrayLogsTitle)
	u.s.Println(strings.TrimRight(logs, "\n"))
	return nil
}

func (u *nodeUI) xrayUpdate() error {
	def, _ := u.svc.Xray.DefaultVersion(u.ctx)
	in, err := u.s.PromptDefault(text.PromptXrayVersion, def)
	if err != nil {
		return err
	}
	if v, err := xray.NormalizeVersion(in); err == nil && !xray.SupportsMLKEMTargets(v) {
		u.s.Println(u.s.Colors.Warn(text.XrayOldVersion(v, xray.MinMLKEMVersion)))
	}
	u.s.Println(text.XrayUpdating)
	v, err := u.svc.UpdateXray(u.ctx, in)
	if err != nil {
		return err
	}
	u.s.Done(text.XrayUpdated(v))
	return nil
}

func (u *nodeUI) xrayUninstall() error {
	st := u.state()
	clients := 0
	for _, n := range st.Nodes {
		clients += len(n.Clients)
	}
	ok, err := u.s.Confirm(text.ConfirmUninstallXray(len(st.Nodes), clients))
	if err != nil || !ok {
		return err
	}
	if err := u.svc.UninstallXray(u.ctx); err != nil {
		return err
	}
	u.s.Done(text.XrayUninstalled)
	return ui.ErrBack
}

// outboundLabel names an outbound IP mode.
func outboundLabel(mode string) string {
	if i := slices.Index(xray.OutboundModes, mode); i >= 0 {
		return text.OutboundIPLabels[i]
	}
	return mode
}

func (u *nodeUI) xrayOutboundIP() error {
	cur := u.state().Xray.OutboundIP
	u.s.Println(text.OutboundIPNote)
	items := make([]ui.Item, len(xray.OutboundModes))
	for i, m := range xray.OutboundModes {
		items[i].Label = text.OutboundIPLabels[i]
		if m == cur {
			items[i].Label += " " + text.CurrentMark
		}
	}
	c, err := u.s.Choose(ui.Menu{Items: items})
	if err != nil || c == 0 {
		return err
	}
	mode := xray.OutboundModes[c-1]
	if mode == cur {
		u.s.End(text.OutboundIPUnchanged)
		return nil
	}
	ipv6, only := xray.OnlyIPv6(mode)
	if only {
		u.s.Println(u.s.Colors.Warn(text.OutboundIPOnlyWarning))
	}
	err = u.svc.SetOutboundIP(u.ctx, mode)
	if errors.Is(err, nodemgr.ErrFamilyUnavailable) {
		u.s.End(u.s.Colors.Warn(text.OutboundIPUnavailable(ipv6)))
		return nil
	}
	if err != nil {
		return err
	}
	u.s.Done(text.OutboundIPDone(text.OutboundIPLabels[c-1]))
	return nil
}
