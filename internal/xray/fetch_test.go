package xray

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

var errDial = errors.New("no route")

// TestFetcherKeepsDialError: the reason a connection failed survives the
// HTTP client, so the menu can explain a missing IPv4 exit (DESIGN 19.3).
func TestFetcherKeepsDialError(t *testing.T) {
	var dialed []string
	f := NewHTTPFetcher(func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = append(dialed, addr)
		return nil, fmt.Errorf("dial %s: %w", addr, errDial)
	})
	if _, err := f.Fetch(t.Context(), "https://github.com/x", 10); !errors.Is(err, errDial) {
		t.Errorf("Fetch error = %v", err)
	}
	if _, err := f.Redirect(t.Context(), "https://github.com/x/releases/latest"); !errors.Is(err, errDial) {
		t.Errorf("Redirect error = %v", err)
	}
	if len(dialed) != 2 || dialed[0] != "github.com:443" {
		t.Errorf("dialed %v", dialed)
	}
}

func TestFetchBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chunked" {
			w.(http.Flusher).Flush() // no Content-Length
		}
		w.Write(bytes.Repeat([]byte("x"), 1000))
	}))
	defer srv.Close()
	f := NewHTTPFetcher(nil)
	for _, path := range []string{"/known", "/chunked"} {
		data, err := f.Fetch(t.Context(), srv.URL+path, 1000)
		if err != nil || len(data) != 1000 {
			t.Errorf("%s: got %d bytes, err %v", path, len(data), err)
		}
		if _, err := f.Fetch(t.Context(), srv.URL+path, 999); err == nil {
			t.Errorf("%s: no error over the limit", path)
		}
	}
}
