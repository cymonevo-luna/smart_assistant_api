package composio

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitHub tool slugs as exposed by the Composio GitHub toolkit. They are declared
// as variables (not constants) so a deployment using different slugs can
// override them at startup without forking this package.
var (
	ToolGetPullRequest    = "GITHUB_GET_A_PULL_REQUEST"
	ToolCreatePullRequest = "GITHUB_CREATE_A_PULL_REQUEST"
)

// GitHub wraps a Client with typed helpers for the GitHub operations an
// orchestrator needs: open a pull request and read its state to detect merges.
// Access is brokered through Composio, so the PR is authored by the GitHub
// account connected in Composio and no GitHub credentials are handled here.
type GitHub struct {
	c *Client
}

// NewGitHub returns a GitHub helper bound to c.
func NewGitHub(c *Client) *GitHub { return &GitHub{c: c} }

// PullRequest is the subset of PR state the orchestrator needs.
type PullRequest struct {
	Number   int        `json:"number"`
	State    string     `json:"state"`
	Merged   bool       `json:"merged"`
	MergedAt *time.Time `json:"merged_at"`
	HTMLURL  string     `json:"html_url"`
	Title    string     `json:"title"`
}

// CreatePullRequest opens a pull request via Composio, so it is authored by the
// GitHub account connected in Composio rather than by any ambient credential.
// It opens a PR in owner/repo from the head branch into base, with the given
// title and body, and returns the created pull request.
func (g *GitHub) CreatePullRequest(ctx context.Context, owner, repo, head, base, title, body string) (*PullRequest, error) {
	res, err := g.c.Execute(ctx, ToolCreatePullRequest, map[string]any{
		"owner": owner,
		"repo":  repo,
		"head":  head,
		"base":  base,
		"title": title,
		"body":  body,
	})
	if err != nil {
		return nil, err
	}
	pr, err := decodeData[PullRequest](res.Data)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

// GetPullRequest fetches a single pull request via Composio.
func (g *GitHub) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	res, err := g.c.Execute(ctx, ToolGetPullRequest, map[string]any{
		"owner":       owner,
		"repo":        repo,
		"pull_number": number,
	})
	if err != nil {
		return nil, err
	}
	pr, err := decodeData[PullRequest](res.Data)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

var prURLRe = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// ParsePullRequestURL extracts owner, repo and number from a PR HTML URL such as
// https://github.com/org/repo/pull/42.
func ParsePullRequestURL(url string) (owner, repo string, number int, err error) {
	m := prURLRe.FindStringSubmatch(url)
	if m == nil {
		return "", "", 0, fmt.Errorf("composio: not a pull request url: %q", url)
	}
	number, err = strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("composio: bad pr number in %q: %w", url, err)
	}
	return m[1], m[2], number, nil
}

var repoURLRe = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/?#]+)`)

// ParseRepoSlug extracts an "owner/repo" slug from a GitHub repository URL such
// as https://github.com/owner/repo. A trailing ".git" and any sub-path are
// ignored, and SSH URLs (git@github.com:owner/repo.git) are also supported.
func ParseRepoSlug(gitURL string) (string, error) {
	m := repoURLRe.FindStringSubmatch(strings.TrimSpace(gitURL))
	if m == nil {
		return "", fmt.Errorf("composio: not a github repository url: %q", gitURL)
	}
	return m[1] + "/" + strings.TrimSuffix(m[2], ".git"), nil
}
