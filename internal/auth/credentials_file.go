package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	settingsDirName  = ".airbyte"
	settingsFileName = "settings.json"
	settingsFileMode = 0o600
	settingsDirMode  = 0o700
)

// settingsFile is the on-disk shape of ~/.airbyte/settings.json:
//
//	{
//	  "settings": {
//	    "credentials":     { "client_id": "...", "client_secret": "..." },
//	    "organization_id": "..."
//	  }
//	}
//
// The outer wrapper exists so future settings (formats, defaults, telemetry
// preferences) can be added alongside `credentials` without churning the
// file structure again.
type settingsFile struct {
	Settings settingsBody `json:"settings"`
}

type settingsBody struct {
	Credentials    credentialsBody `json:"credentials"`
	OrganizationID string          `json:"organization_id"`
	Workspace      string          `json:"workspace,omitempty"`
}

type credentialsBody struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func settingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, settingsDirName, settingsFileName)
}

// ReadSettingsFile parses ~/.airbyte/settings.json into a Settings value.
// Returns the underlying error (including os.ErrNotExist) so callers can
// distinguish "no file" from "file is broken."
func ReadSettingsFile() (*Settings, error) {
	path := settingsPath()
	if path == "" {
		return nil, fmt.Errorf("unable to determine home directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sf settingsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing settings file: %w", err)
	}

	return &Settings{
		Credentials: Credentials{
			ClientID:     sf.Settings.Credentials.ClientID,
			ClientSecret: sf.Settings.Credentials.ClientSecret,
		},
		OrganizationID: sf.Settings.OrganizationID,
		Workspace:      sf.Settings.Workspace,
	}, nil
}

// WriteSettingsFile atomically writes ~/.airbyte/settings.json with 0600
// permissions, creating the directory if needed.
func WriteSettingsFile(s *Settings) error {
	path := settingsPath()
	if path == "" {
		return fmt.Errorf("unable to determine home directory")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, settingsDirMode); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}

	sf := settingsFile{
		Settings: settingsBody{
			Credentials: credentialsBody{
				ClientID:     s.Credentials.ClientID,
				ClientSecret: s.Credentials.ClientSecret,
			},
			OrganizationID: s.OrganizationID,
			Workspace:      s.Workspace,
		},
	}
	content, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing settings: %w", err)
	}

	if err := tmp.Chmod(settingsFileMode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("setting file permissions: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming settings file: %w", err)
	}

	return nil
}
