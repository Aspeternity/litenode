// Package ui renders menus and reads user input. It contains no business
// logic.
package ui

import (
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
)

// virtNames maps systemd-detect-virt results to display names. Unlisted
// types are shown as reported.
var virtNames = map[string]string{
	"kvm": "KVM", "qemu": "QEMU", "xen": "Xen", "vmware": "VMware",
	"microsoft": "Hyper-V", "oracle": "VirtualBox", "amazon": "AWS",
	"google": "GCE", "bochs": "Bochs", "parallels": "Parallels",
	"openvz": "OpenVZ", "lxc": "LXC", "lxc-libvirt": "LXC",
	"systemd-nspawn": "nspawn", "docker": "Docker", "podman": "Podman",
	"wsl": "WSL",
}

// SystemLine renders the system line of the main menu status area,
// e.g. "Debian 12 | KVM | 973M | BBR ✓".
func SystemLine(info system.Info) string {
	parts := []string{osName(info.OS), VirtName(info.Virt)}
	if info.MemTotalMB > 0 {
		parts = append(parts, memSize(info.MemTotalMB))
	}
	parts = append(parts, text.BBR(info.BBREnabled))
	return strings.Join(parts, " | ")
}

func osName(o system.OSRelease) string {
	if n := o.ShortName(); n != "" {
		return n
	}
	return text.UnknownOS
}

// VirtName is the display name of a virtualization type.
func VirtName(v system.Virt) string {
	switch v.Type {
	case "":
		return text.VirtUnknown
	case "none":
		return text.VirtNone
	}
	if n, ok := virtNames[v.Type]; ok {
		return n
	}
	return v.Type
}

// memSize formats MiB as "512M" below 1 GiB and "1.9G" / "2G" above.
func memSize(mb int) string {
	if mb < 1024 {
		return fmt.Sprintf("%dM", mb)
	}
	g := fmt.Sprintf("%.1f", float64(mb)/1024)
	return strings.TrimSuffix(g, ".0") + "G"
}

// Main status area column positions, from the DESIGN section 8 example.
const (
	statusCol2 = 17
	statusCol3 = 30
)

// MainStatus is the data shown in the main menu status area.
type MainStatus struct {
	// Xray is the pre-rendered Xray state; empty shows "not installed".
	Xray     string
	Nodes    int
	Forwards int
	// SSH and Firewall are pre-rendered by their modules; empty shows "-".
	SSH      string
	Firewall string
	System   system.Info
	// Warnings are shown below the system line, e.g. when the LiteNode
	// firewall and ufw are both active (DESIGN section 2).
	Warnings []string
}

// column pads s to width, keeping at least two spaces before the next
// field when s is wider than its column.
func column(s string, width int) string {
	if Width(s)+2 > width {
		return s + "  "
	}
	return PadRight(s, width)
}

// MainStatusLines renders the main menu status area.
func MainStatusLines(st MainStatus, c Colors) []string {
	xray := st.Xray
	if xray == "" {
		xray = c.Off(text.XrayNotInstalled)
	}
	ssh, fw := st.SSH, st.Firewall
	if ssh == "" {
		ssh = text.StatusUnknown
	}
	if fw == "" {
		fw = text.StatusUnknown
	}
	lines := []string{
		column(column(text.StatusXray(xray), statusCol2)+text.StatusNodes(st.Nodes), statusCol3) +
			text.StatusForwards(st.Forwards),
		column(text.StatusSSH(ssh), statusCol3) + text.StatusFirewall(fw),
		SystemLine(st.System),
	}
	for _, w := range st.Warnings {
		lines = append(lines, c.Warn(text.Warning(w)))
	}
	return lines
}
