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
	parts := []string{osName(info.OS), virtName(info.Virt)}
	if info.MemTotalMB > 0 {
		parts = append(parts, memSize(info.MemTotalMB))
	}
	parts = append(parts, text.BBR(info.BBREnabled))
	return strings.Join(parts, " | ")
}

// ConflictLine renders the warning about coexisting software, or "" if none.
func ConflictLine(conflicts []string) string {
	if len(conflicts) == 0 {
		return ""
	}
	names := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		switch c {
		case system.ConflictXUI:
			names = append(names, text.ConflictXUI)
		case system.ConflictXray:
			names = append(names, text.ConflictXray)
		case system.ConflictDocker:
			names = append(names, text.ConflictDocker)
		default:
			names = append(names, c)
		}
	}
	return text.ConflictWarning(names)
}

func osName(o system.OSRelease) string {
	if n := o.ShortName(); n != "" {
		return n
	}
	return text.UnknownOS
}

func virtName(v system.Virt) string {
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
