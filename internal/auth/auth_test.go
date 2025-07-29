package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCheckPasswordHash(t *testing.T) {
	password1 := "correctPassword123!"
	password2 := "anotherPassword456!"
	hash1, _ := HashPassword(password1)
	hash2, _ := HashPassword(password2)

	tests := []struct {
		name     string
		password string
		hash     string
		wantErr  bool
	}{
		{
			name:     "Correct password",
			password: password1,
			hash:     hash1,
			wantErr:  false,
		},
		{
			name:     "Incorrect password",
			password: "wrongPassword",
			hash:     hash1,
			wantErr:  true,
		},
		{
			name:     "Password doesn't match different hash",
			password: password1,
			hash:     hash2,
			wantErr:  true,
		},
		{
			name:     "Empty password",
			password: "",
			hash:     hash1,
			wantErr:  true,
		},
		{
			name:     "Invalid hash",
			password: password1,
			hash:     "invalidhash",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPasswordHash(tt.password, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPasswordHash() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateJWT(t *testing.T) {
	validToken, _ := MakeJWT(uuid.New(), "key", time.Minute)
	expiredToken, _ := MakeJWT(uuid.New(), "key", -time.Minute)

	tests := []struct {
		name        string
		tokenString string
		key         string
		wantErr     bool
	}{
		{
			name:        "valid token, valid key",
			tokenString: validToken,
			key:         "key",
			wantErr:     false,
		},
		{
			name:        "valid token, invalid key",
			tokenString: validToken,
			key:         "secret",
			wantErr:     true,
		},
		{
			name:        "expired token",
			tokenString: expiredToken,
			key:         "key",
			wantErr:     true,
		},
		{
			name:        "empty token",
			tokenString: "",
			key:         "key",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateJWT(tt.tokenString, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJWT() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		header    http.Header
		want      string
		expectErr bool
	}{
		{
			name: "valid token",
			header: func() http.Header {
				h := http.Header{}
				h.Set("Authorization", "Bearer TOKEN_STRING")
				return h
			}(),
			want:      "TOKEN_STRING",
			expectErr: false,
		},
		{
			name:      "missing Authorization header",
			header:    http.Header{},
			want:      "",
			expectErr: true,
		},
		{
			name: "empty Bearer token",
			header: func() http.Header {
				h := http.Header{}
				h.Set("Authorization", "Bearer")
				return h
			}(),
			want:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetBearerToken(tt.header)
			if (err != nil) != tt.expectErr {
				t.Errorf("GetBearerToken() error = %v, expectErr = %v", err, tt.expectErr)
			}
			if got != tt.want {
				t.Errorf("GetBearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}
