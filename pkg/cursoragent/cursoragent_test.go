package cursoragent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLaunch_BuildsRequestAndParsesResponse(t *testing.T) {
	var gotAuth string
	var gotReq launchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/agents" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		_, _ = w.Write([]byte(`{"id":"bc-1","status":"PENDING","target":{"branchName":"luna/x"}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "k", BaseURL: srv.URL})
	ag, err := c.Launch(context.Background(), LaunchInput{
		Prompt:       "do work",
		Repository:   "https://github.com/o/r",
		Ref:          "main",
		AutoCreatePR: true,
		BranchName:   "luna/x",
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if ag.ID != "bc-1" || ag.Status != StatusPending {
		t.Errorf("unexpected agent %+v", ag)
	}
	if gotAuth != "Bearer k" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotReq.Prompt.Text != "do work" || gotReq.Source.Repository != "https://github.com/o/r" {
		t.Errorf("unexpected request %+v", gotReq)
	}
	if gotReq.Target == nil || !gotReq.Target.AutoCreatePR {
		t.Errorf("expected autoCreatePr target, got %+v", gotReq.Target)
	}
}

func TestGet_ReturnsStatusAndPR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/agents/bc-1" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"bc-1","status":"FINISHED","target":{"prUrl":"https://github.com/o/r/pull/7"}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "k", BaseURL: srv.URL})
	ag, err := c.Get(context.Background(), "bc-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !IsTerminal(ag.Status) {
		t.Errorf("expected terminal status, got %q", ag.Status)
	}
	if ag.Target.PRURL != "https://github.com/o/r/pull/7" {
		t.Errorf("pr url = %q", ag.Target.PRURL)
	}
}

func TestResultText_ConcatenatesAssistant(t *testing.T) {
	// The live API uses "user_message"/"assistant_message"; older payloads used
	// "user"/"assistant" or empty types. ResultText must keep assistant output
	// from all of these while always dropping user-authored messages.
	cases := []struct {
		name     string
		messages string
		want     string
	}{
		{
			name:     "api message types",
			messages: `[{"type":"user_message","text":"go"},{"type":"assistant_message","text":"part1 "},{"type":"assistant_message","text":"part2"}]`,
			want:     "part1 \npart2\n",
		},
		{
			name:     "legacy and empty types",
			messages: `[{"type":"user","text":"go"},{"type":"assistant","text":"part1 "},{"type":"","text":"part2"}]`,
			want:     "part1 \npart2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v0/agents/bc-1/conversation" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"messages":` + tc.messages + `}`))
			}))
			defer srv.Close()

			c := New(Config{APIKey: "k", BaseURL: srv.URL})
			text, err := c.ResultText(context.Background(), "bc-1")
			if err != nil {
				t.Fatalf("result text: %v", err)
			}
			if text != tc.want {
				t.Errorf("text = %q, want %q", text, tc.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	for _, s := range []string{StatusFinished, StatusError, StatusExpired} {
		if !IsTerminal(s) {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range []string{StatusCreating, StatusPending, StatusRunning} {
		if IsTerminal(s) {
			t.Errorf("%q should not be terminal", s)
		}
	}
}
