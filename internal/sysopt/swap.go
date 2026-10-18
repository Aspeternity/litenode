package sysopt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/system"
)

const (
	// SwapPath is LiteNode's swap file. Other swap (partitions, files of
	// the provider) is shown but never changed.
	SwapPath = "/litenode.swap"
	// FstabPath is where the swap file is mounted at boot.
	FstabPath = "/etc/fstab"
	// MinSwapMB and MaxSwapMB bound the swap size.
	MinSwapMB = 128
	MaxSwapMB = 32 * 1024
	// DiskReserveMB is the disk space left free after creating swap.
	DiskReserveMB = 256
	// memMarginMB is the memory left free when swapping out a file.
	memMarginMB = 64
)

// Errors of swap operations.
var (
	ErrSwapExists  = errors.New("swap file exists")
	ErrNoSwap      = errors.New("no LiteNode swap file")
	ErrSwapSize    = errors.New("invalid swap size")
	ErrNoDiskSpace = errors.New("not enough disk space")
	// ErrSwapInUse means the memory cannot hold what the swap file holds,
	// so turning it off could start the OOM killer.
	ErrSwapInUse = errors.New("swap in use exceeds available memory")
)

// SwapEntry is one line of /proc/swaps.
type SwapEntry struct {
	Path   string
	SizeMB int
	UsedMB int
}

// ParseSwaps parses /proc/swaps. Sizes are rounded to MiB: a swap file
// shows one page less than its size.
func ParseSwaps(data []byte) []SwapEntry {
	var out []SwapEntry
	for i, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 4 {
			continue
		}
		size, err1 := strconv.Atoi(f[2])
		used, err2 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil {
			continue
		}
		// Paths with spaces are escaped as \040.
		out = append(out, SwapEntry{Path: strings.ReplaceAll(f[0], `\040`, " "), SizeMB: (size + 512) / 1024, UsedMB: (used + 512) / 1024})
	}
	return out
}

// FormatMB shows a size as "512M" or "1G".
func FormatMB(mb int) string {
	if mb >= 1024 && mb%1024 == 0 {
		return strconv.Itoa(mb/1024) + "G"
	}
	return strconv.Itoa(mb) + "M"
}

// ParseSize reads "512", "512M", "1G" or "1.5G" as MiB.
func ParseSize(s string) (int, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, "B")
	unit := 1.0
	switch {
	case strings.HasSuffix(s, "G"):
		unit, s = 1024, strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		s = strings.TrimSuffix(s, "M")
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, ErrSwapSize
	}
	return int(v * unit), nil
}

// SuggestMB is the swap size suggested for memMB of memory: 1G below 1G
// of memory, 0 (no suggestion) otherwise.
func SuggestMB(memMB int) int {
	if memMB > 0 && memMB < 1024 {
		return 1024
	}
	return 0
}

// FstabWithSwap returns fstab with or without the line of SwapPath. Other
// lines are kept as they are.
func FstabWithSwap(data []byte, present bool) []byte {
	var b strings.Builder
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if line == "" {
			continue
		}
		if f := strings.Fields(line); len(f) > 0 && f[0] == SwapPath {
			continue
		}
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	if present {
		b.WriteString(SwapPath + " none swap sw 0 0\n")
	}
	return []byte(b.String())
}

// fstabHasSwap reports whether fstab mounts SwapPath.
func fstabHasSwap(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == SwapPath {
			return true
		}
	}
	return false
}

// Swap is the swap state shown in the menu.
type Swap struct {
	// Entries are all active swap areas.
	Entries []SwapEntry
	// Own is LiteNode's swap file when active.
	Own *SwapEntry
	// MemMB and MemAvailableMB come from /proc/meminfo.
	MemMB, MemAvailableMB int
}

// TotalMB is the size of all active swap.
func (s Swap) TotalMB() int {
	n := 0
	for _, e := range s.Entries {
		n += e.SizeMB
	}
	return n
}

// SwapStatus reads /proc/swaps and /proc/meminfo.
func (s *Service) SwapStatus() (Swap, error) {
	data, err := s.Runner.ReadFile("/proc/swaps")
	if err != nil {
		return Swap{}, fmt.Errorf("read /proc/swaps: %w", err)
	}
	sw := Swap{Entries: ParseSwaps(data)}
	for i := range sw.Entries {
		if sw.Entries[i].Path == SwapPath {
			sw.Own = &sw.Entries[i]
		}
	}
	if data, err := s.Runner.ReadFile("/proc/meminfo"); err == nil {
		sw.MemMB = meminfoMB(data, "MemTotal:")
		sw.MemAvailableMB = meminfoMB(data, "MemAvailable:")
	}
	return sw, nil
}

func meminfoMB(data []byte, key string) int {
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == key {
			kb, _ := strconv.Atoi(f[1])
			return kb / 1024
		}
	}
	return 0
}

// DiskAvailableMB is the free space of the file system holding SwapPath.
func (s *Service) DiskAvailableMB(ctx context.Context) (int, error) {
	res, err := s.Runner.Query(ctx, "df", "-P", "-B1M", "/")
	if err != nil {
		return 0, fmt.Errorf("df: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	if f := strings.Fields(lines[len(lines)-1]); len(f) >= 4 {
		if n, err := strconv.Atoi(f[3]); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("parse df output %q", res.Stdout)
}

// MaxCreateMB is the largest swap that fits on disk, given the space a
// replaced file frees (0 when creating).
func (s *Service) MaxCreateMB(ctx context.Context) (int, error) {
	avail, err := s.DiskAvailableMB(ctx)
	if err != nil {
		return 0, err
	}
	return min(avail-DiskReserveMB, MaxSwapMB), nil
}

func (s *Service) checkSize(ctx context.Context, mb int) error {
	if mb < MinSwapMB || mb > MaxSwapMB {
		return ErrSwapSize
	}
	max, err := s.MaxCreateMB(ctx)
	if err != nil {
		return err
	}
	if mb > max {
		return ErrNoDiskSpace
	}
	return nil
}

// makeSwapFile creates an initialized swap file of mb MiB at path. It
// falls back to dd when fallocate is not supported, and removes the file
// on failure.
func (s *Service) makeSwapFile(ctx context.Context, path string, mb int) (err error) {
	r := s.Runner
	defer func() {
		if err != nil {
			_ = r.Remove(path)
		}
	}()
	if _, err := r.Exec(ctx, "fallocate", "-l", strconv.Itoa(mb)+"M", path); err != nil {
		_ = r.Remove(path)
		if _, err := r.Exec(ctx, "dd", "if=/dev/zero", "of="+path, "bs=1M", "count="+strconv.Itoa(mb), "status=none"); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}
	if err := r.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if _, err := r.Exec(ctx, "mkswap", path); err != nil {
		return fmt.Errorf("mkswap %s: %w", path, err)
	}
	return nil
}

// checkSwapOff refuses to turn off own when memory cannot take its pages.
func checkSwapOff(sw Swap) error {
	if sw.Own != nil && sw.Own.UsedMB > 0 && sw.Own.UsedMB > sw.MemAvailableMB-memMarginMB {
		return ErrSwapInUse
	}
	return nil
}

// CreateSwap creates, enables and mounts the swap file at boot.
func (s *Service) CreateSwap(ctx context.Context, mb int) error {
	sw, err := s.SwapStatus()
	if err != nil {
		return err
	}
	fstab, err := s.Runner.ReadFile(FstabPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", FstabPath, err)
	}
	if sw.Own != nil || fstabHasSwap(fstab) {
		return ErrSwapExists
	}
	// A file left by an interrupted run is not in use: replace it.
	if err := s.Runner.Remove(SwapPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", SwapPath, err)
	}
	if err := s.checkSize(ctx, mb); err != nil {
		return err
	}
	if err := s.makeSwapFile(ctx, SwapPath, mb); err != nil {
		return err
	}
	return s.applyFstab(ctx, fstab, true)
}

// DeleteSwap turns off and removes the swap file and its fstab line.
func (s *Service) DeleteSwap(ctx context.Context) error {
	sw, err := s.SwapStatus()
	if err != nil {
		return err
	}
	fstab, err := s.Runner.ReadFile(FstabPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", FstabPath, err)
	}
	if sw.Own == nil && !fstabHasSwap(fstab) {
		return ErrNoSwap
	}
	if err := checkSwapOff(sw); err != nil {
		return err
	}
	return s.applyFstab(ctx, fstab, false)
}

// ResizeSwap replaces the swap file with one of mb MiB. The new file is
// made first, so a failure keeps the old one; fstab stays the same.
func (s *Service) ResizeSwap(ctx context.Context, mb int) error {
	sw, err := s.SwapStatus()
	if err != nil {
		return err
	}
	if sw.Own == nil {
		return ErrNoSwap
	}
	if err := checkSwapOff(sw); err != nil {
		return err
	}
	if err := s.checkSize(ctx, mb); err != nil {
		return err
	}
	r := s.Runner
	tmp := SwapPath + ".new"
	if err := s.makeSwapFile(ctx, tmp, mb); err != nil {
		return err
	}
	if _, err := r.Exec(ctx, "swapoff", SwapPath); err != nil {
		_ = r.Remove(tmp)
		return fmt.Errorf("swapoff %s: %w", SwapPath, err)
	}
	if err := r.Rename(tmp, SwapPath); err != nil {
		_ = r.Remove(tmp)
		if _, err2 := r.Exec(ctx, "swapon", SwapPath); err2 != nil {
			return errors.Join(fmt.Errorf("rename %s: %w", tmp, err), fmt.Errorf("swapon %s: %w", SwapPath, err2))
		}
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	if _, err := r.Exec(ctx, "swapon", SwapPath); err != nil {
		return fmt.Errorf("swapon %s: %w", SwapPath, err)
	}
	return nil
}

// applyFstab writes fstab with or without the swap line as a "swap"
// change; the kind's apply step turns the file on or off to match.
func (s *Service) applyFstab(ctx context.Context, fstab []byte, present bool) error {
	f := change.File{Path: FstabPath, Data: FstabWithSwap(fstab, present), Mode: 0o644}
	_, err := s.Changes.Apply(ctx, change.Change{Parts: []change.Part{{Kind: SwapKind(), Files: []change.File{f}}}})
	return err
}

// SwapKind is the "swap" configuration kind. Apply makes the swap file
// match /etc/fstab: on when listed, off and removed when not. After a
// restore this removes a file made by the failed change.
func SwapKind() *change.Kind {
	return &change.Kind{
		Name: "swap",
		Apply: func(ctx context.Context, r system.Runner) error {
			fstab, err := r.ReadFile(FstabPath)
			if err != nil {
				return fmt.Errorf("read %s: %w", FstabPath, err)
			}
			data, err := r.ReadFile("/proc/swaps")
			if err != nil {
				return fmt.Errorf("read /proc/swaps: %w", err)
			}
			active := false
			for _, e := range ParseSwaps(data) {
				active = active || e.Path == SwapPath
			}
			if fstabHasSwap(fstab) {
				if !active {
					if _, err := r.Exec(ctx, "swapon", SwapPath); err != nil {
						return fmt.Errorf("swapon %s: %w", SwapPath, err)
					}
				}
				return nil
			}
			if active {
				if _, err := r.Exec(ctx, "swapoff", SwapPath); err != nil {
					return fmt.Errorf("swapoff %s: %w", SwapPath, err)
				}
			}
			if err := r.Remove(SwapPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", SwapPath, err)
			}
			return nil
		},
	}
}
