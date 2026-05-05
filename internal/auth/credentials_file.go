package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	credentialsDirName  = ".airbyte"
	credentialsFileName = "credentials"
	credentialsFileMode = 0o600
	credentialsDirMode  = 0o700
)

type credentialsFile struct {
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	OrganizationID string `json:"organization_id,omitempty"`
}

func credentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, credentialsDirName, credentialsFileName)
}

func ReadCredentialsFile() (*Credentials, error) {
	path := credentialsPath()
	if path == "" {
		return nil, fmt.Errorf("unable to determine home directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cf credentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing credentials file: %w", err)
	}

	return &Credentials{
		ClientID:       cf.ClientID,
		ClientSecret:   cf.ClientSecret,
		OrganizationID: cf.OrganizationID,
	}, nil
}

func WriteCredentialsFile(creds *Credentials) error {
	path := credentialsPath()
	if path == "" {
		return fmt.Errorf("unable to determine home directory")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, credentialsDirMode); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}

	cf := credentialsFile{
		ClientID:       creds.ClientID,
		ClientSecret:   creds.ClientSecret,
		OrganizationID: creds.OrganizationID,
	}
	content, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing credentials: %w", err)
	}

	if err := tmp.Chmod(credentialsFileMode); err != nil {
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
		return fmt.Errorf("renaming credentials file: %w", err)
	}

	return nil
}
