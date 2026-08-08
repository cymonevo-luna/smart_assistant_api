// Package github is a small, dependency-free client for the parts of the GitHub
// REST API an orchestrator needs: checking whether a pull request has merged.
// It authenticates with a personal access token or GitHub App installation token.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// DefaultBaseURL is the public GitHub REST API base.
const DefaultBaseURL = "https://api.github.com"

// Config configures a Client.
type Config struct {
	Token      string
	BaseURL    string
	HTTPClient *http.Client
}

// Client talks to the GitHub REST API.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New builds a Client from cfg.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{token: cfg.Token, baseURL: base, http: hc}
}

// PullRequest is the subset of PR state the orchestrator needs.
type PullRequest struct {
	Number   int        `json:"number"`
	State    string     `json:"state"`
	Merged   bool       `json:"merged"`
	MergedAt *time.Time `json:"merged_at"`
	HTMLURL  string     `json:"html_url"`
	Title    string     `json:"title"`
}

// GetPullRequest fetches a single pull request.
func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("github: new request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: get pr: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github: get pr %s/%s#%d: status %d: %s", owner, repo, number, resp.StatusCode, string(raw))
	}

	var pr PullRequest
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, fmt.Errorf("github: decode pr: %w", err)
	}
	return &pr, nil
}

var prURLRe = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// ParsePullRequestURL extracts owner, repo and number from a PR HTML URL such as
// https://github.com/org/repo/pull/42.
func ParsePullRequestURL(url string) (owner, repo string, number int, err error) {
	m := prURLRe.FindStringSubmatch(url)
	if m == nil {
		return "", "", 0, fmt.Errorf("github: not a pull request url: %q", url)
	}
	number, err = strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("github: bad pr number in %q: %w", url, err)
	}
	return m[1], m[2], number, nil
}
