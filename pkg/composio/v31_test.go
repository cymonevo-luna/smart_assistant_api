package composio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateAPIKey_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3.1/connected_accounts" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","status":401}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "bad-key", BaseURL: srv.URL})
	err := c.ValidateAPIKey(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should include status 401, got: %v", err)
	}
}

func TestListConnectedAccounts_ParsesMockResponse(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3.1/connected_accounts" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want test-key", got)
		}
		if !strings.Contains(r.URL.RawQuery, "statuses=ACTIVE") {
			t.Errorf("expected statuses=ACTIVE in query, got %q", r.URL.RawQuery)
		}
		page++
		if page == 1 {
			_, _ = w.Write([]byte(`{
				"items": [
					{"id":"ca_github","status":"ACTIVE","toolkit":{"slug":"github"},"alias":"work"},
					{"id":"ca_gmail","status":"ACTIVE","toolkit":{"slug":"gmail"},"word_id":"w1"}
				],
				"next_cursor": null
			}`))
			return
		}
		t.Fatal("unexpected pagination request")
	}))
	defer srv.Close()

	c := New(Config{APIKey: "test-key", BaseURL: srv.URL})
	accounts, err := c.ListConnectedAccounts(context.Background(), ListConnectedAccountsOpts{
		Statuses: []string{"ACTIVE"},
	})
	if err != nil {
		t.Fatalf("ListConnectedAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].ToolkitSlug != "github" || accounts[0].ID != "ca_github" || accounts[0].Alias != "work" {
		t.Errorf("github account: %+v", accounts[0])
	}
	if accounts[1].ToolkitSlug != "gmail" || accounts[1].ID != "ca_gmail" || accounts[1].WordID != "w1" {
		t.Errorf("gmail account: %+v", accounts[1])
	}
}

func TestListConnectedAccounts_Paginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{
				"items": [{"id":"ca1","status":"ACTIVE","toolkit":{"slug":"slack"}}],
				"next_cursor": "page2"
			}`))
			return
		}
		if r.URL.Query().Get("cursor") != "page2" {
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
		_, _ = w.Write([]byte(`{
			"items": [{"id":"ca2","status":"ACTIVE","toolkit":{"slug":"notion"}}],
			"next_cursor": null
		}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	accounts, err := c.ListConnectedAccounts(context.Background(), ListConnectedAccountsOpts{
		Statuses: []string{"ACTIVE"},
	})
	if err != nil {
		t.Fatalf("ListConnectedAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts across pages, got %d", len(accounts))
	}
	if accounts[0].ToolkitSlug != "slack" || accounts[1].ToolkitSlug != "notion" {
		t.Errorf("unexpected accounts: %+v", accounts)
	}
}

func TestCreateSession_ReturnsSessionID(t *testing.T) {
	var gotAPIKey string
	var gotBody sessionCreateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		if r.URL.Path != "/api/v3.1/tool_router/session" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"session_id": "trs_test123",
			"mcp": {"url": "https://mcp.example.com", "headers": {"Authorization": "Bearer tok"}}
		}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "user-key", BaseURL: srv.URL})
	opts := SessionCreateOpts{}
	opts.ManageConnections.Enable = true
	opts.ConnectedAccounts = map[string][]string{"github": {"ca_github"}}
	opts.MCP = true

	sess, err := c.CreateSession(context.Background(), "user-1", opts)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.SessionID != "trs_test123" {
		t.Errorf("session_id = %q, want trs_test123", sess.SessionID)
	}
	if sess.MCP == nil || sess.MCP.URL != "https://mcp.example.com" {
		t.Errorf("unexpected MCP: %+v", sess.MCP)
	}
	if sess.MCP.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("MCP headers = %+v", sess.MCP.Headers)
	}
	if gotAPIKey != "user-key" {
		t.Errorf("x-api-key = %q", gotAPIKey)
	}
	if gotBody.UserID != "user-1" {
		t.Errorf("user_id = %q", gotBody.UserID)
	}
	if gotBody.ManageConnections == nil || !gotBody.ManageConnections.Enable {
		t.Errorf("manage_connections = %+v", gotBody.ManageConnections)
	}
	if len(gotBody.ConnectedAccounts["github"]) != 1 || gotBody.ConnectedAccounts["github"][0] != "ca_github" {
		t.Errorf("connected_accounts = %+v", gotBody.ConnectedAccounts)
	}
}

func TestWithAPIKey_ScopesPerRequestKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		_, _ = w.Write([]byte(`{"items":[],"next_cursor":null}`))
	}))
	defer srv.Close()

	base := New(Config{APIKey: "default", BaseURL: srv.URL})
	if _, err := base.WithAPIKey("per-user").ListConnectedAccounts(context.Background(), ListConnectedAccountsOpts{}); err != nil {
		t.Fatalf("ListConnectedAccounts: %v", err)
	}
	if gotKey != "per-user" {
		t.Errorf("x-api-key = %q, want per-user", gotKey)
	}
}

func TestExecuteTool_Success(t *testing.T) {
	var gotBody sessionExecuteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3.1/tool_router/session/sess-1/execute" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"ok":true},"error":null,"log_id":"log-1"}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	res, err := c.ExecuteTool(context.Background(), "sess-1", "GITHUB_CREATE_ISSUE", map[string]any{"title": "hi"})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if gotBody.ToolSlug != "GITHUB_CREATE_ISSUE" {
		t.Errorf("tool_slug = %q", gotBody.ToolSlug)
	}
	if gotBody.Arguments["title"] != "hi" {
		t.Errorf("arguments = %+v", gotBody.Arguments)
	}
	if res.LogID != "log-1" {
		t.Errorf("log_id = %q", res.LogID)
	}
}

func TestExecuteTool_ElicitationPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": {
				"type": "elicitation",
				"message": "needs input",
				"fields": [{"name": "repo", "required": true}]
			},
			"error": "needs user input",
			"log_id": "log-elic"
		}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	res, err := c.ExecuteTool(context.Background(), "sess-1", "SOME_TOOL", nil)
	if err == nil {
		t.Fatal("expected error for elicitation/needs-input payload")
	}
	if res == nil {
		t.Fatal("expected result alongside error")
	}
	if !strings.Contains(err.Error(), "needs user input") {
		t.Errorf("error = %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data["type"] != "elicitation" {
		t.Errorf("data type = %v", data["type"])
	}
}

func TestExecuteMeta_SendsSlug(t *testing.T) {
	var gotBody sessionExecuteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3.1/tool_router/session/sess-1/execute_meta" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"data":{},"error":null,"log_id":"m1"}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.ExecuteMeta(context.Background(), "sess-1", "COMPOSIO_SEARCH_TOOLS", map[string]any{}); err != nil {
		t.Fatalf("ExecuteMeta: %v", err)
	}
	if gotBody.Slug != "COMPOSIO_SEARCH_TOOLS" {
		t.Errorf("slug = %q", gotBody.Slug)
	}
}

func TestAttachSession_ReturnsSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3.1/tool_router/session/trs_abc/attach" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"session_id":"trs_abc","mcp":{"url":"https://mcp.example.com"}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	sess, err := c.AttachSession(context.Background(), "trs_abc")
	if err != nil {
		t.Fatalf("AttachSession: %v", err)
	}
	if sess.SessionID != "trs_abc" {
		t.Errorf("session_id = %q", sess.SessionID)
	}
}

func TestSearchTools_SendsQuery(t *testing.T) {
	var gotBody searchToolsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"success":true,"error":null,"results":[]}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	res, err := c.SearchTools(context.Background(), "sess-1", "create github issue")
	if err != nil {
		t.Fatalf("SearchTools: %v", err)
	}
	if !res.Success {
		t.Error("expected success")
	}
	if len(gotBody.Queries) != 1 || gotBody.Queries[0].UseCase != "create github issue" {
		t.Errorf("queries = %+v", gotBody.Queries)
	}
}
