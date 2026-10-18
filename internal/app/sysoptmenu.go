package app

import (
	"context"
	"errors"
	"strings"

	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
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
}

// mark shows a yes / no state.
func (u *sysoptUI) mark(on bool) string {
	if on {
		return u.s.Colors.Good(text.StateOn)
	}
	return u.s.Colors.Off(text.StateOff)
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
	m := ui.Menu{
		Heading: text.MenuSysopt,
		Status:  []string{strings.Join(fields, "   ")},
		Items: []ui.Item{
			{Label: text.SysoptBBR, Hidden: !u.caps.Has(system.CapBBR), Action: func() error { return u.bbr(bbr) }},
			{Label: text.SysoptSwap, Hint: text.SysoptSwapHint, Hidden: !u.caps.Has(system.CapSwap), Action: u.swap},
			{Label: text.SysoptPreference, Action: u.preference},
			{Label: text.SysoptDNS, Action: u.dns},
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
		if err := u.svc.SetBBR(u.ctx, false); err != nil {
			return err
		}
		u.s.Done(text.BBRDisabledDone)
		return nil
	}
	ok, err := u.s.Confirm(text.ConfirmBBREnable)
	if err != nil || !ok {
		return err
	}
	if err := u.svc.SetBBR(u.ctx, true); err != nil {
		return err
	}
	u.s.Done(text.BBREnabledDone)
	return nil
}

// describeSysopt maps system optimization errors to text; ok is false for
// other errors.
func describeSysopt(err error) (string, bool) {
	for target, msg := range map[error]string{
		sysopt.ErrDNSServers: text.ErrDNSServers,
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
		mb, ok, err := u.promptSwapSize(sysopt.SuggestMB(sw.MemMB))
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
	mb, ok, err := u.promptSwapSize(sw.Own.SizeMB)
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
// fits, 1G otherwise.
func (u *sysoptUI) promptSwapSize(def int) (mb int, ok bool, err error) {
	limit, err := u.svc.MaxCreateMB(u.ctx)
	if err != nil {
		return 0, false, err
	}
	if limit < sysopt.MinSwapMB {
		u.s.End(u.s.Colors.Warn(text.ErrNoDiskSpace))
		return 0, false, nil
	}
	u.s.Println(text.SwapMax(sysopt.FormatMB(limit)))
	if def <= 0 {
		def = 1024
	}
	def = max(min(def, limit), sysopt.MinSwapMB)
	for {
		in, err := u.s.PromptDefault(text.PromptSwapSize, sysopt.FormatMB(def))
		if err != nil {
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
		u.s.End(text.PreferenceUnchange)
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
		u.s.End(u.s.Colors.Warn(text.PreferenceForeign))
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
	if len(d.Servers) > 0 {
		cur = strings.Join(d.Servers, " ")
	}
	if d.Managed {
		source = text.DNSByLiteNode
	}
	u.s.Println(text.DNSCurrent(cur, source, d.Resolved))
	m := ui.Menu{}
	for _, p := range sysopt.DNSPresets {
		m.Items = append(m.Items, ui.Item{Label: text.DNSPresetLabel(p.Name, strings.Join(p.Servers, " "))})
	}
	m.Items = append(m.Items, ui.Item{Label: text.DNSCustom}, ui.Item{Label: text.DNSReset, Hidden: !d.Resolved || !d.Managed})
	c, err := u.s.Choose(m)
	switch {
	case err != nil || c == 0:
		return err
	case c == len(sysopt.DNSPresets)+2:
		if err := u.svc.ResetDNS(u.ctx); err != nil {
			return err
		}
		u.s.Done(text.DNSResetDone)
		return nil
	}
	var servers []string
	if c <= len(sysopt.DNSPresets) {
		servers = sysopt.DNSPresets[c-1].Servers
	} else {
		for {
			in, err := u.s.Prompt(text.PromptDNSServers + ": ")
			if err != nil {
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
			u.s.Println(u.s.Colors.Warn(text.DNSProbeFailed(srv, err)))
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
		u.s.Println(u.s.Colors.Warn(text.DNSVerifyFailed))
	}
	return nil
}
