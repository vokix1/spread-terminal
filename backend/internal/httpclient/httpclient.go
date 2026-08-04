// Package httpclient provides a shared, hardened HTTP client for exchange connectors.
// It handles:
//   - Redirects (including HTTP→HTTPS upgrades)
//   - Compressed responses (gzip)
//   - Realistic browser-like headers to avoid bot detection
//   - Per-request context with cancellation
//   - Automatic retry on EOF / connection-reset (transient errors)
package httpclient

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Default is a shared client suitable for all exchange public REST APIs.
var Default = New(20 * time.Second)

// New creates a new hardened HTTP client with the given per-request timeout.
func New(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    false, // allow gzip
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		// Follow redirects, including HTTP→HTTPS upgrade
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// GetJSON performs a GET request with retry on transient errors and decodes
// the JSON response into dst. It adds standard headers that most exchanges expect.
func GetJSON(ctx context.Context, client *http.Client, url string, dst interface{}) error {
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// brief wait between retries
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 800 * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		// Headers that make us look like a real browser/trading tool
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Encoding", "gzip, deflate")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SpreadTerminal/1.0)")
		req.Header.Set("Cache-Control", "no-cache")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if isTransient(err) {
				continue
			}
			return err
		}

		body, err := readBody(resp)
		if err != nil {
			_ = resp.Body.Close()
			lastErr = err
			if isTransient(err) {
				continue
			}
			return err
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// 429 or 5xx: retry
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
				continue
			}
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
		}

		if err := json.Unmarshal(body, dst); err != nil {
			return fmt.Errorf("json decode %s: %w", url, err)
		}
		return nil
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	}
	return io.ReadAll(io.LimitReader(reader, 32<<20)) // max 32MB
}

// isTransient returns true for errors that are safe to retry.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, msg := range []string{"EOF", "connection reset", "broken pipe", "i/o timeout", "no such host"} {
		if len(s) > 0 {
			for i := 0; i <= len(s)-len(msg); i++ {
				if s[i:i+len(msg)] == msg {
					return true
				}
			}
		}
	}
	return false
}
