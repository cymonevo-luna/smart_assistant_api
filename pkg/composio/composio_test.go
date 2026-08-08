package composio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecute_SuccessAndHeaders(t *testing.T) {
	var gotSlug, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotSlug = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		if req.Arguments["idBoard"] != "b1" {
			t.Errorf("missing argument, got %v", req.Arguments)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successful":true,"data":[{"id":"l1","name":"BACKLOG"}]}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	res, err := c.Execute(context.Background(), ToolGetLists, map[string]any{"idBoard": "b1"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.Successful {
		t.Error("expected successful result")
	}
	if gotAPIKey != "key" {
		t.Errorf("api key header = %q", gotAPIKey)
	}
	if gotSlug != "/api/v3/tools/execute/"+ToolGetLists {
		t.Errorf("unexpected path %q", gotSlug)
	}
}

func TestForEntity_OverridesUserAndAccounts(t *testing.T) {
	var gotUser, gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		gotUser = req.UserID
		gotAccount = req.ConnectedAccountID
		_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
	}))
	defer srv.Close()

	// Base client uses a default entity; ForEntity pins a different one.
	c := New(Config{APIKey: "key", BaseURL: srv.URL, UserID: "base-user"}).
		ForEntity("trello-user", "ca_trello")
	if _, err := c.Execute(context.Background(), "X", nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotUser != "trello-user" {
		t.Errorf("user_id = %q, want trello-user", gotUser)
	}
	if gotAccount != "ca_trello" {
		t.Errorf("connected_account_id = %q, want ca_trello", gotAccount)
	}
}

func TestForEntity_EmptyUserKeepsDefault(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		gotUser = req.UserID
		_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL, UserID: "base-user"}).ForEntity("", "ca_x")
	if _, err := c.Execute(context.Background(), "X", nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotUser != "base-user" {
		t.Errorf("user_id = %q, want base-user (unchanged)", gotUser)
	}
}

func TestExecute_ConnectedAccountFallback(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		seen = append(seen, req.ConnectedAccountID)
		// The first account fails; the second succeeds.
		if req.ConnectedAccountID == "acc-1" {
			_, _ = w.Write([]byte(`{"successful":false,"error":"not authorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL}).ForAccounts("acc-1", "acc-2")
	res, err := c.Execute(context.Background(), "X", nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.Successful {
		t.Error("expected success after fallback")
	}
	if len(seen) != 2 || seen[0] != "acc-1" || seen[1] != "acc-2" {
		t.Errorf("expected fallback acc-1 then acc-2, got %v", seen)
	}
}

func TestExecute_ConnectedAccountNotFoundAutoResolvesByEntity(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		seen = append(seen, req.ConnectedAccountID)
		if req.ConnectedAccountID != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"slug":"ActionExecute_ConnectedAccountNotFound","message":"No connected account found with ID stale for user ID entity-user for toolkit trello"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL, UserID: "entity-user"}).
		ForEntity("entity-user", "stale")
	res, err := c.Execute(context.Background(), "X", nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.Successful {
		t.Error("expected success after entity auto-resolve")
	}
	if len(seen) != 2 || seen[0] != "stale" || seen[1] != "" {
		t.Errorf("expected stale then auto-resolve, got %v", seen)
	}
}

func TestExecute_AllConnectedAccountsFail(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		_, _ = w.Write([]byte(`{"successful":false,"error":"nope"}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL}).ForAccounts("acc-1", "acc-2")
	if _, err := c.Execute(context.Background(), "X", nil); err == nil {
		t.Fatal("expected error when every connected account fails")
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestExecute_NoAccountsSendsEmpty(t *testing.T) {
	var got string
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		got = req.ConnectedAccountID
		_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Execute(context.Background(), "X", nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !hit {
		t.Fatal("expected a single request")
	}
	if got != "" {
		t.Errorf("expected empty connected account, got %q", got)
	}
}

func TestExecute_UnsuccessfulIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"successful":false,"error":"no connection"}`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Execute(context.Background(), "X", nil); err == nil {
		t.Fatal("expected error for unsuccessful execution")
	}
}

func TestExecute_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := New(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Execute(context.Background(), "X", nil); err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestTrello_GetListsAndCards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/tools/execute/" + ToolGetLists:
			_, _ = w.Write([]byte(`{"successful":true,"data":[{"id":"l1","name":"BACKLOG"},{"id":"l2","name":"DONE"}]}`))
		case "/api/v3/tools/execute/" + ToolGetCardsInList:
			_, _ = w.Write([]byte(`{"successful":true,"data":[{"id":"c1","name":"Card","idList":"l1"}]}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	tr := NewTrello(New(Config{APIKey: "key", BaseURL: srv.URL}))
	lists, err := tr.GetLists(context.Background(), "b1")
	if err != nil || len(lists) != 2 || lists[0].Name != "BACKLOG" {
		t.Fatalf("get lists: %v %v", lists, err)
	}
	cards, err := tr.GetCardsInList(context.Background(), "l1")
	if err != nil || len(cards) != 1 || cards[0].ID != "c1" {
		t.Fatalf("get cards: %v %v", cards, err)
	}
}

func TestDecodeData_AcceptsEnvelope(t *testing.T) {
	lists, err := decodeData[[]List]([]byte(`{"response_data":[{"id":"l1","name":"X"}]}`))
	if err != nil || len(lists) != 1 || lists[0].ID != "l1" {
		t.Fatalf("envelope decode: %v %v", lists, err)
	}
}

func TestDecodeData_AcceptsDetailsEnvelope(t *testing.T) {
	// Composio Trello GET tools nest the result under "details".
	lists, err := decodeData[[]List]([]byte(`{"details":[{"id":"l1","name":"BACKLOG"},{"id":"l2","name":"DONE"}]}`))
	if err != nil || len(lists) != 2 || lists[0].Name != "BACKLOG" {
		t.Fatalf("details envelope decode: %v %v", lists, err)
	}

	card, err := decodeData[Card]([]byte(`{"details":{"id":"c1","name":"Task","idList":"l1"}}`))
	if err != nil || card.ID != "c1" || card.IDList != "l1" {
		t.Fatalf("details envelope (object) decode: %+v %v", card, err)
	}
}

func TestDecodeData_BareArrayStillWorks(t *testing.T) {
	lists, err := decodeData[[]List]([]byte(`[{"id":"l1","name":"X"}]`))
	if err != nil || len(lists) != 1 || lists[0].ID != "l1" {
		t.Fatalf("bare array decode: %v %v", lists, err)
	}
}
