package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCredentials_EnvVars(t *testing.T) {
	tests := []struct {
		name       string
		clientID   string
		clientSec  string
		wantID     string
		wantSecret string
		wantErr    bool
	}{
		{
			name:       "both env vars set",
			clientID:   "env-id",
			clientSec:  "env-secret",
			wantID:     "env-id",
			wantSecret: "env-secret",
		},
		{
			name:      "only client_id set",
			clientID:  "env-id",
			clientSec: "",
			wantErr:   true,
		},
		{
			name:      "only client_secret set",
			clientID:  "",
			clientSec: "env-secret",
			wantErr:   true,
		},
		{
			name:    "neither set",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("AIRBYTE_CLIENT_ID", tt.clientID)
			t.Setenv("AIRBYTE_CLIENT_SECRET", tt.clientSec)

			creds, err := ResolveCredentials()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if creds.ClientID != tt.wantID {
				t.Errorf("ClientID = %q, want %q", creds.ClientID, tt.wantID)
			}
			if creds.ClientSecret != tt.wantSecret {
				t.Errorf("ClientSecret = %q, want %q", creds.ClientSecret, tt.wantSecret)
			}
		})
	}
}

func TestResolveCredentials_EnvTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, ".airbyte")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	content := `{"client_id":"file-id","client_secret":"file-secret"}`
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	t.Setenv("AIRBYTE_CLIENT_ID", "env-id")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "env-secret")

	creds, err := ResolveCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "env-id" {
		t.Errorf("expected env ClientID, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "env-secret" {
		t.Errorf("expected env ClientSecret, got %q", creds.ClientSecret)
	}
}

func TestResolveCredentials_FallsBackToFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AIRBYTE_CLIENT_ID", "")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "")

	dir := filepath.Join(tmpDir, ".airbyte")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	content := `{"client_id":"file-id","client_secret":"file-secret"}`
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	creds, err := ResolveCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "file-id" {
		t.Errorf("expected file ClientID, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "file-secret" {
		t.Errorf("expected file ClientSecret, got %q", creds.ClientSecret)
	}
}

func TestCredentialsFile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	original := &Credentials{ClientID: "test-id", ClientSecret: "test-secret"}
	if err := WriteCredentialsFile(original); err != nil {
		t.Fatalf("writing: %v", err)
	}

	path := filepath.Join(tmpDir, ".airbyte", "credentials")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}

	loaded, err := ReadCredentialsFile()
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if loaded.ClientID != original.ClientID {
		t.Errorf("ClientID = %q, want %q", loaded.ClientID, original.ClientID)
	}
	if loaded.ClientSecret != original.ClientSecret {
		t.Errorf("ClientSecret = %q, want %q", loaded.ClientSecret, original.ClientSecret)
	}
}

func TestCredentialsFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, ".airbyte")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	content := "not valid json"
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	_, err := ReadCredentialsFile()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
