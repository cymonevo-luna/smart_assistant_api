package crypto

import "testing"

func TestEncryptorRoundTrip(t *testing.T) {
	e, err := NewEncryptor("test-secret-key-for-encryption")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plain := "{\"refresh_token\":\"secret-token\"}"
	enc, err := e.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("expected ciphertext to differ from plaintext")
	}

	got, err := e.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("expected %q, got %q", plain, got)
	}
}

func TestEncryptorEmptySecret(t *testing.T) {
	_, err := NewEncryptor("")
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}
