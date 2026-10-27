package xray

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher downloads a URL into memory. Downloads only read from the
// network, so they also run in dry-run mode.
type Fetcher interface {
	Fetch(ctx context.Context, url string, limit int64) ([]byte, error)
	// Redirect requests url without following a redirect and returns
	// the Location it redirects to.
	Redirect(ctx context.Context, url string) (string, error)
}

// HTTPFetcher is the real Fetcher.
type HTTPFetcher struct {
	Client *http.Client
}

// NewHTTPFetcher returns a Fetcher with a generous timeout for slow VPS
// networks.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{Client: &http.Client{Timeout: 10 * time.Minute}}
}

func (f *HTTPFetcher) Redirect(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "litenode")
	c := *f.Client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", url, err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
		return "", fmt.Errorf("request %s: HTTP %s, no redirect", url, resp.Status)
	}
	return loc, nil
}

func (f *HTTPFetcher) Fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "litenode")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download %s: larger than %d bytes", url, limit)
	}
	return data, nil
}
