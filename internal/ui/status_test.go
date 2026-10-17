package ui

import (
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
)

func TestSystemLine(t *testing.T) {
	deb12 := system.OSRelease{ID: "debian", VersionID: "12"}
	tests := []struct {
		name string
		info system.Info
		want string
	}{
		{"kvm", system.Info{OS: deb12, Virt: system.Virt{Type: "kvm"}, MemTotalMB: 973, BBREnabled: true},
			"Debian 12 | KVM | 973M | BBR ✓"},
		{"lxc large memory", system.Info{OS: system.OSRelease{ID: "ubuntu", VersionID: "24.04"},
			Virt: system.Virt{Type: "lxc", Container: true}, MemTotalMB: 2048},
			"Ubuntu 24.04 | LXC | 2G | BBR ✗"},
		{"bare metal", system.Info{OS: deb12, Virt: system.Virt{Type: "none"}, MemTotalMB: 1946},
			"Debian 12 | 物理机 | 1.9G | BBR ✗"},
		{"unknown virt and memory", system.Info{OS: deb12},
			"Debian 12 | 未知虚拟化 | BBR ✗"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SystemLine(tt.info); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMainStatusWarnings(t *testing.T) {
	info := system.Info{OS: system.OSRelease{ID: "debian", VersionID: "12"}, Virt: system.Virt{Type: "kvm"}}
	if got := MainStatusLines(MainStatus{System: info}, Colors{}); len(got) != 3 {
		t.Errorf("no warnings: got %d lines, want 3", len(got))
	}
	got := MainStatusLines(MainStatus{System: info, Warnings: []string{"a", "b"}}, Colors{})
	if len(got) != 5 || got[3] != "⚠ a" || got[4] != "⚠ b" {
		t.Errorf("got %q", got)
	}
}
