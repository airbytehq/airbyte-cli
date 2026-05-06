package auth

import (
	"fmt"
	"os"
)

// Credentials are the raw OAuth client credentials used to mint access
// tokens. The token endpoint is the only consumer.
type Credentials struct {
	ClientID     string
	ClientSecret string
}

// Settings is the full set of user-supplied configuration that determines
// who the CLI talks to: the OAuth credentials plus the organization to
// scope every request to. organization_id is required — without it the
// API rejects most calls with a workspace_id-style validation error.
type Settings struct {
	Credentials    Credentials
	OrganizationID string
}

// ResolveSettings returns the Settings to use for the current invocation.
// Resolution order:
//  1. Environment variables (all three of AIRBYTE_CLIENT_ID,
//     AIRBYTE_CLIENT_SECRET, AIRBYTE_ORGANIZATION_ID must be set).
//  2. ~/.airbyte/settings.json (all three fields must be populated).
//  3. Error.
func ResolveSettings() (*Settings, error) {
	if s, ok := fromEnv(); ok {
		return s, nil
	}

	if s, ok, err := fromFile(); ok {
		return s, nil
	} else if err != nil {
		return nil, fmt.Errorf("settings file error: %w", err)
	}

	return nil, fmt.Errorf("no settings found: set AIRBYTE_CLIENT_ID, AIRBYTE_CLIENT_SECRET, and AIRBYTE_ORGANIZATION_ID environment variables, or create ~/.airbyte/settings.json")
}

func fromEnv() (*Settings, bool) {
	id := os.Getenv("AIRBYTE_CLIENT_ID")
	secret := os.Getenv("AIRBYTE_CLIENT_SECRET")
	orgID := os.Getenv("AIRBYTE_ORGANIZATION_ID")
	if id == "" || secret == "" || orgID == "" {
		return nil, false
	}
	return &Settings{
		Credentials:    Credentials{ClientID: id, ClientSecret: secret},
		OrganizationID: orgID,
	}, true
}

func fromFile() (*Settings, bool, error) {
	s, err := ReadSettingsFile()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var missing []string
	if s.Credentials.ClientID == "" {
		missing = append(missing, "settings.credentials.client_id")
	}
	if s.Credentials.ClientSecret == "" {
		missing = append(missing, "settings.credentials.client_secret")
	}
	if s.OrganizationID == "" {
		missing = append(missing, "settings.organization_id")
	}
	if len(missing) > 0 {
		return nil, false, fmt.Errorf("settings file is missing required fields: %s", joinFields(missing))
	}
	return s, true, nil
}

func joinFields(fields []string) string {
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	case 2:
		return fields[0] + " and " + fields[1]
	default:
		// Oxford-comma list for 3+ fields.
		out := ""
		for i, f := range fields {
			switch {
			case i == 0:
				out = f
			case i == len(fields)-1:
				out += ", and " + f
			default:
				out += ", " + f
			}
		}
		return out
	}
}
