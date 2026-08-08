package composio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHub_GetPullRequest_Direct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tools/execute/"+ToolGetPullRequest {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"successful":true,"data":{"number":42,"state":"closed","merged":true,"html_url":"https://github.com/org/repo/pull/42","title":"Add feature"}}`))
	}))
	defer srv.Close()

	gh := NewGitHub(New(Config{APIKey: "key", BaseURL: srv.URL}))
	pr, err := gh.GetPullRequest(context.Background(), "org", "repo", 42)
	if err != nil {
		t.Fatalf("get pr: %v", err)
	}
	if pr.Number != 42 || !pr.Merged || pr.State != "closed" {
		t.Errorf("unexpected pr: %+v", pr)
	}
}

func TestGitHub_GetPullRequest_Envelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"successful":true,"data":{"response_data":{"number":7,"merged":false}}}`))
	}))
	defer srv.Close()

	gh := NewGitHub(New(Config{APIKey: "key", BaseURL: srv.URL}))
	pr, err := gh.GetPullRequest(context.Background(), "org", "repo", 7)
	if err != nil {
		t.Fatalf("get pr: %v", err)
	}
	if pr.Number != 7 || pr.Merged {
		t.Errorf("unexpected pr: %+v", pr)
	}
}

func TestGitHub_CreatePullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tools/execute/"+ToolCreatePullRequest {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"successful":true,"data":{"number":12,"state":"open","html_url":"https://github.com/org/repo/pull/12","title":"Add feature"}}`))
	}))
	defer srv.Close()

	gh := NewGitHub(New(Config{APIKey: "key", BaseURL: srv.URL}))
	pr, err := gh.CreatePullRequest(context.Background(), "org", "repo", "feature", "main", "Add feature", "body")
	if err != nil {
		t.Fatalf("create pr: %v", err)
	}
	if pr.Number != 12 || pr.HTMLURL != "https://github.com/org/repo/pull/12" || pr.State != "open" {
		t.Errorf("unexpected pr: %+v", pr)
	}
}

func TestParsePullRequestURL(t *testing.T) {
	owner, repo, number, err := ParsePullRequestURL("https://github.com/acme/widgets/pull/123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if owner != "acme" || repo != "widgets" || number != 123 {
		t.Errorf("unexpected parse: %s/%s#%d", owner, repo, number)
	}

	if _, _, _, err := ParsePullRequestURL("https://example.com/not/a/pr"); err == nil {
		t.Error("expected error for non-PR url")
	}
}

func TestParseRepoSlug(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/widgets":             "acme/widgets",
		"https://github.com/acme/widgets.git":         "acme/widgets",
		"https://github.com/acme/widgets/":            "acme/widgets",
		"https://github.com/acme-org/wid_gets/tree/x": "acme-org/wid_gets",
		"git@github.com:acme/widgets.git":             "acme/widgets",
	}
	for in, want := range cases {
		got, err := ParseRepoSlug(in)
		if err != nil {
			t.Errorf("ParseRepoSlug(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseRepoSlug(%q) = %q, want %q", in, got, want)
		}
	}

	if _, err := ParseRepoSlug("https://gitlab.com/acme/widgets"); err == nil {
		t.Error("expected error for non-github url")
	}
}

func TestParseBoardID(t *testing.T) {
	cases := map[string]string{
		"https://trello.com/b/abc12345/my-board": "abc12345",
		"https://trello.com/b/abc12345":          "abc12345",
		"abc12345":                               "abc12345",
		"6a30b3a705ee44d41d0413bc":               "6a30b3a705ee44d41d0413bc",
	}
	for in, want := range cases {
		got, err := ParseBoardID(in)
		if err != nil {
			t.Errorf("ParseBoardID(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseBoardID(%q) = %q, want %q", in, got, want)
		}
	}

	if _, err := ParseBoardID("not a board id!"); err == nil {
		t.Error("expected error for invalid board ref")
	}
}
