package assistant

import (
	"context"
	"sync"
	"testing"
	"time"

	assistantsettings "github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	userplugin "github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/llm"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/store"
)

type recordingClassifier struct {
	mu   sync.Mutex
	last string
}

func (r *recordingClassifier) Classify(_ context.Context, req llm.ClassifyRequest) (*llm.ClassifyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = req.Text
	return &llm.ClassifyResult{Matched: false}, nil
}

func (r *recordingClassifier) lastText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

type fakeSessionRepo struct {
	mu    sync.Mutex
	items map[string]*Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{items: map[string]*Session{}}
}

func (r *fakeSessionRepo) Create(_ context.Context, s *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *s
	r.items[s.ID] = &clone
	return nil
}

func (r *fakeSessionRepo) FindByID(_ context.Context, id any) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.items[id.(string)]; ok {
		clone := *s
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakeSessionRepo) Update(_ context.Context, id any, s *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *s
	r.items[id.(string)] = &clone
	return nil
}

func (r *fakeSessionRepo) Delete(context.Context, any) error { return nil }
func (r *fakeSessionRepo) Find(context.Context, store.Query) ([]Session, error) {
	return nil, nil
}
func (r *fakeSessionRepo) FindOne(context.Context, store.Query) (*Session, error) {
	return nil, store.ErrNotFound
}
func (r *fakeSessionRepo) Count(context.Context, store.Query) (int64, error) { return 0, nil }
func (r *fakeSessionRepo) FindOneAndUpdate(context.Context, store.Query, map[string]any) (*Session, error) {
	return nil, store.ErrNotFound
}

type fakeMessageRepo struct {
	mu    sync.Mutex
	items []Message
}

func (r *fakeMessageRepo) Create(_ context.Context, m *Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, *m)
	return nil
}

func (r *fakeMessageRepo) FindBySessionID(_ context.Context, sessionID string) ([]Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Message, 0)
	for _, m := range r.items {
		if m.SessionID == sessionID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeMessageRepo) FindByID(context.Context, any) (*Message, error) {
	return nil, store.ErrNotFound
}
func (r *fakeMessageRepo) Update(context.Context, any, *Message) error { return nil }
func (r *fakeMessageRepo) Delete(context.Context, any) error           { return nil }
func (r *fakeMessageRepo) Find(context.Context, store.Query) ([]Message, error) {
	return nil, nil
}
func (r *fakeMessageRepo) FindOne(context.Context, store.Query) (*Message, error) {
	return nil, store.ErrNotFound
}
func (r *fakeMessageRepo) Count(context.Context, store.Query) (int64, error) { return 0, nil }
func (r *fakeMessageRepo) FindOneAndUpdate(context.Context, store.Query, map[string]any) (*Message, error) {
	return nil, store.ErrNotFound
}

type fakeSettingsRepo struct {
	settings *assistantsettings.Settings
}

func (r *fakeSettingsRepo) Create(_ context.Context, s *assistantsettings.Settings) error {
	r.settings = s
	return nil
}
func (r *fakeSettingsRepo) FindByID(context.Context, any) (*assistantsettings.Settings, error) {
	if r.settings == nil {
		return nil, store.ErrNotFound
	}
	clone := *r.settings
	return &clone, nil
}
func (r *fakeSettingsRepo) Update(context.Context, any, *assistantsettings.Settings) error {
	return nil
}
func (r *fakeSettingsRepo) Delete(context.Context, any) error { return nil }
func (r *fakeSettingsRepo) Find(context.Context, store.Query) ([]assistantsettings.Settings, error) {
	return nil, nil
}
func (r *fakeSettingsRepo) FindOne(context.Context, store.Query) (*assistantsettings.Settings, error) {
	return nil, store.ErrNotFound
}
func (r *fakeSettingsRepo) Count(context.Context, store.Query) (int64, error) { return 0, nil }
func (r *fakeSettingsRepo) FindOneAndUpdate(context.Context, store.Query, map[string]any) (*assistantsettings.Settings, error) {
	return nil, store.ErrNotFound
}

type fakeUserPluginRepo struct {
	installs []userplugin.UserPlugin
}

func (r fakeUserPluginRepo) FindByUserID(context.Context, string) ([]userplugin.UserPlugin, error) {
	return r.installs, nil
}
func (fakeUserPluginRepo) FindByUserIDAndPluginID(context.Context, string, string) (*userplugin.UserPlugin, error) {
	return nil, store.ErrNotFound
}
func (fakeUserPluginRepo) Create(context.Context, *userplugin.UserPlugin) error { return nil }
func (r fakeUserPluginRepo) FindByID(_ context.Context, id any) (*userplugin.UserPlugin, error) {
	for i := range r.installs {
		if r.installs[i].ID == id.(string) {
			clone := r.installs[i]
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}
func (fakeUserPluginRepo) Update(context.Context, any, *userplugin.UserPlugin) error { return nil }
func (fakeUserPluginRepo) Delete(context.Context, any) error                         { return nil }
func (fakeUserPluginRepo) Find(context.Context, store.Query) ([]userplugin.UserPlugin, error) {
	return nil, nil
}
func (fakeUserPluginRepo) FindOne(context.Context, store.Query) (*userplugin.UserPlugin, error) {
	return nil, store.ErrNotFound
}
func (fakeUserPluginRepo) Count(context.Context, store.Query) (int64, error) { return 0, nil }
func (fakeUserPluginRepo) FindOneAndUpdate(context.Context, store.Query, map[string]any) (*userplugin.UserPlugin, error) {
	return nil, store.ErrNotFound
}

type fakePluginRepo struct {
	plugins []plugin.Plugin
}

func (r fakePluginRepo) Find(_ context.Context, q store.Query) ([]plugin.Plugin, error) {
	if len(q.Conditions) > 0 && q.Conditions[0].Field == "id" {
		return r.plugins, nil
	}
	return r.plugins, nil
}
func (r fakePluginRepo) FindByID(_ context.Context, id any) (*plugin.Plugin, error) {
	for i := range r.plugins {
		if r.plugins[i].ID == id.(string) {
			clone := r.plugins[i]
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}
func (r fakePluginRepo) FindBySlug(context.Context, string) (*plugin.Plugin, error) {
	return nil, store.ErrNotFound
}
func (r fakePluginRepo) Create(context.Context, *plugin.Plugin) error { return nil }
func (r fakePluginRepo) Update(context.Context, any, *plugin.Plugin) error {
	return nil
}
func (r fakePluginRepo) Delete(context.Context, any) error { return nil }
func (r fakePluginRepo) FindOne(context.Context, store.Query) (*plugin.Plugin, error) {
	return nil, store.ErrNotFound
}
func (r fakePluginRepo) Count(context.Context, store.Query) (int64, error) { return 0, nil }
func (r fakePluginRepo) FindOneAndUpdate(context.Context, store.Query, map[string]any) (*plugin.Plugin, error) {
	return nil, store.ErrNotFound
}

func TestProcessMessageStripsWakeWordBeforeClassification(t *testing.T) {
	sessions := newFakeSessionRepo()
	messages := &fakeMessageRepo{}
	settingsRepo := &fakeSettingsRepo{
		settings: &assistantsettings.Settings{
			UserID:    "user-1",
			WakeWord:  "Jarvis",
			UpdatedAt: time.Now().UTC(),
		},
	}
	settingsSvc := assistantsettings.NewService(settingsRepo)
	recorder := &recordingClassifier{}
	log, _ := logger.New("debug", false)

	catalog := plugin.Plugin{
		ID:   "plugin-1",
		Slug: "lights",
		Name: "Lights",
		Manifest: plugin.PluginManifest{
			Triggers:      []string{"turn on lights"},
			RequiredSetup: false,
			SetupType:     plugin.SetupTypeNone,
			Executor:      plugin.Executor{Type: plugin.ExecutorTypeBuiltin, Config: map[string]any{}},
		},
	}
	userPlugins := fakeUserPluginRepo{installs: []userplugin.UserPlugin{{
		ID: "install-1", UserID: "user-1", PluginID: "plugin-1", Enabled: true,
		SetupStatus: userplugin.SetupStatusCompleted,
	}}}

	svc := NewService(sessions, messages, settingsSvc, userPlugins, fakePluginRepo{plugins: []plugin.Plugin{catalog}}, recorder, NewStubExecutor(log), log)

	session := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		Status:    SessionStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := sessions.Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}

	_, err := svc.ProcessMessage(context.Background(), "user-1", "sess-1", ProcessMessageInput{
		Text:   "Jarvis turn on lights",
		Source: MessageSourceWakeWord,
	})
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}

	got := recorder.lastText()
	if got != "turn on lights" {
		t.Fatalf("classifier received %q, want %q", got, "turn on lights")
	}
}

func TestProcessMessageNoPluginsAcknowledgment(t *testing.T) {
	sessions := newFakeSessionRepo()
	messages := &fakeMessageRepo{}
	settingsRepo := &fakeSettingsRepo{
		settings: &assistantsettings.Settings{
			UserID:    "user-1",
			WakeWord:  "Jarvis",
			UpdatedAt: time.Now().UTC(),
		},
	}
	settingsSvc := assistantsettings.NewService(settingsRepo)
	log, _ := logger.New("debug", false)

	svc := NewService(sessions, messages, settingsSvc, fakeUserPluginRepo{}, fakePluginRepo{}, llm.NewMockClassifier(), NewStubExecutor(log), log)

	session := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		Status:    SessionStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = sessions.Create(context.Background(), session)

	out, err := svc.ProcessMessage(context.Background(), "user-1", "sess-1", ProcessMessageInput{
		Text:   "Jarvis what's the weather",
		Source: MessageSourceWakeWord,
	})
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if out.Reply.Type != ReplyTypeText {
		t.Fatalf("expected text reply, got %q", out.Reply.Type)
	}
	if out.Reply.Text != noActionText {
		t.Fatalf("unexpected reply text: %q", out.Reply.Text)
	}
	if out.Reply.Action != nil {
		t.Fatal("expected no action in reply")
	}
}
