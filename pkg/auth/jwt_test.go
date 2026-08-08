package auth

import (
	"testing"
	"time"
)

func newManager() *TokenManager {
	return NewTokenManager("test-secret", 15*time.Minute, 24*time.Hour, "test")
}

func TestGenerateAndParse(t *testing.T) {
	m := newManager()
	pair, err := m.GeneratePair("u1", "a@b.com", RoleAdmin)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := m.Parse(pair.AccessToken)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != "u1" || claims.Email != "a@b.com" {
		t.Errorf("unexpected claims: %+v", claims)
	}
	if !claims.IsAdmin() {
		t.Errorf("expected admin role")
	}
	if claims.Type != AccessToken {
		t.Errorf("expected access token type, got %q", claims.Type)
	}
}

func TestParseRefresh_RejectsAccessToken(t *testing.T) {
	m := newManager()
	pair, _ := m.GeneratePair("u1", "a@b.com", RoleUser)

	if _, err := m.ParseRefresh(pair.AccessToken); err == nil {
		t.Error("expected error when parsing access token as refresh")
	}
	if _, err := m.ParseRefresh(pair.RefreshToken); err != nil {
		t.Errorf("expected refresh token to parse, got %v", err)
	}
}

func TestParse_RejectsTamperedToken(t *testing.T) {
	m := newManager()
	pair, _ := m.GeneratePair("u1", "a@b.com", RoleUser)
	if _, err := m.Parse(pair.AccessToken + "tampered"); err == nil {
		t.Error("expected error for tampered token")
	}
}

func TestParse_RejectsWrongSecret(t *testing.T) {
	m := newManager()
	pair, _ := m.GeneratePair("u1", "a@b.com", RoleUser)
	other := NewTokenManager("different-secret", time.Minute, time.Hour, "test")
	if _, err := other.Parse(pair.AccessToken); err == nil {
		t.Error("expected error when verifying with wrong secret")
	}
}

func BenchmarkGeneratePair(b *testing.B) {
	m := newManager()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.GeneratePair("u1", "a@b.com", RoleUser); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	m := newManager()
	pair, _ := m.GeneratePair("u1", "a@b.com", RoleUser)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.Parse(pair.AccessToken); err != nil {
			b.Fatal(err)
		}
	}
}
