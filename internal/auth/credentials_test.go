package auth

import (
	"os"
	"path/filepath"
	"testing"
)

const validSettingsJSON = `{
  "settings": {
    "credentials": {
      "client_id": "file-id",
      "client_secret": "file-secret"
    },
    "organization_id": "file-org"
  }
}`

func writeSettings(t *testing.T, tmpDir, body string) {
	t.Helper()
	dir := filepath.Join(tmpDir, ".airbyte-agents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing settings: %v", err)
	}
}

func TestResolveSettings_EnvVars(t *testing.T) {
	tests := []struct {
		name      string
		clientID  string
		clientSec string
		orgID     string
		wantErr   bool
	}{
		{name: "all three env vars set", clientID: "env-id", clientSec: "env-secret", orgID: "env-org"},
		{name: "missing client_id", clientSec: "env-secret", orgID: "env-org", wantErr: true},
		{name: "missing client_secret", clientID: "env-id", orgID: "env-org", wantErr: true},
		{name: "missing organization_id", clientID: "env-id", clientSec: "env-secret", wantErr: true},
		{name: "all empty", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("AIRBYTE_CLIENT_ID", tt.clientID)
			t.Setenv("AIRBYTE_CLIENT_SECRET", tt.clientSec)
			t.Setenv("AIRBYTE_ORGANIZATION_ID", tt.orgID)

			s, err := ResolveSettings()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Credentials.ClientID != tt.clientID {
				t.Errorf("ClientID = %q, want %q", s.Credentials.ClientID, tt.clientID)
			}
			if s.Credentials.ClientSecret != tt.clientSec {
				t.Errorf("ClientSecret = %q, want %q", s.Credentials.ClientSecret, tt.clientSec)
			}
			if s.OrganizationID != tt.orgID {
				t.Errorf("OrganizationID = %q, want %q", s.OrganizationID, tt.orgID)
			}
		})
	}
}

func TestResolveSettings_EnvTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	writeSettings(t, tmpDir, validSettingsJSON)

	t.Setenv("AIRBYTE_CLIENT_ID", "env-id")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "env-secret")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "env-org")

	s, err := ResolveSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Credentials.ClientID != "env-id" {
		t.Errorf("expected env client_id, got %q", s.Credentials.ClientID)
	}
	if s.OrganizationID != "env-org" {
		t.Errorf("expected env organization_id, got %q", s.OrganizationID)
	}
}

func TestResolveSettings_PartialEnvFallsBackToFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	writeSettings(t, tmpDir, validSettingsJSON)

	// only two of three env vars set — partial env => fall through to file.
	t.Setenv("AIRBYTE_CLIENT_ID", "env-id")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "env-secret")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "")

	s, err := ResolveSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Credentials.ClientID != "file-id" {
		t.Errorf("expected file client_id, got %q", s.Credentials.ClientID)
	}
	if s.OrganizationID != "file-org" {
		t.Errorf("expected file organization_id, got %q", s.OrganizationID)
	}
}

func TestResolveSettings_FileMissingOrganizationID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AIRBYTE_CLIENT_ID", "")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "")

	writeSettings(t, tmpDir, `{
  "settings": {
    "credentials": {"client_id": "x", "client_secret": "y"}
  }
}`)

	if _, err := ResolveSettings(); err == nil {
		t.Fatal("expected error when organization_id is missing from settings file")
	}
}

func TestResolveSettings_FallsBackToFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AIRBYTE_CLIENT_ID", "")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "")
	writeSettings(t, tmpDir, validSettingsJSON)

	s, err := ResolveSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Credentials.ClientID != "file-id" {
		t.Errorf("ClientID = %q, want file-id", s.Credentials.ClientID)
	}
	if s.Credentials.ClientSecret != "file-secret" {
		t.Errorf("ClientSecret = %q, want file-secret", s.Credentials.ClientSecret)
	}
	if s.OrganizationID != "file-org" {
		t.Errorf("OrganizationID = %q, want file-org", s.OrganizationID)
	}
}

func TestSettingsFile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	original := &Settings{
		Credentials:    Credentials{ClientID: "test-id", ClientSecret: "test-secret"},
		OrganizationID: "test-org",
	}
	if err := WriteSettingsFile(original); err != nil {
		t.Fatalf("writing: %v", err)
	}

	path := filepath.Join(tmpDir, ".airbyte-agents", "settings.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}

	loaded, err := ReadSettingsFile()
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if loaded.Credentials.ClientID != original.Credentials.ClientID {
		t.Errorf("ClientID = %q, want %q", loaded.Credentials.ClientID, original.Credentials.ClientID)
	}
	if loaded.Credentials.ClientSecret != original.Credentials.ClientSecret {
		t.Errorf("ClientSecret = %q, want %q", loaded.Credentials.ClientSecret, original.Credentials.ClientSecret)
	}
	if loaded.OrganizationID != original.OrganizationID {
		t.Errorf("OrganizationID = %q, want %q", loaded.OrganizationID, original.OrganizationID)
	}
}

func TestSettingsFile_RoundTripsAllowDestructive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AIRBYTE_ALLOW_DESTRUCTIVE", "")

	original := &Settings{
		Credentials:      Credentials{ClientID: "id", ClientSecret: "secret"},
		OrganizationID:   "org",
		AllowDestructive: true,
	}
	if err := WriteSettingsFile(original); err != nil {
		t.Fatalf("writing: %v", err)
	}

	loaded, err := ReadSettingsFile()
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !loaded.AllowDestructive {
		t.Error("AllowDestructive did not survive round-trip")
	}

	// Resolved settings should also reflect the file value when env
	// doesn't override.
	t.Setenv("AIRBYTE_CLIENT_ID", "")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "")
	resolved, err := ResolveSettings()
	if err != nil {
		t.Fatalf("ResolveSettings: %v", err)
	}
	if !resolved.AllowDestructive {
		t.Error("ResolveSettings did not propagate AllowDestructive from file")
	}
}

func TestSettingsFile_AllowDestructiveDefaultsFalseWhenAbsent(t *testing.T) {
	// File has no allow_destructive key → field is false.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AIRBYTE_ALLOW_DESTRUCTIVE", "")
	t.Setenv("AIRBYTE_CLIENT_ID", "")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "")

	writeSettings(t, tmpDir, validSettingsJSON)

	s, err := ResolveSettings()
	if err != nil {
		t.Fatalf("ResolveSettings: %v", err)
	}
	if s.AllowDestructive {
		t.Error("AllowDestructive defaulted to true; expected false when key is absent")
	}
}

func TestResolveSettings_EnvAllowDestructiveOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// File explicitly enables it; env explicitly disables. Env should
	// win.
	if err := WriteSettingsFile(&Settings{
		Credentials:      Credentials{ClientID: "id", ClientSecret: "secret"},
		OrganizationID:   "org",
		AllowDestructive: true,
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	t.Setenv("AIRBYTE_CLIENT_ID", "")
	t.Setenv("AIRBYTE_CLIENT_SECRET", "")
	t.Setenv("AIRBYTE_ORGANIZATION_ID", "")
	t.Setenv("AIRBYTE_ALLOW_DESTRUCTIVE", "false")

	s, err := ResolveSettings()
	if err != nil {
		t.Fatalf("ResolveSettings: %v", err)
	}
	if s.AllowDestructive {
		t.Error("env AIRBYTE_ALLOW_DESTRUCTIVE=false did not override file value")
	}
}

func TestSettingsFile_PreservesNestedStructure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := WriteSettingsFile(&Settings{
		Credentials:    Credentials{ClientID: "a", ClientSecret: "b"},
		OrganizationID: "c",
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".airbyte-agents", "settings.json"))
	if err != nil {
		t.Fatalf("reading raw: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		`"settings"`,
		`"credentials"`,
		`"client_id": "a"`,
		`"client_secret": "b"`,
		`"organization_id": "c"`,
	} {
		if !contains(got, want) {
			t.Errorf("on-disk JSON missing %q\n%s", want, got)
		}
	}
}

func TestSettingsFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	writeSettings(t, tmpDir, "not valid json")

	if _, err := ReadSettingsFile(); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
