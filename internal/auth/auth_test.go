package auth

import (
	"testing"
)

func TestHashPasswordAndCheckPasswordHash(t *testing.T) {
	password := "MySecurePassword123!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}
	err = CheckPasswordHash(password, hash)
	if err != nil {
		t.Errorf("CheckPasswordHash failed with correct password: %v", err)
	}
	err = CheckPasswordHash("WrongPassword", hash)
	if err == nil {
		t.Error("CheckPasswordHash did not fail with incorrect password")
	}
}
