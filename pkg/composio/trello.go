package composio

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Trello tool slugs as exposed by the Composio Trello toolkit. They are declared
// as variables (not constants) so a deployment using different slugs can
// override them at startup without forking this package.
var (
	ToolGetLists       = "TRELLO_GET_BOARDS_LISTS_BY_ID_BOARD"
	ToolGetCardsInList = "TRELLO_GET_LISTS_CARDS_BY_ID_LIST"
	ToolGetCard        = "TRELLO_GET_CARDS_BY_ID_CARD"
	ToolCreateCard     = "TRELLO_ADD_CARDS"
	ToolMoveCard       = "TRELLO_UPDATE_CARDS_ID_LIST_BY_ID_CARD"
	ToolUpdateCardDesc = "TRELLO_UPDATE_CARDS_DESC_BY_ID_CARD"
	ToolAddComment     = "TRELLO_ADD_CARDS_ACTIONS_COMMENTS_BY_ID_CARD"
)

// Trello wraps a Client with typed helpers for the board operations luna-style
// orchestrators need: enumerate lists, read/move/update cards, and comment.
type Trello struct {
	c *Client
}

// NewTrello returns a Trello helper bound to c.
func NewTrello(c *Client) *Trello { return &Trello{c: c} }

// List is a Trello list (column) on a board.
type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Label is a Trello label attached to a card.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Member is a Trello member assigned to a card.
type Member struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Username string `json:"username"`
}

// CheckItem is one entry on a Trello checklist.
type CheckItem struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// Checklist is a Trello checklist on a card.
type Checklist struct {
	Name       string      `json:"name"`
	CheckItems []CheckItem `json:"checkItems"`
}

// Attachment is a file or link attached to a Trello card.
type Attachment struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	MimeType string `json:"mimeType"`
	Bytes    int64  `json:"bytes"`
}

// Action is a Trello card action (e.g. a comment). Only commentCard actions are
// surfaced via Card.Comments.
type Action struct {
	Type string `json:"type"`
	Date string `json:"date"`
	Data struct {
		Text string `json:"text"`
	} `json:"data"`
	MemberCreator struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
		Username string `json:"username"`
	} `json:"memberCreator"`
}

// Comment is a flattened card comment for orchestrator prompts.
type Comment struct {
	Author string
	Date   string
	Text   string
}

// Card is a Trello card with the fields orchestrators need for ticket breakdown
// and execution.
type Card struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Desc        string       `json:"desc"`
	IDList      string       `json:"idList"`
	URL         string       `json:"url"`
	Closed      bool         `json:"closed"`
	Due         string       `json:"due"`
	Labels      []Label      `json:"labels"`
	Members     []Member     `json:"members"`
	Attachments []Attachment `json:"attachments"`
	Checklists  []Checklist  `json:"checklists"`
	Actions     []Action     `json:"actions"`
}

// Comments returns commentCard actions with non-empty text. Author falls back
// from fullName to username to id when Composio/Trello omit display fields.
func (c Card) Comments() []Comment {
	var out []Comment
	for _, a := range c.Actions {
		if a.Type != "commentCard" {
			continue
		}
		text := strings.TrimSpace(a.Data.Text)
		if text == "" {
			continue
		}
		author := strings.TrimSpace(a.MemberCreator.FullName)
		if author == "" {
			author = strings.TrimSpace(a.MemberCreator.Username)
		}
		if author == "" {
			author = strings.TrimSpace(a.MemberCreator.ID)
		}
		out = append(out, Comment{
			Author: author,
			Date:   a.Date,
			Text:   text,
		})
	}
	return out
}

// GetLists returns the lists on a board identified by its id or short link.
func (t *Trello) GetLists(ctx context.Context, boardID string) ([]List, error) {
	res, err := t.c.Execute(ctx, ToolGetLists, map[string]any{"idBoard": boardID})
	if err != nil {
		return nil, err
	}
	return decodeData[[]List](res.Data)
}

// GetCardsInList returns the cards currently in a list.
func (t *Trello) GetCardsInList(ctx context.Context, listID string) ([]Card, error) {
	res, err := t.c.Execute(ctx, ToolGetCardsInList, map[string]any{"idList": listID})
	if err != nil {
		return nil, err
	}
	return decodeData[[]Card](res.Data)
}

// GetCard returns a single card by id with full ticket context for orchestrators.
// Trello's GET card endpoint returns only id and badges unless an explicit
// "fields" set is requested, so name/desc and sub-resources are omitted by
// default and agents would see an empty ticket. The arguments below request the
// core fields plus labels, members, attachments, checklists, due date, and
// recent comments.
func (t *Trello) GetCard(ctx context.Context, cardID string) (*Card, error) {
	res, err := t.c.Execute(ctx, ToolGetCard, map[string]any{
		"idCard":                      cardID,
		"fields":                      "name,desc,idList,url,closed,due,labels",
		"attachments":                 "true",
		"attachment_fields":           "name,url,mimeType,bytes",
		"checklists":                  "all",
		"checklist_fields":            "all",
		"members":                     "true",
		"member_fields":               "fullName,username",
		"actions":                     "commentCard",
		"actions_limit":               "20",
		"action_memberCreator":        "true",
		"action_memberCreator_fields": "fullName,username",
	})
	if err != nil {
		return nil, err
	}
	card, err := decodeData[Card](res.Data)
	if err != nil {
		return nil, err
	}
	return &card, nil
}

// CreateCard creates a card in the given list and returns it.
func (t *Trello) CreateCard(ctx context.Context, listID, name, desc string) (*Card, error) {
	res, err := t.c.Execute(ctx, ToolCreateCard, map[string]any{
		"idList": listID,
		"name":   name,
		"desc":   desc,
	})
	if err != nil {
		return nil, err
	}
	card, err := decodeData[Card](res.Data)
	if err != nil {
		return nil, err
	}
	return &card, nil
}

// MoveCard moves a card to a different list. The destination list id is passed
// in the "value" argument per the Composio Trello tool contract.
func (t *Trello) MoveCard(ctx context.Context, cardID, listID string) error {
	_, err := t.c.Execute(ctx, ToolMoveCard, map[string]any{
		"idCard": cardID,
		"value":  listID,
	})
	return err
}

// UpdateCardDescription replaces a card's description. The new description is
// passed in the "value" argument per the Composio Trello tool contract.
func (t *Trello) UpdateCardDescription(ctx context.Context, cardID, desc string) error {
	_, err := t.c.Execute(ctx, ToolUpdateCardDesc, map[string]any{
		"idCard": cardID,
		"value":  desc,
	})
	return err
}

// AddComment posts a comment to a card.
func (t *Trello) AddComment(ctx context.Context, cardID, text string) error {
	_, err := t.c.Execute(ctx, ToolAddComment, map[string]any{
		"idCard": cardID,
		"text":   text,
	})
	return err
}

var (
	boardURLRe  = regexp.MustCompile(`trello\.com/b/([A-Za-z0-9]+)`)
	bareBoardRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// ParseBoardID extracts a Trello board reference (its 8-character shortLink or
// 24-character id) from a board URL such as
// https://trello.com/b/abc12345/my-board. The board operations accept either
// form. A bare id/shortLink (no URL) is accepted and returned unchanged so
// callers can pass whichever the user supplied.
func ParseBoardID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if m := boardURLRe.FindStringSubmatch(s); m != nil {
		return m[1], nil
	}
	if bareBoardRe.MatchString(s) {
		return s, nil
	}
	return "", fmt.Errorf("composio: cannot extract a trello board id from %q", s)
}

// envelopeKeys are the wrapper keys Composio uses around an upstream tool
// payload. The exact key varies by tool/version (e.g. Trello GET tools nest the
// result under "details"), so decodeData unwraps the first one present.
var envelopeKeys = []string{"response_data", "details"}

// decodeData unmarshals a tool's data payload into T. Composio wraps the
// upstream payload in a single-key envelope (e.g. {"details": ...} or
// {"response_data": ...}); the wrapper is unwrapped first when present so both
// shapes decode correctly (a struct T would otherwise silently ignore the
// envelope and yield a zero value).
func decodeData[T any](data json.RawMessage) (T, error) {
	var zero T
	if len(data) == 0 {
		return zero, nil
	}

	// Unwrap a known envelope key when the payload is a JSON object that carries
	// one. A bare array/scalar (or an object without an envelope key) is decoded
	// as-is.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err == nil {
		for _, key := range envelopeKeys {
			if inner, ok := envelope[key]; ok && len(inner) > 0 {
				data = inner
				break
			}
		}
	}

	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return zero, fmt.Errorf("composio: cannot decode data into %T: %w", zero, err)
	}
	return out, nil
}
