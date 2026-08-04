// Package httpclient provides a shared, hardened HTTP client for exchange connectors.
package httpclient

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var Default = New(30 * time.Second)

func New(timeout time.Duration) *http.Client {
	if timeout < 20*time.Second {
		timeout = 20 * time.Second
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func GetJSON(ctx context.Context, client *http.Client, url string, dst interface{}) error {
	return doJSON(ctx, client, http.MethodGet, url, nil, dst)
}

func PostJSON(ctx context.Context, client *http.Client, url string, body io.Reader, dst interface{}) error {
	return doJSON(ctx, client, http.MethodPost, url, body, dst)
}

func doJSON(ctx context.Context, client *http.Client, method, url string, body io.Reader, dst interface{}) error {
	const maxRetries = 4
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		var requestBody io.Reader
		if body != nil {
			if seeker, ok := body.(io.Seeker); ok {
				_, _ = seeker.Seek(0, io.SeekStart)
				requestBody = seeker
			} else if attempt == 0 {
				requestBody = body
			} else {
				return fmt.Errorf("cannot retry non-seekable request body")
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, requestBody)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("User-Agent", "SpreadTerminal/1.0")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if isTransient(err) {
				continue
			}
			return err
		}

		data, readErr := readBody(resp)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if isTransient(readErr) {
				continue
			}
			return readErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("HTTP %d: %s: %s", resp.StatusCode, url, truncate(data, 256))
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-time.After(time.Duration(seconds) * time.Second):
						}
					}
				}
				continue
			}
			return lastErr
		}

		if err := json.Unmarshal(data, dst); err != nil {
			return fmt.Errorf("json decode %s: %w", url, err)
		}
		return nil
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	}
	return io.ReadAll(io.LimitReader(reader, 32<<20))
}

func isTransient(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"eof", "connection reset", "broken pipe", "tls handshake timeout", "timeout awaiting response headers", "no such host", "server misbehaving"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func truncate(data []byte, max int) string {
	if len(data) <= max {
		return strings.TrimSpace(string(data))
	}
	return strings.TrimSpace(string(data[:max])) + "…"
}
