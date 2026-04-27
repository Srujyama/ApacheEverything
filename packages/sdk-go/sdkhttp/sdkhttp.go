// Package sdkhttp is a thin HTTP client connectors share for fetching from
// public APIs. It handles the parts every connector needs and none should
// reimplement: a sane User-Agent, a timeout, and exponential backoff on
// 429 / 5xx responses. Anything fancier (auth, paging, streaming) is the
// connector's responsibility.
package sdkhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// UserAgent identifies Sunny in upstream API logs. Some upstream APIs
// (notably api.weather.gov) require a non-default User-Agent.
const UserAgent = "Sunny/0.1 (+https://github.com/sunny/sunny)"

// Client is a small wrapper around http.Client with retry behavior tuned for
// public open-data APIs.
type Client struct {
	HTTP       *http.Client
	MaxRetries int           // total attempts = MaxRetries + 1
	BaseDelay  time.Duration // initial backoff
	MaxDelay   time.Duration // cap on backoff between attempts
}

// New returns a Client with sensible defaults: 30s timeout, 4 retries,
// 500ms→8s exponential backoff.
func New() *Client {
	return &Client{
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 4,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   8 * time.Second,
	}
}

// GetJSON fetches url and returns the response body. The caller decodes —
// keeping json.Decoder out of here lets connectors use jsoniter or stream
// large responses if they need to.
func (c *Client) GetJSON(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, url, headers)
}

// Get is the same as GetJSON but doesn't set Accept: application/json. Use
// for CSV or text endpoints.
func (c *Client) Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	if _, ok := headers["Accept"]; !ok {
		headers["Accept"] = "*/*"
	}
	return c.do(ctx, http.MethodGet, url, headers)
}

func (c *Client) do(ctx context.Context, method, url string, headers map[string]string) ([]byte, error) {
	delay := c.BaseDelay
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			if delay < c.MaxDelay {
				delay *= 2
				if delay > c.MaxDelay {
					delay = c.MaxDelay
				}
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		if _, ok := headers["Accept"]; !ok {
			req.Header.Set("Accept", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, snippet(body))
			continue
		default:
			// 4xx other than 429 — not retryable.
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, snippet(body))
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no response")
	}
	return nil, fmt.Errorf("after %d attempts: %w", c.MaxRetries+1, lastErr)
}

func snippet(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}
