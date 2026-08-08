//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/response"
)

// client is a thin HTTP helper for talking to the test server. It knows how to
// attach a bearer token, marshal JSON bodies, and decode the standard response
// envelope so tests stay focused on the journey rather than plumbing.
type client struct {
	t     *testing.T
	token string
	http  *http.Client
}

// newClient returns an unauthenticated client.
func newClient(t *testing.T) *client {
	t.Helper()
	return &client{t: t, http: &http.Client{Timeout: 10 * time.Second}}
}

// authed returns a copy of the client that sends the given bearer token.
func (c *client) authed(token string) *client {
	return &client{t: c.t, token: token, http: c.http}
}

// apiResult captures everything a test might want to assert on.
type apiResult struct {
	Status   int
	Envelope response.Envelope
	Body     []byte
}

// requireStatus fails the test unless the response carried the expected status.
func (r apiResult) requireStatus(t *testing.T, want int) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, r.Status, string(r.Body))
	}
}

// decode unmarshals the envelope's `data` field into v.
func (r apiResult) decode(t *testing.T, v any) {
	t.Helper()
	raw, err := json.Marshal(r.Envelope.Data)
	if err != nil {
		t.Fatalf("re-marshal envelope data: %v", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode envelope data: %v", err)
	}
}

func (c *client) get(path string) apiResult            { return c.do(http.MethodGet, path, nil) }
func (c *client) post(path string, body any) apiResult { return c.do(http.MethodPost, path, body) }
func (c *client) put(path string, body any) apiResult  { return c.do(http.MethodPut, path, body) }
func (c *client) delete(path string) apiResult         { return c.do(http.MethodDelete, path, nil) }

func (c *client) do(method, path string, body any) apiResult {
	c.t.Helper()

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		c.t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read response body: %v", err)
	}

	result := apiResult{Status: resp.StatusCode, Body: raw}
	if len(raw) > 0 {
		// 204 (and similar) carry no body; ignore decode errors there.
		_ = json.Unmarshal(raw, &result.Envelope)
	}
	return result
}
