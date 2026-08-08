package composio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const extendedCardJSON = `{
  "id": "c1",
  "name": "Fix login",
  "desc": "Users cannot log in",
  "idList": "l1",
  "url": "https://trello.com/c/c1",
  "closed": false,
  "due": null,
  "labels": [{"id": "lb1", "name": "bug", "color": "red"}],
  "members": [{"id": "m1", "fullName": "Alice", "username": "alice"}],
  "attachments": [{"name": "spec.pdf", "url": "https://example.com/spec.pdf", "mimeType": "application/pdf", "bytes": 1024}],
  "checklists": [{"name": "QA", "checkItems": [{"name": "Verify fix", "state": "incomplete"}, {"name": "Deploy", "state": "complete"}]}],
  "actions": [
    {"type": "commentCard", "date": "2026-06-01T10:00:00Z", "data": {"text": "Repro steps attached"}, "memberCreator": {"id": "m1", "fullName": "Alice", "username": "alice"}},
    {"type": "commentCard", "date": "2026-06-02T11:00:00Z", "data": {"text": "Looks good"}, "memberCreator": {"id": "m2"}},
    {"type": "updateCard", "date": "2026-06-03T12:00:00Z", "data": {"text": ""}, "memberCreator": {"id": "m1"}}
  ]
}`

func TestTrello_GetCard_RequestsFullFields(t *testing.T) {
	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tools/execute/"+ToolGetCard {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req executeRequest
		_ = json.Unmarshal(body, &req)
		gotArgs = req.Arguments
		_, _ = w.Write([]byte(`{"successful":true,"data":{"details":` + extendedCardJSON + `}}`))
	}))
	defer srv.Close()

	tr := NewTrello(New(Config{APIKey: "key", BaseURL: srv.URL}))
	card, err := tr.GetCard(context.Background(), "c1")
	if err != nil {
		t.Fatalf("get card: %v", err)
	}
	if gotArgs["idCard"] != "c1" {
		t.Errorf("idCard = %v", gotArgs["idCard"])
	}
	if gotArgs["fields"] != "name,desc,idList,url,closed,due,labels" {
		t.Errorf("fields = %v", gotArgs["fields"])
	}
	if gotArgs["attachments"] != "true" || gotArgs["checklists"] != "all" || gotArgs["members"] != "true" {
		t.Errorf("sub-resource args missing: %+v", gotArgs)
	}
	if card.Name != "Fix login" || card.Desc != "Users cannot log in" {
		t.Errorf("core fields missing: %+v", card)
	}
}

func TestDecodeData_ExtendedCard(t *testing.T) {
	card, err := decodeData[Card]([]byte(`{"details":` + extendedCardJSON + `}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(card.Labels) != 1 || card.Labels[0].Name != "bug" {
		t.Errorf("labels: %+v", card.Labels)
	}
	if len(card.Members) != 1 || card.Members[0].FullName != "Alice" {
		t.Errorf("members: %+v", card.Members)
	}
	if len(card.Attachments) != 1 || card.Attachments[0].Name != "spec.pdf" {
		t.Errorf("attachments: %+v", card.Attachments)
	}
	if len(card.Checklists) != 1 || len(card.Checklists[0].CheckItems) != 2 {
		t.Errorf("checklists: %+v", card.Checklists)
	}
	if card.Checklists[0].CheckItems[1].State != "complete" {
		t.Errorf("check item state: %+v", card.Checklists[0].CheckItems[1])
	}
	if card.Due != "" {
		t.Errorf("null due should decode as empty string, got %q", card.Due)
	}
}

func TestDecodeData_ExtendedCard_ResponseDataEnvelope(t *testing.T) {
	card, err := decodeData[Card]([]byte(`{"response_data":` + extendedCardJSON + `}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if card.ID != "c1" || card.Name != "Fix login" {
		t.Errorf("unexpected card: %+v", card)
	}
}

func TestCard_Comments_FiltersAndAuthorFallback(t *testing.T) {
	card, err := decodeData[Card]([]byte(extendedCardJSON))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	comments := card.Comments()
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d: %+v", len(comments), comments)
	}
	if comments[0].Author != "Alice" || comments[0].Text != "Repro steps attached" {
		t.Errorf("first comment: %+v", comments[0])
	}
	if comments[1].Author != "m2" {
		t.Errorf("author fallback to id: got %q, want m2", comments[1].Author)
	}
	if comments[1].Date != "2026-06-02T11:00:00Z" {
		t.Errorf("date: %+v", comments[1])
	}
}

func TestCard_Comments_EmptyWhenNoCommentActions(t *testing.T) {
	card := Card{Actions: []Action{{Type: "updateCard", Data: struct {
		Text string `json:"text"`
	}{Text: "ignored"}}}}
	if len(card.Comments()) != 0 {
		t.Errorf("expected no comments, got %+v", card.Comments())
	}
}
