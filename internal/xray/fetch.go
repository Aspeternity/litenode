package xray

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
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
// networks. dial replaces how connections are made (nil: directly).
func NewHTTPFetcher(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *HTTPFetcher {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if dial != nil {
		tr.DialContext = dial
	}
	return &HTTPFetcher{Client: &http.Client{Timeout: 10 * time.Minute, Transport: tr}}
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
	// Allocating the known size at once matters on small machines: growing
	// the buffer step by step leaves garbage several times the file size.
	var buf bytes.Buffer
	if n := resp.ContentLength; n > 0 && n <= limit {
		buf.Grow(int(n) + bytes.MinRead)
	}
	if _, err := buf.ReadFrom(io.LimitReader(resp.Body, limit+1)); err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if int64(buf.Len()) > limit {
		return nil, fmt.Errorf("download %s: larger than %d bytes", url, limit)
	}
	return buf.Bytes(), nil
}
