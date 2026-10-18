package reality

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// riskyKeywords mark domains whose sites invite traffic theft through the
// REALITY fallback (DESIGN section 10.4). "dl" only matches a whole label
// part, the others match anywhere.
var riskyKeywords = []string{"cdn", "download", "dl", "mirror", "video", "stream", "speedtest", "update"}

// RiskyKeyword returns the first high-risk keyword in host, or "".
func RiskyKeyword(host string) string {
	parts := strings.FieldsFunc(strings.ToLower(host), func(r rune) bool { return r == '.' || r == '-' })
	for _, kw := range riskyKeywords {
		for _, p := range parts {
			if kw == "dl" {
				if p == kw {
					return kw
				}
			} else if strings.Contains(p, kw) {
				return kw
			}
		}
	}
	return ""
}

// CommonTargets is the built-in list offered when the user does not scan
// (DESIGN section 10.1, D-059). Each passed the check and worked end to end
// as a REALITY target from the test VPSes (2026-10-18). LiteNode mainly
// serves landing servers reached through a relay, so the list is chosen by
// protocol checks on the VPS, not by direct reachability from one network.
// Each is still checked from the VPS before it can be used.
var CommonTargets = []string{
	"www.microsoft.com",
	"learn.microsoft.com",
	"www.apple.com",
	"www.icloud.com",
	"itunes.apple.com",
	"gateway.icloud.com",
	"www.amazon.com",
	"www.samsung.com",
	"www.nvidia.com",
	"www.tesla.com",
	"addons.mozilla.org",
	"www.yahoo.com",
	"www.bing.com",
	"www.xbox.com",
}

// CheckAll checks targets concurrently (at most limit at a time) and
// returns the results usable first, then reachable, each by latency.
func (c *Checker) CheckAll(ctx context.Context, targets []string, limit int) []Result {
	if limit < 1 {
		limit = 1
	}
	results := make([]Result, len(targets))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = c.Check(ctx, t)
		}()
	}
	wg.Wait()
	rank := func(r Result) int {
		switch {
		case r.Usable():
			return 0
		case r.Err == nil:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(results, func(a, b Result) int {
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra - rb
		}
		return int(a.Latency - b.Latency)
	})
	return results
}
