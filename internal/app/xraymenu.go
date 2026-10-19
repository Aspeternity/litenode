package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
	"github.com/Aspeternity/litenode/internal/xray"
)

// xrayMenu is the Xray service menu (DESIGN section 12).
func (u *nodeUI) xrayMenu() ui.Menu {
	st := u.svc.XrayStatus(u.ctx)
	if !st.Installed {
		if len(u.state().Nodes) == 0 {
			return ui.Menu{Heading: text.MenuXray, Status: []string{text.XrayNotInstalledNote}}
		}
		// The program is gone but nodes remain (D-095).
		return ui.Menu{Heading: text.MenuXray, Status: []string{u.s.Colors.Warn(text.XrayMissingNote)},
			Items: []ui.Item{{Label: text.XrayInstall, Action: u.xrayReinstall}}}
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
			{Label: text.XrayLogs, Page: true, Action: u.xrayLogs},
			{Label: text.XrayOutboundIP, Action: u.xrayOutboundIP},
			{Label: text.XrayUpdate, Action: u.xrayUpdate},
			{Label: text.XrayUninstall, Action: u.xrayUninstall},
		},
	}
}

func (u *nodeUI) xrayStart() error {
	err := u.svc.StartXray(u.ctx)
	switch {
	case errors.Is(err, nodemgr.ErrNoEnabledNodes):
		u.s.End(text.XrayNoNodes)
		return nil
	case errors.Is(err, nodemgr.ErrAlreadyRunning):
		u.s.End(text.XrayAlreadyRunning)
		return nil
	}
	if err == nil {
		u.s.Done(text.XrayStarted)
	}
	return err
}

func (u *nodeUI) xrayStop() error {
	err := u.svc.StopXray(u.ctx)
	if errors.Is(err, nodemgr.ErrAlreadyStopped) {
		u.s.End(text.XrayAlreadyStopped)
		return nil
	}
	if err != nil {
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
		u.s.EndWarn(text.XrayStoppedByUserErr)
		return nil
	case err == nil:
		u.s.Done(text.XrayRestarted)
	}
	return err
}

// xrayLogs is the log submenu: recent lines, or following the journal
// until Ctrl+C (D-090).
func (u *nodeUI) xrayLogs() error {
	return u.s.Run(func() ui.Menu {
		return ui.Menu{
			Heading: text.XrayLogs,
			Items: []ui.Item{
				{Label: text.XrayRecentLogs, Action: u.xrayRecentLogs},
				{Label: text.XrayLiveLogs, Action: u.xrayLiveLogs},
			},
		}
	})
}

func (u *nodeUI) xrayRecentLogs() error {
	n, err := u.s.PromptNumber(text.PromptLogLines, 50, 0, nodemgr.MaxLogLines)
	if err != nil || n == 0 {
		return err
	}
	logs, err := u.svc.XrayLogs(u.ctx, n)
	if err != nil {
		return err
	}
	u.s.End(text.XrayLogsTitle(n))
	u.s.Hold()
	u.s.Println(strings.TrimRight(logs, "\n"))
	return nil
}

func (u *nodeUI) xrayLiveLogs() error {
	u.s.End(text.XrayLiveLogsTitle)
	u.s.Hold()
	return u.s.Follow(u.ctx, func(ctx context.Context) error {
		return u.svc.FollowXrayLogs(ctx, u.s.Out)
	})
}

func (u *nodeUI) xrayReinstall() error {
	u.s.Println(text.XrayInstallingNow)
	if err := u.svc.ReinstallXray(u.ctx); err != nil {
		return err
	}
	u.s.Done(text.XrayReinstalled)
	return nil
}

func (u *nodeUI) xrayUpdate() error {
	def, _ := u.svc.Xray.DefaultVersion(u.ctx)
	in, err := u.s.PromptValue(text.PromptXrayVersion, def)
	if err != nil || in == "" {
		return err
	}
	if v, err := xray.NormalizeVersion(in); err == nil {
		if !xray.SupportsMLKEMTargets(v) {
			u.s.Println(u.s.Colors.Warn(text.XrayOldVersion(v, xray.MinMLKEMVersion)))
		}
		nodes := u.state().Nodes
		if !xray.SupportsVLESSEnc(v) && slices.ContainsFunc(nodes, func(n node.Node) bool { return n.VLESSEncryption != nil }) {
			u.s.Println(u.s.Colors.Warn(text.XrayOldVersionFor(text.FeatureVLESSEnc, v, xray.MinVLESSEncVersion)))
		}
		if !xray.SupportsHysteria(v) && slices.ContainsFunc(nodes, func(n node.Node) bool { return n.Protocol == "hysteria" }) {
			u.s.Println(u.s.Colors.Warn(text.XrayOldVersionFor(text.FeatureHysteria, v, xray.MinHysteriaVersion)))
		}
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
		u.s.End(text.Unchanged)
		return nil
	}
	ipv6, only := xray.OnlyIPv6(mode)
	if only {
		u.s.Println(u.s.Colors.Warn(text.OutboundIPOnlyWarning))
	}
	err = u.svc.SetOutboundIP(u.ctx, mode)
	if errors.Is(err, nodemgr.ErrFamilyUnavailable) {
		u.s.EndWarn(text.OutboundIPUnavailable(ipv6))
		return nil
	}
	if err != nil {
		return err
	}
	u.s.Done(text.OutboundIPDone(text.OutboundIPLabels[c-1]))
	return nil
}
