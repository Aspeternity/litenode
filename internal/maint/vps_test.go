//go:build vps

// This test downloads the latest LiteNode release and runs the updater's
// failure paths against the real /usr/local/bin/litenode, so it is
// excluded from normal builds. Run it as root on the test VPS:
//
//	CGO_ENABLED=0 go test -c -tags vps -o maint.test ./internal/maint
//	./maint.test -test.v -test.run TestVPS
package maint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/xray"
)

// tamper wraps the real fetcher and changes the binary; with sums set it
// also publishes a matching sha256, so only the version check can fail.
type tamper struct {
	real xray.Fetcher
	sums bool
}

func (t tamper) Fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	name, _ := AssetName(runtime.GOARCH)
	bad := []byte("#!/bin/sh\necho not litenode\n")
	switch {
	case strings.HasSuffix(url, "/"+name):
		return bad, nil
	case t.sums && strings.HasSuffix(url, "/sha256sums.txt"):
		sum := sha256.Sum256(bad)
		return []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n"), nil
	}
	return t.real.Fetch(ctx, url, limit)
}

func (t tamper) Redirect(ctx context.Context, url string) (string, error) {
	return t.real.Redirect(ctx, url)
}

func TestVPSUpdateChecks(t *testing.T) {
	ctx := context.Background()
	before, err := os.ReadFile(BinPath)
	if err != nil {
		t.Fatal(err)
	}
	bakBefore, bakErr := os.ReadFile(BakPath)
	r := system.NewReal(nil)
	real := xray.NewHTTPFetcher()
	tag, err := (&Updater{Runner: r, Fetcher: real, Arch: runtime.GOARCH}).Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("latest release %s", tag)
	for name, c := range map[string]struct {
		f    xray.Fetcher
		want error
	}{
		"tampered download": {tamper{real: real}, ErrDigestMismatch},
		"does not run":      {tamper{real: real, sums: true}, ErrBadBinary},
	} {
		err := (&Updater{Runner: r, Fetcher: c.f, Arch: runtime.GOARCH}).Update(ctx, tag)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
		after, _ := os.ReadFile(BinPath)
		bakAfter, bakAfterErr := os.ReadFile(BakPath)
		if !bytes.Equal(before, after) || !bytes.Equal(bakBefore, bakAfter) || (bakErr == nil) != (bakAfterErr == nil) {
			t.Errorf("%s: program or backup changed", name)
		}
		if _, err := os.Stat(newPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: %s left behind", name, newPath)
		}
		t.Logf("%s: %v (nothing replaced)", name, err)
	}
}
