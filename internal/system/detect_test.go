package system_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

// debian12 is /etc/os-release from the Debian 12 test VPS.
const debian12 = `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION="12 (bookworm)"
VERSION_CODENAME=bookworm
ID=debian
HOME_URL="https://www.debian.org/"
SUPPORT_URL="https://www.debian.org/support"
BUG_REPORT_URL="https://bugs.debian.org/"
`

func TestOSReleaseSupported(t *testing.T) {
	tests := []struct {
		name, data string
		short      string
		want       bool
	}{
		{"debian 12", debian12, "Debian 12", true},
		{"debian 11", "ID=debian\nVERSION_ID=\"11\"\n", "Debian 11", true},
		{"debian 13", "ID=debian\nVERSION_ID=\"13\"\n", "Debian 13", true},
		{"debian 14 future", "ID=debian\nVERSION_ID=\"14\"\n", "Debian 14", true},
		{"debian 10", "ID=debian\nVERSION_ID=\"10\"\n", "Debian 10", false},
		{"debian sid", "PRETTY_NAME=\"Debian GNU/Linux trixie/sid\"\nID=debian\n", "Debian GNU/Linux trixie/sid", false},
		{"ubuntu 22.04", "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"22.04\"\n", "Ubuntu 22.04", true},
		{"ubuntu 24.04", "ID=ubuntu\nVERSION_ID=\"24.04\"\n", "Ubuntu 24.04", true},
		{"ubuntu 26.04 future", "ID=ubuntu\nVERSION_ID=\"26.04\"\n", "Ubuntu 26.04", true},
		{"ubuntu 20.04", "ID=ubuntu\nVERSION_ID=\"20.04\"\n", "Ubuntu 20.04", false},
		{"ubuntu 24.10 interim", "ID=ubuntu\nVERSION_ID=\"24.10\"\n", "Ubuntu 24.10", false},
		{"ubuntu 23.04 interim", "ID=ubuntu\nVERSION_ID=\"23.04\"\n", "Ubuntu 23.04", false},
		{"linux mint", "PRETTY_NAME=\"Linux Mint 22\"\nID=linuxmint\nID_LIKE=\"ubuntu debian\"\nVERSION_ID=\"22\"\n", "Linux Mint 22", false},
		{"centos", "PRETTY_NAME=\"CentOS Stream 9\"\nID=\"centos\"\nVERSION_ID=\"9\"\n", "CentOS Stream 9", false},
		{"alpine", "ID=alpine\nVERSION_ID=3.20.0\nPRETTY_NAME=\"Alpine Linux v3.20\"\n", "Alpine Linux v3.20", false},
		{"single quotes and comments", "# comment\nID='debian'\nVERSION_ID='12'\n", "Debian 12", true},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := system.ParseOSRelease([]byte(tt.data))
			if got := o.Supported(); got != tt.want {
				t.Errorf("Supported() = %v, want %v (%+v)", got, tt.want, o)
			}
			if got := o.ShortName(); got != tt.short {
				t.Errorf("ShortName() = %q, want %q", got, tt.short)
			}
		})
	}
}

func TestDetectVirt(t *testing.T) {
	tests := []struct {
		name string
		resp systemtest.Response
		want system.Virt
	}{
		{"kvm", systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, system.Virt{Type: "kvm"}},
		{"lxc", systemtest.Response{Result: system.Result{Stdout: "lxc\n"}}, system.Virt{Type: "lxc", Container: true}},
		{"openvz", systemtest.Response{Result: system.Result{Stdout: "openvz\n"}}, system.Virt{Type: "openvz", Container: true}},
		{"wsl", systemtest.Response{Result: system.Result{Stdout: "wsl\n"}}, system.Virt{Type: "wsl", Container: true}},
		{"bare metal exits 1", systemtest.Response{Result: system.Result{Stdout: "none\n", ExitCode: 1}}, system.Virt{Type: "none"}},
		{"missing command", systemtest.Response{Err: errors.New("not found")}, system.Virt{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := systemtest.New()
			f.Respond(tt.resp, "systemd-detect-virt")
			if got := system.DetectVirt(context.Background(), f); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseMemTotalMB(t *testing.T) {
	data := "MemTotal:         996720 kB\nMemFree:          245760 kB\n"
	if got := system.ParseMemTotalMB([]byte(data)); got != 973 {
		t.Errorf("got %d, want 973", got)
	}
	if got := system.ParseMemTotalMB([]byte("garbage")); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestDockerInstalled(t *testing.T) {
	args := []string{"list-unit-files", "--no-legend", "--plain", "docker.service"}
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "docker.service enabled enabled\n"}}, "systemctl", args...)
	if !system.DockerInstalled(context.Background(), f) {
		t.Error("docker.service listed: want true")
	}
	none := systemtest.New()
	none.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "systemctl", args...)
	if system.DockerInstalled(context.Background(), none) {
		t.Error("no unit: want false")
	}
}

func TestDetectDoesNotProbeOtherSoftware(t *testing.T) {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte(debian12)
	if _, err := system.Detect(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c.Command, "x-ui") || strings.Contains(c.Command, "xray.service") || strings.Contains(c.Command, "list-unit-files") {
			t.Errorf("detection probes software by name (D-037): %s", c.Command)
		}
	}
}

func TestDetect(t *testing.T) {
	f := systemtest.New()
	f.Files["/etc/os-release"] = []byte(debian12)
	f.Files["/proc/meminfo"] = []byte("MemTotal:         996720 kB\n")
	f.Files["/proc/sys/net/ipv4/tcp_congestion_control"] = []byte("bbr\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")

	info, err := system.Detect(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if info.OS.ShortName() != "Debian 12" || info.Virt.Type != "kvm" || info.MemTotalMB != 973 || !info.BBREnabled {
		t.Errorf("unexpected info %+v", info)
	}
	if m := f.Mutations(); len(m) != 0 {
		t.Errorf("detection must not mutate: %v", m)
	}

	if _, err := system.Detect(context.Background(), systemtest.New()); err == nil {
		t.Error("want error without /etc/os-release")
	}
}

func TestProbeBBR(t *testing.T) {
	ctx := context.Background()

	listed := systemtest.New()
	listed.Files["/proc/sys/net/ipv4/tcp_available_congestion_control"] = []byte("reno cubic bbr\n")
	if !system.ProbeAll(ctx, listed).Has(system.CapBBR) {
		t.Error("bbr listed: want available")
	}

	module := systemtest.New()
	module.Files["/proc/sys/net/ipv4/tcp_available_congestion_control"] = []byte("reno cubic\n")
	if !system.ProbeAll(ctx, module).Has(system.CapBBR) {
		t.Error("tcp_bbr module present: want available")
	}

	missing := systemtest.New()
	missing.Files["/proc/sys/net/ipv4/tcp_available_congestion_control"] = []byte("reno cubic\n")
	missing.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "modinfo", "-n", "tcp_bbr")
	if system.ProbeAll(ctx, missing).Has(system.CapBBR) {
		t.Error("no bbr: want unavailable")
	}
}

func TestProbeSwap(t *testing.T) {
	ctx := context.Background()
	kvm := systemtest.New()
	kvm.Files["/proc/swaps"] = []byte("Filename\tType\tSize\tUsed\tPriority\n")
	kvm.Respond(systemtest.Response{Result: system.Result{Stdout: "kvm\n"}}, "systemd-detect-virt")
	if !system.ProbeAll(ctx, kvm).Has(system.CapSwap) {
		t.Error("KVM: want swap available")
	}
	lxc := systemtest.New()
	lxc.Files["/proc/swaps"] = kvm.Files["/proc/swaps"]
	lxc.Respond(systemtest.Response{Result: system.Result{Stdout: "lxc\n"}}, "systemd-detect-virt")
	if system.ProbeAll(ctx, lxc).Has(system.CapSwap) {
		t.Error("LXC: want swap unavailable")
	}
	if system.ProbeAll(ctx, systemtest.New()).Has(system.CapSwap) {
		t.Error("no /proc/swaps: want swap unavailable")
	}
}
