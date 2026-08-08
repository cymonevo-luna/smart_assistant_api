package user

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/cache"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/queue"
	"github.com/cymonevo/go_template/pkg/store"
)

// fakeRepo is an in-memory Repository used to test the service in isolation
// from any real database.
type fakeRepo struct {
	mu    sync.Mutex
	items map[string]*User
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]*User{}} }

func (r *fakeRepo) Create(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *u
	r.items[u.ID] = &clone
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id any) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.items[id.(string)]; ok {
		clone := *u
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakeRepo) FindOne(_ context.Context, q store.Query) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range q.Conditions {
		if c.Field == "email" {
			for _, u := range r.items {
				if u.Email == c.Value {
					clone := *u
					return &clone, nil
				}
			}
		}
	}
	return nil, store.ErrNotFound
}

func (r *fakeRepo) Find(_ context.Context, _ store.Query) ([]User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]User, 0, len(r.items))
	for _, u := range r.items {
		out = append(out, *u)
	}
	return out, nil
}

func (r *fakeRepo) Update(_ context.Context, id any, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *u
	r.items[id.(string)] = &clone
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	delete(r.items, id.(string))
	return nil
}

func (r *fakeRepo) Count(_ context.Context, _ store.Query) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.items)), nil
}

func (r *fakeRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.FindOne(ctx, store.NewQuery().Eq("email", email))
}

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	log, _ := logger.New("error", false)
	repo := newFakeRepo()
	c := cache.NewTyped[User](cache.NewMemory(), time.Minute)
	tokens := auth.NewTokenManager("secret", 15*time.Minute, time.Hour, "test")
	q := queue.NewMemory(log, queue.DefaultRetryPolicy)
	return NewService(repo, c, store.NoopTxManager{}, q, tokens, log), repo
}

func TestService_CreateAndAuthenticate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	u, err := svc.Create(ctx, CreateUserInput{Email: "A@B.com", Name: "Alice", Password: "supersecret"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Email != "a@b.com" {
		t.Errorf("email should be normalised, got %q", u.Email)
	}
	if u.Role != auth.RoleUser {
		t.Errorf("default role should be user, got %q", u.Role)
	}
	if u.Password == "supersecret" {
		t.Error("password must be hashed")
	}

	pair, got, err := svc.Authenticate(ctx, "a@b.com", "supersecret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != u.ID || pair.AccessToken == "" {
		t.Error("unexpected authentication result")
	}
}

func TestService_CreateDuplicateEmail(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	in := CreateUserInput{Email: "dup@b.com", Name: "Dup", Password: "supersecret"}

	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, in); err == nil {
		t.Fatal("expected conflict error on duplicate email")
	}
}

func TestService_AuthenticateWrongPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Create(ctx, CreateUserInput{Email: "x@b.com", Name: "X", Password: "supersecret"})

	if _, _, err := svc.Authenticate(ctx, "x@b.com", "wrongpass"); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestService_GetByIDCaches(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	u, _ := svc.Create(ctx, CreateUserInput{Email: "c@b.com", Name: "C", Password: "supersecret"})

	// First read populates the cache.
	if _, err := svc.GetByID(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	// Remove from repo; a cached read should still succeed.
	_ = repo.Delete(ctx, u.ID)
	if _, err := svc.GetByID(ctx, u.ID); err != nil {
		t.Fatalf("expected cached hit, got %v", err)
	}
}

// BenchmarkServiceCreate exercises the full create path including bcrypt
// hashing, which dominates the cost and is the heaviest per-request operation.
func BenchmarkServiceCreate(b *testing.B) {
	log, _ := logger.New("error", false)
	repo := newFakeRepo()
	c := cache.NewTyped[User](cache.NewMemory(), time.Minute)
	tokens := auth.NewTokenManager("secret", 15*time.Minute, time.Hour, "test")
	q := queue.NewMemory(log, queue.DefaultRetryPolicy)
	svc := NewService(repo, c, store.NoopTxManager{}, q, tokens, log)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Create(ctx, CreateUserInput{
			Email:    "bench" + time.Now().Format("150405.000000000") + "@b.com",
			Name:     "Bench",
			Password: "supersecret",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
