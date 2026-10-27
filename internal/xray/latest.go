package xray

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// LatestPageURL is the web page that redirects to a repository's latest
// stable release. Unlike the GitHub API it is not rate limited per
// address, which matters behind shared exits such as WARP (D-130).
func LatestPageURL(repo string) string {
	return "https://github.com/" + repo + "/releases/latest"
}

// FeedURL is the Atom feed of a repository's releases, pre-releases
// included.
func FeedURL(repo string) string {
	return "https://github.com/" + repo + "/releases.atom"
}

// LatestTag returns the tag of repo's latest stable release from the
// redirect of its "releases/latest" page.
func LatestTag(ctx context.Context, f Fetcher, repo string) (string, error) {
	loc, err := f.Redirect(ctx, LatestPageURL(repo))
	if err != nil {
		return "", fmt.Errorf("query latest release of %s: %w", repo, err)
	}
	return ParseLatestLocation(loc)
}

// ParseLatestLocation extracts the tag from the redirect target of
// "releases/latest", ".../releases/tag/<tag>".
func ParseLatestLocation(loc string) (string, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("parse latest release location %q: %w", loc, err)
	}
	_, tag, ok := strings.Cut(u.Path, "/releases/tag/")
	if !ok || tag == "" || strings.Contains(tag, "/") {
		return "", fmt.Errorf("no release tag in location %q", loc)
	}
	if t, err := url.PathUnescape(tag); err == nil {
		tag = t
	}
	return NormalizeVersion(tag)
}

// ParseFeed lists the release tags in a GitHub releases Atom feed. Each
// entry links to ".../releases/tag/<tag>"; entries without a valid tag
// are skipped. Drafts never appear in the feed.
func ParseFeed(data []byte) ([]Release, error) {
	var feed struct {
		Entries []struct {
			Links []struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse releases feed: %w", err)
	}
	var out []Release
	for _, e := range feed.Entries {
		for _, l := range e.Links {
			if tag, err := ParseLatestLocation(l.Href); err == nil {
				out = append(out, Release{Tag: tag})
				break
			}
		}
	}
	return out, nil
}
