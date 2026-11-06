package identity

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "typical password", password: "C0rrectHorse", wantErr: false},
		{name: "empty password still hashes", password: "", wantErr: false},
		{name: "over 72 bytes is rejected by bcrypt", password: strings.Repeat("aB3", 30), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hash, err := hashPassword([]byte(tt.password))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("hashPassword(%d-byte password) = %q, want error", len(tt.password), string(hash))
				}
				return
			}
			if err != nil {
				t.Fatalf("hashPassword(%q): %v", tt.password, err)
			}
			if !strings.HasPrefix(string(hash), "$2a$10$") {
				t.Fatalf("hash = %q, want a bcrypt hash with cost 10", string(hash))
			}
		})
	}
}

func TestCheckPassword(t *testing.T) {
	t.Parallel()

	const password = "C0rrectHorse"

	hash, err := hashPassword([]byte(password))
	if err != nil {
		t.Fatalf("setup: hashPassword: %v", err)
	}

	otherHash, err := hashPassword([]byte("Another1Pass"))
	if err != nil {
		t.Fatalf("setup: hashPassword: %v", err)
	}

	tests := []struct {
		name     string
		password string
		hash     string
		want     bool
	}{
		{name: "correct password", password: password, hash: string(hash), want: true},
		{name: "wrong password", password: "Wr0ngPass", hash: string(hash), want: false},
		{name: "password for different user", password: password, hash: string(otherHash), want: false},
		{name: "malformed hash", password: password, hash: "not-a-bcrypt-hash", want: false},
		{name: "empty hash", password: password, hash: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := checkPassword([]byte(tt.password), []byte(tt.hash))
			if got != tt.want {
				t.Fatalf("checkPassword(%q, %q) = %v, want %v", tt.password, tt.hash, got, tt.want)
			}
		})
	}
}

func TestHashPasswordDistinctSalts(t *testing.T) {
	t.Parallel()

	first, err := hashPassword([]byte("C0rrectHorse"))
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	second, err := hashPassword([]byte("C0rrectHorse"))
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}

	if string(first) == string(second) {
		t.Fatal("two hashes of the same password are identical; bcrypt salt is not being applied")
	}
	if !checkPassword([]byte("C0rrectHorse"), first) || !checkPassword([]byte("C0rrectHorse"), second) {
		t.Fatal("not every generated hash verifies the original password")
	}
}
