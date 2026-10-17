package ui

import (
	"os"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
)

func mainMenuFixture(status []string) Menu {
	return Menu{
		Title:     text.MainTitle("v0.1.0"),
		Status:    status,
		Grouped:   true,
		ExitLabel: text.Exit,
		Items: []Item{
			{Label: text.MenuNodes, Group: 1},
			{Label: text.MenuForwards, Group: 1},
			{Label: text.MenuXray, Group: 1},
			{Label: text.MenuSSH, Group: 2},
			{Label: text.MenuFirewall, Group: 2},
			{Label: text.MenuSysopt, Group: 2},
			{Label: text.MenuMaint, Hint: text.MenuMaintHint, Group: 3},
		},
	}
}

// TestRenderMainMenu compares against the DESIGN section 8 example.
func TestRenderMainMenu(t *testing.T) {
	want, err := os.ReadFile("testdata/main_menu.golden")
	if err != nil {
		t.Fatal(err)
	}
	info := system.Info{
		OS:         system.OSRelease{ID: "debian", VersionID: "12"},
		Virt:       system.Virt{Type: "kvm"},
		MemTotalMB: 512, BBREnabled: true,
	}
	status := []string{
		PadRight(PadRight("Xray: ● 运行中", statusCol2)+text.StatusNodes(2), statusCol3) + text.StatusForwards(3),
		PadRight(text.StatusSSH("22222 / 仅密钥"), statusCol3) + text.StatusFirewall("● 已启用"),
		SystemLine(info),
	}
	if got := mainMenuFixture(status).Render(); got != string(want) {
		t.Errorf("main menu mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMainStatusLines(t *testing.T) {
	info := system.Info{OS: system.OSRelease{ID: "debian", VersionID: "12"}, Virt: system.Virt{Type: "kvm"}, MemTotalMB: 512}
	got := MainStatusLines(MainStatus{System: info}, Colors{})
	want := []string{
		"Xray: ○ 未安装   节点: 0      转发规则: 0",
		"SSH: -                        防火墙: -",
		"Debian 12 | KVM | 512M | BBR ✗",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}

	got = MainStatusLines(MainStatus{XrayInstalled: true, Nodes: 2, Forwards: 3,
		SSH: "22222 / 仅密钥", Firewall: "● 已启用", System: info}, Colors{})
	if got[0] != "Xray: ● 已安装   节点: 2      转发规则: 3" ||
		got[1] != "SSH: 22222 / 仅密钥           防火墙: ● 已启用" {
		t.Errorf("got %q", got)
	}
}

func TestRenderSubmenuHiddenItems(t *testing.T) {
	m := Menu{
		Status: []string{"Debian 12 | LXC | 512M | BBR ✗"},
		Notice: "部分功能在当前环境（LXC）中不可用",
		Items: []Item{
			{Label: "开启 BBR", Hidden: true},
			{Label: "swap 管理", Hidden: true},
			{Label: "IPv4 / IPv6 优先级"},
			{Label: "DNS"},
		},
	}
	want := ` Debian 12 | LXC | 512M | BBR ✗
 部分功能在当前环境（LXC）中不可用
──────────────────────────────────────────────
 1. IPv4 / IPv6 优先级
 2. DNS
──────────────────────────────────────────────
 0. 返回
`
	if got := m.Render(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderHintColumnGrowsForLongLabels(t *testing.T) {
	m := Menu{Items: []Item{
		{Label: "更新 Xray", Hint: "(可选指定版本)"},
		{Label: "卸载 Xray", Hint: "(同时删除所有节点和客户端)"},
	}}
	want := `──────────────────────────────────────────────
 1. 更新 Xray       (可选指定版本)
 2. 卸载 Xray       (同时删除所有节点和客户端)
──────────────────────────────────────────────
 0. 返回
`
	if got := m.Render(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWidth(t *testing.T) {
	tests := map[string]int{
		"abc":                  3,
		"节点管理":                 8,
		"（LXC）":                7,
		"● 运行中":                8,
		"\x1b[32m● 运行中\x1b[0m": 8,
		"ufw、Docker":           11,
	}
	for s, want := range tests {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestColors(t *testing.T) {
	if got := NewColors("1").Good("x"); got != "x" {
		t.Errorf("NO_COLOR set: got %q", got)
	}
	if got := NewColors("").Good("x"); got != "\x1b[32mx\x1b[0m" {
		t.Errorf("colors on: got %q", got)
	}
}
