package auth

import (
	"fmt"
	"os"
)

type Credentials struct {
	ClientID       string
	ClientSecret   string
	OrganizationID string
}

func ResolveCredentials() (*Credentials, error) {
	if creds, ok := fromEnv(); ok {
		return creds, nil
	}

	if creds, ok, err := fromFile(); ok {
		return creds, nil
	} else if err != nil {
		return nil, fmt.Errorf("credentials file error: %w", err)
	}

	return nil, fmt.Errorf("no credentials found: set AIRBYTE_CLIENT_ID and AIRBYTE_CLIENT_SECRET environment variables, or create ~/.airbyte/credentials")
}

func fromEnv() (*Credentials, bool) {
	id := os.Getenv("AIRBYTE_CLIENT_ID")
	secret := os.Getenv("AIRBYTE_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil, false
	}
	return &Credentials{
		ClientID:       id,
		ClientSecret:   secret,
		OrganizationID: os.Getenv("AIRBYTE_ORGANIZATION_ID"),
	}, true
}

func fromFile() (*Credentials, bool, error) {
	creds, err := ReadCredentialsFile()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var missing []string
	if creds.ClientID == "" {
		missing = append(missing, "client_id")
	}
	if creds.ClientSecret == "" {
		missing = append(missing, "client_secret")
	}
	if len(missing) > 0 {
		return nil, false, fmt.Errorf("credentials file is missing required fields: %s", joinFields(missing))
	}
	return creds, true, nil
}

func joinFields(fields []string) string {
	switch len(fields) {
	case 1:
		return fields[0]
	default:
		return fields[0] + " and " + fields[1]
	}
}
