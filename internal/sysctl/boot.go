package sysctl

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/system"
)

// BootDirs are the directories systemd-sysctl reads at boot, highest
// priority first: a file shadows those of the same name in later
// directories. /etc/sysctl.conf is read through the
// /etc/sysctl.d/99-sysctl.conf link that Debian and Ubuntu ship.
var BootDirs = []string{"/etc/sysctl.d", "/run/sysctl.d", "/usr/local/lib/sysctl.d", "/usr/lib/sysctl.d", "/lib/sysctl.d"}

// BootValue returns the value the sysctl files give key at boot and the
// file that sets it last, like systemd-sysctl: the *.conf files of
// BootDirs, deduplicated by name and read in name order. ok is false when
// no file sets key.
func BootValue(r system.Runner, key string) (value, file string, ok bool, err error) {
	paths := map[string]string{}
	for _, dir := range BootDirs {
		names, err := r.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return "", "", false, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, name := range names {
			if _, seen := paths[name]; !seen && strings.HasSuffix(name, ".conf") {
				paths[name] = dir + "/" + name
			}
		}
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		path := paths[name]
		data, err := r.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			// A dangling link.
			continue
		} else if err != nil {
			return "", "", false, fmt.Errorf("read %s: %w", path, err)
		}
		if v, found := lookup(data, key); found {
			value, file, ok = v, path, true
		}
	}
	return value, file, ok, nil
}

// lookup returns the last value data gives key. Keys may use slashes
// and a leading "-" (ignore errors); "#" and ";" start comments.
func lookup(data []byte, key string) (value string, ok bool) {
	key = strings.ReplaceAll(key, "/", ".")
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		k = strings.TrimPrefix(strings.TrimSpace(k), "-")
		if strings.ReplaceAll(k, "/", ".") == key {
			value, ok = strings.TrimSpace(v), true
		}
	}
	return value, ok
}
