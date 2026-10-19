package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MinMLKEMVersion is the first Xray whose REALITY server works with
// targets that negotiate X25519MLKEM768 — nowadays nearly every large
// site. Found by bisecting releases on the test VPS (D-054): v26.7.28 and
// earlier, including stable v26.3.27, fail the handshake.
const MinMLKEMVersion = "v26.9.8"

// CompareVersions compares "vX.Y.Z" versions numerically, ignoring any
// suffix after "-". Unparsable parts count as 0.
func CompareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := range 3 {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	v, _, _ = strings.Cut(v, "-")
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// MinVLESSEncVersion is the first stable Xray with VLESS Encryption
// (D-108).
const MinVLESSEncVersion = "v25.9.5"

// SupportsVLESSEnc reports whether Xray version v supports VLESS
// Encryption nodes.
func SupportsVLESSEnc(v string) bool {
	return v != "" && CompareVersions(v, MinVLESSEncVersion) >= 0
}

// SupportsMLKEMTargets reports whether Xray version v can use REALITY
// targets that pick X25519MLKEM768.
func SupportsMLKEMTargets(v string) bool {
	return v != "" && CompareVersions(v, MinMLKEMVersion) >= 0
}

// ReleasesURL lists recent releases, newest first (pre-releases included).
const ReleasesURL = "https://api.github.com/repos/" + Repo + "/releases?per_page=30"

// PickDefault chooses the version to install by default (D-054): the
// latest stable release if it is at least MinMLKEMVersion, otherwise the
// newest release (possibly a pre-release) that is. If none qualifies, the
// latest stable.
func PickDefault(latestStable string, releases []Release) string {
	if SupportsMLKEMTargets(latestStable) {
		return latestStable
	}
	best := ""
	for _, r := range releases {
		if r.Draft || !versionRE.MatchString(r.Tag) || !SupportsMLKEMTargets(r.Tag) {
			continue
		}
		if best == "" || CompareVersions(r.Tag, best) > 0 {
			best = r.Tag
		}
	}
	if best == "" {
		return latestStable
	}
	return best
}

// Release is one entry of the GitHub releases list.
type Release struct {
	Tag        string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// ParseReleases parses the GitHub releases list.
func ParseReleases(data []byte) ([]Release, error) {
	var rs []Release
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("parse releases: %w", err)
	}
	return rs, nil
}

// DefaultVersion is the version installed or updated to when the user
// does not name one (D-054).
func (in *Installer) DefaultVersion(ctx context.Context) (string, error) {
	stable, err := in.Latest(ctx)
	if err != nil {
		return "", err
	}
	if SupportsMLKEMTargets(stable) {
		return stable, nil
	}
	data, err := in.Fetcher.Fetch(ctx, ReleasesURL, 32<<20)
	if err != nil {
		return stable, nil // fall back to stable rather than fail
	}
	rs, err := ParseReleases(data)
	if err != nil {
		return stable, nil
	}
	return PickDefault(stable, rs), nil
}
