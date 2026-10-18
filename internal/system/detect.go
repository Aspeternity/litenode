package system

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// OSRelease holds the fields of /etc/os-release that LiteNode uses.
type OSRelease struct {
	ID         string
	VersionID  string
	PrettyName string
}

// ParseOSRelease parses the contents of /etc/os-release.
func ParseOSRelease(data []byte) OSRelease {
	var o OSRelease
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if uq, err := strconv.Unquote(val); err == nil {
			val = uq
		} else {
			val = strings.Trim(val, `'"`)
		}
		switch key {
		case "ID":
			o.ID = val
		case "VERSION_ID":
			o.VersionID = val
		case "PRETTY_NAME":
			o.PrettyName = val
		}
	}
	return o
}

// Supported reports whether LiteNode supports this system: Debian 11 or
// later, or an Ubuntu LTS release from 22.04 on (YY.04 with an even YY).
// Derivatives, Debian testing/sid (no VERSION_ID) and Ubuntu interim
// releases are not supported.
func (o OSRelease) Supported() bool {
	switch o.ID {
	case "debian":
		v, err := strconv.Atoi(o.VersionID)
		return err == nil && v >= 11
	case "ubuntu":
		yy, mm, ok := strings.Cut(o.VersionID, ".")
		if !ok || mm != "04" {
			return false
		}
		y, err := strconv.Atoi(yy)
		return err == nil && y >= 22 && y%2 == 0
	}
	return false
}

// ShortName returns a compact name such as "Debian 12" or "Ubuntu 24.04".
func (o OSRelease) ShortName() string {
	if o.VersionID == "" && o.PrettyName != "" {
		return o.PrettyName
	}
	switch o.ID {
	case "debian":
		return "Debian " + o.VersionID
	case "ubuntu":
		return "Ubuntu " + o.VersionID
	}
	if o.PrettyName != "" {
		return o.PrettyName
	}
	return strings.TrimSpace(o.ID + " " + o.VersionID)
}

// containerVirt lists systemd-detect-virt results that are containers.
var containerVirt = map[string]bool{
	"openvz": true, "lxc": true, "lxc-libvirt": true, "systemd-nspawn": true,
	"docker": true, "podman": true, "rkt": true, "wsl": true, "proot": true,
	"pouch": true, "container-other": true,
}

// Virt is the virtualization type reported by systemd-detect-virt.
type Virt struct {
	// Type is the raw result, e.g. "kvm", "lxc" or "none". Empty if unknown.
	Type string
	// Container is true for container environments (LXC, OpenVZ, WSL, ...),
	// where kernel-level features such as BBR, swap or sysctl may be missing.
	Container bool
}

// Info is the result of system detection.
type Info struct {
	OS         OSRelease
	Arch       string
	Virt       Virt
	MemTotalMB int
	// BBREnabled reports whether BBR is the current TCP congestion control.
	BBREnabled bool
}

// ReadOSRelease reads and parses /etc/os-release.
func ReadOSRelease(r Runner) (OSRelease, error) {
	data, err := r.ReadFile("/etc/os-release")
	if err != nil {
		return OSRelease{}, fmt.Errorf("read os-release: %w", err)
	}
	return ParseOSRelease(data), nil
}

// Detect gathers system information. Only reading /etc/os-release is
// required; other probes fall back to zero values when they fail.
func Detect(ctx context.Context, r Runner) (Info, error) {
	osr, err := ReadOSRelease(r)
	if err != nil {
		return Info{}, err
	}
	info := Info{
		OS:         osr,
		Arch:       runtime.GOARCH,
		Virt:       DetectVirt(ctx, r),
		BBREnabled: BBREnabled(r),
	}
	if data, err := r.ReadFile("/proc/meminfo"); err == nil {
		info.MemTotalMB = ParseMemTotalMB(data)
	}
	return info, nil
}

// DetectVirt runs systemd-detect-virt. It prints "none" and exits 1 on bare
// metal, so a non-zero exit with output is not treated as a failure.
func DetectVirt(ctx context.Context, r Runner) Virt {
	res, err := r.Query(ctx, "systemd-detect-virt")
	t := strings.TrimSpace(res.Stdout)
	var ce *CommandError
	if err != nil && (!errors.As(err, &ce) || t == "") {
		return Virt{}
	}
	return Virt{Type: t, Container: containerVirt[t]}
}

// ParseMemTotalMB returns MemTotal from /proc/meminfo in MiB, or 0.
func ParseMemTotalMB(data []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && f[0] == "MemTotal:" {
			kb, err := strconv.Atoi(f[1])
			if err != nil {
				return 0
			}
			return kb / 1024
		}
	}
	return 0
}

// BBREnabled reports whether BBR is the current TCP congestion control.
func BBREnabled(r Runner) bool {
	data, err := r.ReadFile("/proc/sys/net/ipv4/tcp_congestion_control")
	return err == nil && strings.TrimSpace(string(data)) == "bbr"
}

// DockerInstalled reports whether docker.service exists. Docker rewrites
// nftables/iptables rules, so enabling the LiteNode firewall warns about it
// (DESIGN section 14). Other software is not detected by name: port
// conflicts are judged by the ports actually in use (D-037).
func DockerInstalled(ctx context.Context, r Runner) bool {
	// systemctl exits non-zero when no unit matches; the output is still valid.
	res, _ := r.Query(ctx, "systemctl", "list-unit-files", "--no-legend", "--plain", "docker.service")
	for line := range strings.Lines(res.Stdout) {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "docker.service" {
			return true
		}
	}
	return false
}
