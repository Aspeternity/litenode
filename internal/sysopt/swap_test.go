package sysopt

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const swapsHeader = "Filename\t\t\t\tType\t\tSize\t\tUsed\t\tPriority\n"

func TestParseSwaps(t *testing.T) {
	got := ParseSwaps([]byte(swapsHeader +
		"/dev/vda2                               partition\t1048572\t\t0\t\t-2\n" +
		"/litenode.swap                          file\t\t524284\t\t10240\t\t-3\n"))
	want := []SwapEntry{{"/dev/vda2", 1024, 0}, {SwapPath, 512, 10}}
	if !slices.Equal(got, want) {
		t.Errorf("ParseSwaps = %+v", got)
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int{"512": 512, "512M": 512, "1G": 1024, "1.5g": 1536, " 2gb ": 2048} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "abc", "-1", "0", "1T"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q): no error", in)
		}
	}
	if FormatMB(1024) != "1G" || FormatMB(1536) != "1536M" || FormatMB(512) != "512M" {
		t.Error("FormatMB")
	}
	if SuggestMB(973) != 1024 || SuggestMB(2048) != 0 {
		t.Error("SuggestMB")
	}
}

func TestFstabWithSwap(t *testing.T) {
	in := "# /etc/fstab\nUUID=x / ext4 defaults 0 1\n/litenode.swap none swap sw 0 0\n/swapfile none swap sw 0 0"
	want := "# /etc/fstab\nUUID=x / ext4 defaults 0 1\n/swapfile none swap sw 0 0\n"
	if got := string(FstabWithSwap([]byte(in), false)); got != want {
		t.Errorf("without:\n%s", got)
	}
	if got := string(FstabWithSwap([]byte(in), true)); got != want+"/litenode.swap none swap sw 0 0\n" {
		t.Errorf("with:\n%s", got)
	}
}

// swapFake simulates swapon / swapoff in /proc/swaps.
func swapFake(t *testing.T) (*systemtest.Fake, *Service) {
	f, s := service(t)
	f.Files["/etc/fstab"] = []byte("UUID=x / ext4 defaults 0 1\n")
	f.Files["/proc/swaps"] = []byte(swapsHeader)
	f.Files["/proc/meminfo"] = []byte("MemTotal: 996720 kB\nMemAvailable: 600000 kB\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Filesystem 1M-blocks Used Available Capacity Mounted on\n/dev/vda1 6900 2500 4300 37% /\n"}}, "df", "-P", "-B1M", "/")
	f.OnCommand("swapon "+SwapPath, func() {
		f.Files["/proc/swaps"] = []byte(swapsHeader + SwapPath + " file 1048572 0 -2\n")
	})
	f.OnCommand("swapoff "+SwapPath, func() { f.Files["/proc/swaps"] = []byte(swapsHeader) })
	f.OnCommand("fallocate -l 1024M "+SwapPath, func() { f.Files[SwapPath] = []byte{} })
	f.OnCommand("fallocate -l 2048M "+SwapPath+".new", func() { f.Files[SwapPath+".new"] = []byte{} })
	return f, s
}

func TestSwapCreateResizeDelete(t *testing.T) {
	ctx := context.Background()
	f, s := swapFake(t)
	if err := s.CreateSwap(ctx, 1024); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(f.Files["/etc/fstab"]), "\n/litenode.swap none swap sw 0 0\n") {
		t.Errorf("fstab:\n%s", f.Files["/etc/fstab"])
	}
	sw, _ := s.SwapStatus()
	if sw.Own == nil || sw.TotalMB() != 1024 {
		t.Errorf("status = %+v", sw)
	}
	if err := s.CreateSwap(ctx, 1024); !errors.Is(err, ErrSwapExists) {
		t.Errorf("second create: %v", err)
	}

	if err := s.ResizeSwap(ctx, 2048); err != nil {
		t.Fatal(err)
	}
	execs := strings.Join(f.Execs(), "\n")
	if !strings.Contains(execs, "mkswap /litenode.swap.new\nswapoff /litenode.swap\nswapon /litenode.swap") {
		t.Errorf("resize execs:\n%s", execs)
	}
	if _, ok := f.Files[SwapPath+".new"]; ok {
		t.Error("new file not renamed")
	}

	if err := s.DeleteSwap(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(f.Files["/etc/fstab"]), SwapPath) {
		t.Error("fstab line kept")
	}
	if _, ok := f.Files[SwapPath]; ok {
		t.Error("swap file kept")
	}
	if err := s.DeleteSwap(ctx); !errors.Is(err, ErrNoSwap) {
		t.Errorf("second delete: %v", err)
	}
}

func TestSwapCreateFallbackAndFailure(t *testing.T) {
	ctx := context.Background()
	f, s := swapFake(t)
	f.Respond(systemtest.Response{Err: errors.New("not supported")}, "fallocate", "-l", "1024M", SwapPath)
	f.Respond(systemtest.Response{Err: errors.New("swapon failed")}, "swapon", SwapPath)
	err := s.CreateSwap(ctx, 1024)
	if err == nil {
		t.Fatal("no error")
	}
	if !slices.Contains(f.Execs(), "dd if=/dev/zero of=/litenode.swap bs=1M count=1024 status=none") {
		t.Errorf("no dd fallback: %q", f.Execs())
	}
	if strings.Contains(string(f.Files["/etc/fstab"]), SwapPath) {
		t.Error("fstab not restored")
	}
	if _, ok := f.Files[SwapPath]; ok {
		t.Error("swap file left after failure")
	}
}

func TestSwapLimits(t *testing.T) {
	ctx := context.Background()
	f, s := swapFake(t)
	if err := s.CreateSwap(ctx, 64); !errors.Is(err, ErrSwapSize) {
		t.Errorf("64M: %v", err)
	}
	if err := s.CreateSwap(ctx, 4200); !errors.Is(err, ErrNoDiskSpace) {
		t.Errorf("4200M: %v", err)
	}
	// Swap holding more than the available memory is not turned off.
	f.Files["/etc/fstab"] = []byte(SwapPath + " none swap sw 0 0\n")
	f.Files["/proc/swaps"] = []byte(swapsHeader + SwapPath + " file 1048572 614400 -2\n")
	if err := s.DeleteSwap(ctx); !errors.Is(err, ErrSwapInUse) {
		t.Errorf("delete in use: %v", err)
	}
}
