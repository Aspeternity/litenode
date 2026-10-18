package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/reality"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/ui"
	"github.com/Aspeternity/litenode/internal/xray"
)

// TargetChecker checks REALITY targets (reality.Checker).
type TargetChecker interface {
	Check(ctx context.Context, target string) reality.Result
	CheckAll(ctx context.Context, targets []string, limit int) []reality.Result
}

// mlkemAware is implemented by checkers that need to know whether the Xray
// in use handles ML-KEM targets.
type mlkemAware interface {
	SetMLKEMSupported(bool)
}

// TargetScanner scans for nearby REALITY targets (reality.Scanner).
type TargetScanner interface {
	Scan(ctx context.Context, center netip.Addr, progress func(done, total int)) ([]reality.Result, error)
}

// AddressDetector finds the public address (share.Detector).
type AddressDetector interface {
	Detect(ctx context.Context) share.Detected
	OnInterface(ip string) bool
}

// nodeUI holds the node menus (DESIGN section 9). It only displays and
// reads input; nodemgr does the work.
type nodeUI struct {
	ctx      context.Context
	s        *ui.Session
	svc      *nodemgr.Service
	checker  TargetChecker
	detector AddressDetector
	scanner  TargetScanner
	// dns resolves domains added as share addresses (nil: the system
	// resolver).
	dns share.Resolver
}

func (u *nodeUI) state() *state.State {
	st, err := u.svc.Store.Load()
	if err != nil {
		return state.New()
	}
	return st
}

// protocolLabel is e.g. "VLESS-TCP-Reality".
func (u *nodeUI) protocolLabel(n node.Node) string {
	label := func(c node.Component, ok bool, name string) string {
		if ok {
			return c.Label()
		}
		return name
	}
	p, pok := u.svc.Registry.Protocol(n.Protocol)
	t, tok := u.svc.Registry.Transport(n.Transport)
	s, sok := u.svc.Registry.Security(n.Security)
	return label(p, pok, n.Protocol) + "-" + label(t, tok, n.Transport) + "-" + label(s, sok, n.Security)
}

// menu is the node list menu (DESIGN section 9.1).
func (u *nodeUI) menu() ui.Menu {
	st := u.state()
	status := []string{text.ShareAddressLine(st.Server.DefaultShareAddress(), len(st.Server.ShareAddresses))}
	if len(st.Nodes) == 0 {
		status = append(status, text.NodeNone)
	} else {
		rows := [][]string{{text.ColID, text.ColRemark, text.ColProtocol, text.ColPort, text.ColClients, text.ColState}}
		for i, n := range st.Nodes {
			state := u.s.Colors.Off(text.StateDisabled)
			if n.Enabled {
				state = u.s.Colors.Good(text.StateEnabled)
			}
			rows = append(rows, []string{strconv.Itoa(i + 1), n.Remark, u.protocolLabel(n), strconv.Itoa(n.Port), strconv.Itoa(len(n.Clients)), state})
		}
		status = append(status, ui.Table(rows)...)
	}
	none := len(st.Nodes) == 0
	return ui.Menu{
		Heading: text.MenuNodes,
		Status:  status,
		Items: []ui.Item{
			{Label: text.NodeAdd, Action: u.add},
			{Label: text.NodeModify, Hidden: none, Page: true, Action: u.modify},
			{Label: text.NodeToggle, Hidden: none, Action: u.toggle},
			{Label: text.NodeClients, Hidden: none, Page: true, Action: u.clients},
			{Label: text.NodeShareAddress, Hint: text.NodeShareAddrHint, Page: true, Action: u.shareAddress},
			{Label: text.NodeDelete, Hidden: none, Action: u.delete},
		},
	}
}

// chooseNode asks for a node's sequence number; nil means back.
func (u *nodeUI) chooseNode() (*node.Node, error) {
	st := u.state()
	if len(st.Nodes) == 0 {
		return nil, nil
	}
	i, err := u.s.PromptNumber(text.PromptNodeNumber, -1, 0, len(st.Nodes))
	if err != nil || i == 0 {
		return nil, err
	}
	n := st.Nodes[i-1]
	return &n, nil
}

// add runs the add-node flow (DESIGN section 9.2).
func (u *nodeUI) add() error {
	presets := u.svc.Registry.Presets()
	u.s.Step(1, 4, text.ChoosePreset)
	m := ui.Menu{Heading: text.ChoosePreset}
	for _, p := range presets {
		m.Items = append(m.Items, ui.Item{Label: p.Label})
	}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return err
	}
	preset := presets[choice-1]

	addr, err := u.ensureShareAddresses()
	if err != nil || addr == "" {
		return err // "" : the user went back
	}
	u.s.Step(2, 4, text.PromptPort)
	port, err := u.askNewPort(addr)
	if err != nil {
		return err
	}
	target, err := u.chooseTarget(3, 4)
	if err != nil || target == "" {
		return err
	}
	u.s.Step(4, 4, text.PromptRemark)
	remark, err := u.askRemark(u.defaultRemark(port), 0, true)
	if err != nil {
		return err
	}
	if _, ok := u.svc.Xray.Installed(u.ctx); !ok {
		u.s.Println(text.InstallingXray)
	}
	u.s.Println(text.ApplyingChange)
	n, err := u.svc.Add(u.ctx, nodemgr.AddInput{PresetID: preset.ID, Port: port, Target: target, Remark: remark})
	if err != nil {
		return err
	}
	u.s.Done(text.NodeAdded)
	u.showLink(n, n.Clients[0], addr, true, true)
	if len(u.state().Server.ShareAddresses) > 1 {
		u.s.Println(text.OtherAddressesHint)
	}
	return nil
}

// showLink prints a client's link for addr, optionally with a QR code and
// hints. isDefault: addr is the default address (the remark has no
// suffix, D-068).
func (u *nodeUI) showLink(n node.Node, c node.Client, addr string, isDefault, full bool) {
	link, err := share.LinkFor(u.svc.Registry, n, c, addr, isDefault)
	if err != nil {
		u.s.Println(u.s.Colors.Bad(text.ActionFailed(err)))
		return
	}
	u.s.Println("\n" + text.ShareLinkTitle)
	u.s.Println(link)
	if !full {
		return
	}
	if qr, err := share.QR(link, u.s.Colors.Enabled); err == nil {
		u.s.Println()
		fmt.Fprint(u.s.Out, qr)
	}
	port := n.Port
	if n.SharePort > 0 {
		port = n.SharePort
	}
	u.s.Println(text.SecurityGroup(port))
	if !ufw.Active(u.ctx, u.svc.Runner) {
		if f := firewall.DetectForeignFilter(u.ctx, u.svc.Runner); f.Found() {
			u.s.Println(u.s.Colors.Warn(text.ForeignFilter(n.Port, f.Where)))
		}
	}
}

// printLinks lists the new links of all enabled clients of a node.
func (u *nodeUI) printLinks(id int64) {
	st := u.state()
	for _, n := range st.Nodes {
		if n.ID != id {
			continue
		}
		u.s.Println("\n" + text.NewLinksTitle)
		for _, c := range n.Clients {
			if !c.Enabled {
				continue
			}
			if link, err := share.Link(u.svc.Registry, n, c, st.Server.DefaultShareAddress()); err == nil {
				u.s.Println(c.Remark + ":")
				u.s.Println(link)
			}
		}
	}
}

// askNewPort asks for a free port for a new node: 443 if free, else a
// random free port (DESIGN section 9.2 step 2).
func (u *nodeUI) askNewPort(addr string) (int, error) {
	st := u.state()
	pc, err := u.svc.PortChecker(st)
	if err != nil {
		return 0, err
	}
	def := 443
	if len(pc.Check([]ports.Key{{Proto: ports.TCP, Port: def}}, "", nil)) > 0 {
		def = pc.RandomFree(nil)
	}
	if ip := net.ParseIP(addr); ip != nil && !u.detector.OnInterface(addr) {
		u.s.Println(u.s.Colors.Warn(text.NATHint))
	}
	for {
		port, err := u.s.PromptNumber(text.PromptPort, def, 1, 65535)
		if err != nil {
			return 0, err
		}
		if conflicts := pc.Check([]ports.Key{{Proto: ports.TCP, Port: port}}, "", nil); len(conflicts) > 0 {
			for _, c := range conflicts {
				u.s.Println(u.s.Colors.Warn(text.PortTaken(c.Port, string(c.Proto), c.Owner)))
			}
			continue
		}
		if port != 443 {
			u.s.Println(u.s.Colors.Warn(text.Non443Hint))
		}
		return port, nil
	}
}

func (u *nodeUI) defaultRemark(port int) string {
	st := u.state()
	base := "Reality-" + strconv.Itoa(port)
	r := base
	for i := 2; node.CheckNodeRemark(st.Nodes, r, 0) != nil || node.CheckClientRemark(st.Nodes, r, 0) != nil; i++ {
		r = base + "-" + strconv.Itoa(i)
	}
	return r
}

// askRemark asks for a node remark until it is valid and unique. For a
// new node the first client takes the same remark, so it must also be
// unique among clients.
func (u *nodeUI) askRemark(def string, id int64, newNode bool) (string, error) {
	for {
		in, err := u.s.PromptDefault(text.PromptRemark, def)
		if err != nil {
			return "", err
		}
		r, err := node.NormalizeRemark(in)
		if err == nil {
			st := u.state()
			err = node.CheckNodeRemark(st.Nodes, r, id)
			if err == nil && newNode {
				err = node.CheckClientRemark(st.Nodes, r, 0)
			}
		}
		if err == nil {
			return r, nil
		}
		u.s.Println(u.s.Colors.Warn(describe(err)))
	}
}

// prepareChecker tells the checker whether the Xray in use (or the one
// that will be installed for the first node) handles ML-KEM targets.
func (u *nodeUI) prepareChecker() {
	a, ok := u.checker.(mlkemAware)
	if !ok {
		return
	}
	v, installed := u.svc.Xray.Installed(u.ctx)
	if !installed {
		v, _ = u.svc.Xray.DefaultVersion(u.ctx)
	}
	a.SetMLKEMSupported(xray.SupportsMLKEMTargets(v))
}

// chooseTarget lets the user pick a checked REALITY target (DESIGN
// section 10.1). Going back from a method, or a method finding nothing,
// returns to this menu; "" means the user left it (back). In a flow it
// is step n of total (0: unnumbered).
func (u *nodeUI) chooseTarget(n, total int) (string, error) {
	u.prepareChecker()
	m := ui.Menu{Heading: text.ChooseTarget, Items: []ui.Item{
		{Label: text.TargetScan, Hint: text.TargetScanHint}, {Label: text.TargetCommon}, {Label: text.TargetManual}}}
	for {
		u.s.Step(n, total, text.ChooseTarget)
		choice, err := u.s.Choose(m)
		if err != nil || choice == 0 {
			return "", err
		}
		var t string
		switch choice {
		case 1:
			t, err = u.scanTarget()
		case 2:
			t, err = u.commonTarget()
		default:
			t, err = u.manualTarget()
		}
		if err != nil || t != "" {
			return t, err
		}
	}
}

// chooseResult offers checked targets as a menu. "" means back.
func (u *nodeUI) chooseResult(heading, status string, results []reality.Result) (string, error) {
	width := 0
	for _, r := range results {
		width = max(width, ui.Width(r.Host))
	}
	m := ui.Menu{Heading: heading}
	if status != "" {
		m.Status = []string{status}
	}
	for _, r := range results {
		m.Items = append(m.Items, ui.Item{Label: text.CheckLine(ui.PadRight(r.Host, width), r.Latency.Milliseconds(), r.TLS13, r.H2, r.X25519, r.CertOK)})
	}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return "", err
	}
	return results[choice-1].Target, nil
}

// commonTarget offers the usable sites of the common list (D-059). ""
// means back or none usable.
func (u *nodeUI) commonTarget() (string, error) {
	u.s.Println(text.Checking)
	var usable []reality.Result
	for _, r := range u.checker.CheckAll(u.ctx, reality.CommonTargets, 10) {
		if r.Usable() {
			usable = append(usable, r)
		}
	}
	if len(usable) == 0 {
		u.s.Println(u.s.Colors.Warn(text.TargetNoneUsable))
		return "", nil
	}
	return u.chooseResult(text.CommonHeading, text.CommonListHint, usable)
}

// scanTarget scans nearby sites (DESIGN section 10.3, D-058). "" with no
// error means the user declined or went back, or nothing usable was found.
func (u *nodeUI) scanTarget() (string, error) {
	st := u.state()
	if !st.Server.ScanWarned {
		ok, err := u.s.Confirm(text.ScanWarning)
		if err != nil || !ok {
			return "", err
		}
		st.Server.ScanWarned = true
		if err := u.svc.Store.Save(st); err != nil {
			return "", err
		}
	}
	center, ok := u.scanCenter()
	if !ok {
		u.s.Println(u.s.Colors.Warn(text.ScanNoAddress))
		return "", nil
	}
	u.s.Println(text.ScanStart(netip.PrefixFrom(center, 24).Masked().String()))
	last := 0
	results, err := u.scanner.Scan(u.ctx, center, func(done, total int) {
		if done == total || done-last >= 50 {
			last = done
			u.s.Println(text.ScanProgress(done, total))
		}
	})
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		u.s.Println(u.s.Colors.Warn(text.ScanNoResult))
		return "", nil
	}
	return u.chooseResult(text.ScanHeading, text.ScanFound, results)
}

// scanCenter is the VPS's public IPv4: the first IPv4 share address,
// otherwise the detected address.
func (u *nodeUI) scanCenter() (netip.Addr, bool) {
	for _, addr := range u.state().Server.ShareAddresses {
		if a, err := netip.ParseAddr(addr); err == nil && a.Unmap().Is4() {
			return a.Unmap(), true
		}
	}
	if d := u.detector.Detect(u.ctx); d.IPv4 != "" {
		if a, err := netip.ParseAddr(d.IPv4); err == nil {
			return a, true
		}
	}
	return netip.Addr{}, false
}

func (u *nodeUI) manualTarget() (string, error) {
	for {
		in, err := u.s.Prompt(text.PromptTarget + ": ")
		if err != nil {
			return "", err
		}
		if in == "" {
			return "", nil
		}
		if _, _, err := reality.SplitTarget(in); err != nil {
			u.s.Println(u.s.Colors.Warn(text.ActionFailed(err)))
			continue
		}
		u.s.Println(text.Checking)
		r := u.checker.Check(u.ctx, in)
		u.printCheck(r)
		if !r.Usable() {
			u.s.Println(u.s.Colors.Bad(text.TargetNotUsable))
			continue
		}
		if r.Keyword != "" || r.LargeHomepage() {
			ok, err := u.s.Confirm(text.TargetUseAnyway)
			if err != nil {
				return "", err
			}
			if !ok {
				continue
			}
		}
		return r.Target, nil
	}
}

func (u *nodeUI) printCheck(r reality.Result) {
	if r.Err != nil {
		u.s.Println(text.CheckUnreachable(r.Host, r.Err))
		return
	}
	u.s.Println(text.CheckLine(r.Host, r.Latency.Milliseconds(), r.TLS13, r.H2, r.X25519, r.CertOK))
	if r.MLKEM && !r.MLKEMSupported {
		u.s.Println(u.s.Colors.Warn(text.MLKEMUnsupported(xray.MinMLKEMVersion)))
	}
	u.s.Println(text.HomepageSize(r.HomepageSize))
	if r.Keyword != "" {
		u.s.Println(u.s.Colors.Warn(text.RiskyKeyword(r.Keyword)))
	}
	if r.LargeHomepage() {
		u.s.Println(u.s.Colors.Warn(text.LargeHomepage(r.HomepageSize)))
	}
}

// toggle enables or disables a node.
func (u *nodeUI) toggle() error {
	n, err := u.chooseNode()
	if err != nil || n == nil {
		return err
	}
	if err := u.svc.SetEnabled(u.ctx, n.ID, !n.Enabled); err != nil {
		return err
	}
	if n.Enabled {
		u.s.Done(text.NodeDisabled)
	} else {
		u.s.Done(text.NodeEnabled)
	}
	return nil
}

// delete removes a node after confirmation.
func (u *nodeUI) delete() error {
	n, err := u.chooseNode()
	if err != nil || n == nil {
		return err
	}
	ok, err := u.s.Confirm(text.ConfirmDelete(n.Remark, len(n.Clients)))
	if err != nil || !ok {
		return err
	}
	if err := u.svc.Delete(u.ctx, n.ID); err != nil {
		return err
	}
	u.s.Done(text.NodeDeleted)
	return nil
}

// describe maps known errors to user-facing text.
func describe(err error) string {
	var pt *nodemgr.PortTakenError
	var ce *change.Error
	var ie *nodemgr.XrayInstallError
	// A failed restore matters most; otherwise SSH errors have their own
	// text, also when wrapped in a change error (restored).
	if !errors.As(err, &ce) || ce.RestoreErr == nil {
		if msg, ok := describeSSH(err); ok {
			return msg
		}
	}
	switch {
	case errors.Is(err, node.ErrRemarkEmpty):
		return text.ErrRemarkEmpty
	case errors.Is(err, node.ErrRemarkTooLong):
		return text.ErrRemarkTooLong(node.MaxRemarkLen)
	case errors.Is(err, node.ErrRemarkControl):
		return text.ErrRemarkControl
	case errors.Is(err, node.ErrRemarkDuplicate):
		return text.ErrRemarkDuplicate
	case errors.Is(err, nodemgr.ErrNotFound):
		return text.ErrNodeNotFound
	case errors.Is(err, nodemgr.ErrClientNotFound):
		return text.ErrClientNotFound
	case errors.As(err, &pt):
		msg := ""
		for i, c := range pt.Conflicts {
			if i > 0 {
				msg += "\n"
			}
			msg += text.PortTaken(c.Port, string(c.Proto), c.Owner)
		}
		return msg
	case errors.As(err, &ie):
		return text.XrayInstallFailed(ie.Err)
	case errors.As(err, &ce):
		return text.ChangeFailed(ce, ce.RestoreErr == nil)
	}
	return text.ActionFailed(err)
}
