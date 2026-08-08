package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPullRequest_Merged(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/repos/o/r/pulls/7" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"number":7,"state":"closed","merged":true,"merged_at":"2024-01-02T03:04:05Z","html_url":"https://github.com/o/r/pull/7"}`))
	}))
	defer srv.Close()

	c := New(Config{Token: "tok", BaseURL: srv.URL})
	pr, err := c.GetPullRequest(context.Background(), "o", "r", 7)
	if err != nil {
		t.Fatalf("get pr: %v", err)
	}
	if !pr.Merged || pr.MergedAt == nil {
		t.Errorf("expected merged pr, got %+v", pr)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header = %q", gotAuth)
	}
}

func TestGetPullRequest_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	if _, err := c.GetPullRequest(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestParsePullRequestURL(t *testing.T) {
	owner, repo, number, err := ParsePullRequestURL("https://github.com/org/repo/pull/42")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if owner != "org" || repo != "repo" || number != 42 {
		t.Errorf("got %s/%s#%d", owner, repo, number)
	}
	if _, _, _, err := ParsePullRequestURL("https://example.com/x"); err == nil {
		t.Fatal("expected error for non-PR url")
	}
}
