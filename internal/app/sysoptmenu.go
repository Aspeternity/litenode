package app

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
	"github.com/Aspeternity/litenode/internal/warp"
)

// sysoptUI holds the system optimization menu (DESIGN section 15). It
// only displays and reads input; sysopt.Service does the work.
type sysoptUI struct {
	ctx  context.Context
	s    *ui.Session
	svc  *sysopt.Service
	info system.Info
	caps system.Capabilities
	// probeDNS tests DNS servers before they are used.
	probeDNS sysopt.DNSProber
	// syncWait is how long to wait for the clock to synchronize.
	syncWait time.Duration
	warp     *warp.Service
	egress   Egress
}

// mark shows a yes / no state; the thin ✗ is red, gray is hard to see
// (D-127).
func (u *sysoptUI) mark(on bool) string {
	if on {
		return u.s.Colors.Good(text.StateOn)
	}
	return u.s.Colors.Bad(text.StateOff)
}

func (u *sysoptUI) menu() ui.Menu {
	var fields []string
	bbr, err := u.svc.BBRStatus()
	if err != nil {
		return ui.Menu{Heading: text.MenuSysopt, Status: []string{u.s.Colors.Bad(text.ActionFailed(err))}}
	}
	if u.caps.Has(system.CapBBR) {
		fields = append(fields, text.StatusBBR(u.mark(bbr.Active)))
	}
	if u.caps.Has(system.CapSwap) {
		v := text.SwapNone
		if sw, err := u.svc.SwapStatus(); err == nil && sw.TotalMB() > 0 {
			v = sysopt.FormatMB(sw.TotalMB())
		}
		fields = append(fields, text.StatusSwap(v))
	}
	if p, err := u.svc.PreferenceStatus(); err == nil {
		fields = append(fields, text.StatusPreference(p.IPv4))
	}
	clock, clockErr := u.svc.ClockStatus(u.ctx)
	if clockErr == nil {
		synced := ""
		if !u.info.Virt.Container {
			synced = u.mark(clock.Synced)
		}
		fields = append(fields, text.StatusClock(clock.Timezone, synced))
	}
	status := []string{strings.Join(fields, "   ")}
	if !u.info.Virt.Container {
		// On its own line: the first one is full.
		v := text.WARPNone
		if st, _ := u.warp.Status(u.ctx); st.Enabled {
			v = u.s.Colors.Good(text.StateOn)
		}
		status = append(status, text.StatusWARP(v))
	}
	m := ui.Menu{
		Heading: text.MenuSysopt,
		Status:  status,
		Items: []ui.Item{
			{Label: text.SysoptBBR, Hidden: !u.caps.Has(system.CapBBR), Action: func() error { return u.bbr(bbr) }},
			{Label: text.SysoptSwap, Hidden: !u.caps.Has(system.CapSwap), Action: u.swap},
			{Label: text.SysoptPreference, Action: u.preference},
			{Label: text.SysoptDNS, Action: u.dns},
			{Label: text.SysoptClock, Hidden: clockErr != nil, Action: u.clock},
			{Label: text.SysoptWARP, Hidden: u.info.Virt.Container, Page: true, Action: func() error { return u.s.Run(u.warpMenu) }},
		},
	}
	for _, it := range m.Items {
		if it.Hidden {
			m.Notice = u.s.Colors.Warn(text.SysoptUnavailableIn(ui.VirtName(u.info.Virt)))
			break
		}
	}
	return m
}

func (u *sysoptUI) bbr(b sysopt.BBR) error {
	switch {
	case b.Active && !b.Managed:
		u.s.End(text.SysoptBBRByOther)
		return nil
	case b.Active:
		ok, err := u.s.Confirm(text.ConfirmBBRDisable)
		if err != nil || !ok {
			return err
		}
		keptBy, err := u.svc.SetBBR(u.ctx, false)
		if err != nil {
			return err
		}
		if keptBy != "" {
			u.s.Done(text.BBRKeptBy(keptBy))
			u.s.Hold()
			return nil
		}
		u.s.Done(text.BBRDisabledDone)
		return nil
	}
	ok, err := u.s.Confirm(text.ConfirmBBREnable)
	if err != nil || !ok {
		return err
	}
	if _, err := u.svc.SetBBR(u.ctx, true); err != nil {
		return err
	}
	u.s.Done(text.BBREnabledDone)
	return nil
}

// describeSysopt maps system optimization errors to text; ok is false for
// other errors.
func describeSysopt(err error) (string, bool) {
	for target, msg := range map[error]string{
		sysopt.ErrDNSServers: text.ErrDNSServers, sysopt.ErrTimezone: text.ErrTimezone,
		sysopt.ErrSwapExists: text.ErrSwapExists, sysopt.ErrNoSwap: text.ErrNoSwap,
		sysopt.ErrSwapInUse: text.ErrSwapInUse, sysopt.ErrNoDiskSpace: text.ErrNoDiskSpace,
		sysopt.ErrSwapSize: text.ErrSwapSize(sysopt.FormatMB(sysopt.MinSwapMB), sysopt.FormatMB(sysopt.MaxSwapMB)),
	} {
		if errors.Is(err, target) {
			return msg, true
		}
	}
	return "", false
}

func (u *sysoptUI) swap() error {
	sw, err := u.svc.SwapStatus()
	if err != nil {
		return err
	}
	for _, e := range sw.Entries {
		u.s.Println(text.SwapLine(e.Path, sysopt.FormatMB(e.SizeMB), e.UsedMB, e.Path == sysopt.SwapPath))
	}
	if sw.Own == nil {
		suggest := ""
		if mb := sysopt.SuggestMB(sw.MemMB); mb > 0 {
			suggest = sysopt.FormatMB(mb)
		}
		u.s.Println(text.SwapMemory(sysopt.FormatMB(sw.MemMB), suggest))
		mb, ok, err := u.promptSwapSize(sysopt.SuggestMB(sw.MemMB), false)
		if err != nil || !ok {
			return err
		}
		u.s.Println(text.SwapCreating)
		if err := u.svc.CreateSwap(u.ctx, mb); err != nil {
			return err
		}
		u.s.Done(text.SwapCreated(sysopt.FormatMB(mb)))
		return nil
	}
	c, err := u.s.Choose(ui.Menu{Items: []ui.Item{{Label: text.SwapResize}, {Label: text.SwapDelete}}})
	switch {
	case err != nil || c == 0:
		return err
	case c == 2:
		ok, err := u.s.Confirm(text.ConfirmSwapDelete)
		if err != nil || !ok {
			return err
		}
		if err := u.svc.DeleteSwap(u.ctx); err != nil {
			return err
		}
		u.s.Done(text.SwapDeleted)
		return nil
	}
	mb, ok, err := u.promptSwapSize(sw.Own.SizeMB, true)
	if err != nil || !ok {
		return err
	}
	u.s.Println(text.SwapCreating)
	if err := u.svc.ResizeSwap(u.ctx, mb); err != nil {
		return err
	}
	u.s.Done(text.SwapResized(sysopt.FormatMB(mb)))
	return nil
}

// promptSwapSize asks for a size within what the disk can hold; ok is
// false when the disk has no room. def (MiB) is the default when it
// fits, 1G otherwise. resize adds why the limit can be below the
// current size plus the free space.
func (u *sysoptUI) promptSwapSize(def int, resize bool) (mb int, ok bool, err error) {
	limit, err := u.svc.MaxCreateMB(u.ctx)
	if err != nil {
		return 0, false, err
	}
	if limit < sysopt.MinSwapMB {
		u.s.EndWarn(text.ErrNoDiskSpace)
		return 0, false, nil
	}
	u.s.Println(text.SwapMax(sysopt.FormatMB(limit)))
	if resize {
		u.s.Println(text.SwapResizeNote)
	}
	if def <= 0 {
		def = 1024
	}
	def = max(min(def, limit), sysopt.MinSwapMB)
	for {
		in, err := u.s.PromptValue(text.PromptSwapSize, sysopt.FormatMB(def))
		if err != nil || in == "" {
			return 0, false, err
		}
		mb, err := sysopt.ParseSize(in)
		if err == nil && mb >= sysopt.MinSwapMB && mb <= limit {
			return mb, true, nil
		}
		u.s.Println(u.s.Colors.Warn(text.ErrSwapSize(sysopt.FormatMB(sysopt.MinSwapMB), sysopt.FormatMB(limit))))
	}
}

func (u *sysoptUI) preference() error {
	p, err := u.svc.PreferenceStatus()
	if err != nil {
		return err
	}
	u.s.Println(text.PreferenceHint)
	label := func(l string, cur bool) string {
		if cur {
			return l + " " + text.CurrentMark
		}
		return l
	}
	c, err := u.s.Choose(ui.Menu{Items: []ui.Item{{Label: label(text.PreferIPv4, p.IPv4)}, {Label: label(text.PreferIPv6, !p.IPv4)}}})
	if err != nil || c == 0 {
		return err
	}
	ipv4 := c == 1
	if ipv4 == p.IPv4 {
		u.s.End(text.Unchanged)
		return nil
	}
	if err := u.svc.SetPreference(u.ctx, ipv4); err != nil {
		return err
	}
	if ipv4 {
		u.s.Done(text.PreferIPv4Done)
		return nil
	}
	if after, err := u.svc.PreferenceStatus(); err == nil && after.IPv4 {
		u.s.EndWarn(text.PreferenceForeign)
		return nil
	}
	u.s.Done(text.PreferIPv6Done)
	return nil
}

func (u *sysoptUI) dns() error {
	d, err := u.svc.DNSStatus(u.ctx)
	if err != nil {
		return err
	}
	cur, source := text.DNSNone, text.DNSBySystem
	down := u.unreachableDNS(d.Servers)
	if len(d.Servers) > 0 {
		shown := make([]string, len(d.Servers))
		for i, srv := range d.Servers {
			shown[i] = srv
			if down[i] {
				shown[i] = u.s.Colors.Bad(text.DNSUnreachable(srv))
			}
		}
		cur = strings.Join(shown, " ")
	}
	if d.Managed {
		source = text.DNSByLiteNode
	}
	u.s.Println(text.DNSCurrent(cur, source, d.Resolved))
	if slices.Contains(down, true) {
		u.s.Println(u.s.Colors.Warn(text.DNSUnreachableHint))
	}
	eg := u.egress.Status(u.ctx)
	presets := sysopt.PresetsFor(eg.IPv4.OK, eg.IPv6.OK)
	m := ui.Menu{}
	for _, p := range presets {
		m.Items = append(m.Items, ui.Item{Label: text.DNSPresetLabel(p.Name, strings.Join(p.Servers, " "))})
	}
	m.Items = append(m.Items, ui.Item{Label: text.DNSCustom}, ui.Item{Label: text.DNSReset, Hidden: !d.Resolved || !d.Managed})
	c, err := u.s.Choose(m)
	switch {
	case err != nil || c == 0:
		return err
	case c == len(presets)+2:
		if err := u.svc.ResetDNS(u.ctx); err != nil {
			return err
		}
		u.s.Done(text.DNSResetDone)
		return nil
	}
	var servers []string
	if c <= len(presets) {
		servers = presets[c-1].Servers
	} else {
		for {
			in, err := u.s.PromptValue(text.PromptDNSServers, "")
			if err != nil || in == "" {
				return err
			}
			if servers, err = sysopt.ParseServers(in); err == nil {
				break
			}
			u.s.Println(u.s.Colors.Warn(text.ErrDNSServers))
		}
	}
	failed := false
	for _, srv := range servers {
		if err := u.probeDNS(u.ctx, srv); err != nil {
			u.s.Println(u.s.Colors.Warn(text.DNSProbeFailed(srv, text.NetReason(err))))
			failed = true
		}
	}
	if failed {
		if ok, err := u.s.Confirm(text.ConfirmDNSFailed); err != nil || !ok {
			return err
		}
	}
	if !d.Resolved {
		u.s.Println(u.s.Colors.Warn(text.DNSResolvConfNote))
	}
	if err := u.svc.SetDNS(u.ctx, servers); err != nil {
		return err
	}
	u.s.Done(text.DNSSet(strings.Join(servers, " ")))
	if !u.svc.LookupWorks(u.ctx) {
		u.s.Hold()
		u.s.Println(u.s.Colors.Warn(text.DNSVerifyFailed))
	}
	return nil
}

// offerDNSFix asks at startup to replace an unreachable IPv4 DNS server
// that every lookup waits for, with the IPv6 servers of the first preset
// that answers (D-138). It reports whether it printed anything, so that
// the caller waits for Enter before the main menu clears the screen.
func (u *sysoptUI) offerDNSFix() (bool, error) {
	st, err := u.svc.Store.Load()
	if err != nil || st.Server.SlowDNSDeclined {
		return false, nil
	}
	eg := u.egress.Status(u.ctx)
	if !eg.IPv6.OK {
		return false, nil
	}
	d, err := u.svc.DNSStatus(u.ctx)
	if err != nil {
		return false, nil
	}
	slow, ok := sysopt.SlowFirstServer(d, eg.IPv4.OK)
	if !ok || !u.unreachableDNS([]string{slow})[0] {
		return false, nil
	}
	var preset sysopt.DNSPreset
	for _, p := range sysopt.PresetsFor(false, true) {
		if u.probeDNS(u.ctx, p.Servers[0]) == nil {
			preset = p
			break
		}
	}
	u.s.Println(u.s.Colors.Warn(text.DNSSlowFirst(slow)))
	if preset.Name == "" {
		u.s.Println(u.s.Colors.Warn(text.DNSUnreachableHint))
		return true, nil
	}
	u.s.Println(u.s.Colors.Warn(text.DNSResolvConfNote))
	servers := strings.Join(preset.Servers, " ")
	yes, err := u.s.ConfirmYes(text.ConfirmDNSFix(preset.Name, servers))
	if err != nil {
		return true, err
	}
	if !yes {
		st.Server.SlowDNSDeclined = true
		if err := u.svc.Store.Save(st); err != nil {
			u.s.Println(u.s.Colors.Bad(text.ActionFailed(err)))
			return true, nil
		}
		u.s.Println(text.DNSFixDeclined)
		return true, nil
	}
	if err := u.svc.SetDNS(u.ctx, preset.Servers); err != nil {
		u.s.Println(u.s.Colors.Bad(text.ActionFailed(err)))
		return true, nil
	}
	u.s.Done(text.DNSSet(servers))
	return true, nil
}

// unreachableDNS probes the servers in parallel, 2 seconds each, and
// reports which did not answer (D-134).
func (u *sysoptUI) unreachableDNS(servers []string) []bool {
	down := make([]bool, len(servers))
	ctx, cancel := context.WithTimeout(u.ctx, 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i, srv := range servers {
		wg.Go(func() { down[i] = u.probeDNS(ctx, srv) != nil })
	}
	wg.Wait()
	return down
}

func (u *sysoptUI) clock() error {
	c, err := u.svc.ClockStatus(u.ctx)
	if err != nil {
		return err
	}
	u.s.Println(text.ClockCurrent(c.Timezone, c.NTP, c.Synced))
	// Containers take the time of the host.
	choice, err := u.s.Choose(ui.Menu{Items: []ui.Item{
		{Label: text.ClockSetTimezone},
		{Label: text.ClockEnableNTP, Hidden: u.info.Virt.Container},
	}})
	switch {
	case err != nil || choice == 0:
		return err
	case choice == 1:
		return u.timezone(c.Timezone)
	}
	if c.NTP && c.Synced {
		u.s.End(text.NTPAlreadySynced)
		return nil
	}
	if !c.CanNTP {
		u.s.Println(text.NTPInstallingNote)
	}
	if err := u.svc.EnableNTP(u.ctx); err != nil {
		return err
	}
	u.s.Println(text.NTPWaiting)
	if u.svc.WaitSynced(u.ctx, u.syncWait, 2*time.Second) {
		u.s.Done(text.NTPSyncedDone)
	} else {
		u.s.Done(text.NTPPendingDone)
	}
	return nil
}

func (u *sysoptUI) timezone(cur string) error {
	m := ui.Menu{}
	for _, tz := range sysopt.CommonTimezones {
		l := tz
		if tz == cur {
			l += " " + text.CurrentMark
		}
		m.Items = append(m.Items, ui.Item{Label: l})
	}
	m.Items = append(m.Items, ui.Item{Label: text.TimezoneCustom})
	c, err := u.s.Choose(m)
	if err != nil || c == 0 {
		return err
	}
	var tz string
	if c <= len(sysopt.CommonTimezones) {
		tz = sysopt.CommonTimezones[c-1]
	} else if tz, err = u.s.PromptValue(text.PromptTimezone, ""); err != nil || tz == "" {
		return err
	}
	if err := u.svc.SetTimezone(u.ctx, strings.TrimSpace(tz)); err != nil {
		return err
	}
	u.s.Done(text.TimezoneSet(strings.TrimSpace(tz)))
	return nil
}

// warpMenu is the WARP page (DESIGN section 19.4).
func (u *sysoptUI) warpMenu() ui.Menu {
	st, err := u.warp.Status(u.ctx)
	if err != nil {
		return ui.Menu{Heading: text.SysoptWARP, Status: []string{u.s.Colors.Bad(text.ActionFailed(err))}}
	}
	if !st.Enabled {
		return ui.Menu{Heading: text.SysoptWARP, Status: []string{text.WARPOff},
			Items: []ui.Item{{Label: text.WARPEnable, Action: u.warpEnable}}}
	}
	exit := ""
	if f := u.egress.Status(u.ctx).Of(st.Conf.IPv6); f.WARP {
		exit = f.IP.String()
	}
	hs := text.WARPNoHandshake
	switch {
	case !st.Active:
		hs = u.s.Colors.Bad(text.WARPInactive)
	case !st.Handshake.IsZero():
		hs = text.WARPHandshakeAgo(time.Since(st.Handshake))
	}
	return ui.Menu{Heading: text.SysoptWARP,
		Status: []string{text.WARPOn(st.Conf.IPv6, exit, st.Conf.Endpoint.String(), hs)},
		Items:  []ui.Item{{Label: text.WARPReselect, Action: u.warpReselect}, {Label: text.WARPDisable, Action: u.warpDisable}}}
}

func (u *sysoptUI) warpEnable() error {
	eg := u.egress.Status(u.ctx)
	switch {
	case !eg.IPv4.OK && !eg.IPv6.OK:
		u.s.EndWarn(text.WARPNoExit)
		return nil
	case eg.IPv4.OK && eg.IPv6.OK:
		byWARP := ""
		if eg.IPv4.WARP {
			byWARP = "IPv4"
		} else if eg.IPv6.WARP {
			byWARP = "IPv6"
		}
		u.s.End(text.WARPNotNeeded(byWARP))
		return nil
	}
	ipv6 := !eg.IPv6.OK
	if ok, err := u.s.Confirm(text.ConfirmWARPEnable(ipv6)); err != nil || !ok {
		return err
	}
	u.s.Println(text.WARPPreparing)
	if err := u.warp.Enable(u.ctx, ipv6, u.warpTrying); err != nil {
		return u.warpFailed(err)
	}
	u.egress.Reset()
	exit := ""
	if f := u.egress.Status(u.ctx).Of(ipv6); f.OK {
		exit = f.IP.String()
	}
	u.s.Done(text.WARPEnabled(ipv6, exit))
	return nil
}

func (u *sysoptUI) warpTrying(ep netip.AddrPort) { u.s.Println(text.WARPTrying(ep.String())) }

func (u *sysoptUI) warpFailed(err error) error {
	switch {
	case errors.Is(err, warp.ErrNoKernel):
		u.s.EndWarn(text.WARPNoKernel)
	case errors.Is(err, warp.ErrNoEndpoint):
		u.s.EndWarn(text.WARPNoEndpoint(err))
	default:
		return err
	}
	return nil
}

func (u *sysoptUI) warpReselect() error {
	if err := u.warp.Reselect(u.ctx, u.warpTrying); err != nil {
		return u.warpFailed(err)
	}
	u.egress.Reset()
	c, _ := u.warp.Conf()
	u.s.Done(text.WARPReselected(c.Endpoint.String()))
	return nil
}

func (u *sysoptUI) warpDisable() error {
	if ok, err := u.s.Confirm(text.ConfirmWARPDisable); err != nil || !ok {
		return err
	}
	if err := u.warp.Disable(u.ctx); err != nil {
		return err
	}
	u.egress.Reset()
	u.s.Done(text.WARPDisabled)
	return nil
}
