package auth

import (
	"fmt"
	"os"
	"strings"
)

// Credentials are the raw OAuth client credentials used to mint access
// tokens. The token endpoint is the only consumer.
type Credentials struct {
	ClientID     string
	ClientSecret string
}

// Settings is the full set of user-supplied configuration that determines
// who the CLI talks to: the OAuth credentials, the organization to scope
// every request to, and the user's preferred default workspace.
//
// organization_id is required — without it the API rejects most calls
// with a workspace_id-style validation error.
//
// Workspace is optional and represents the user's "I usually mean this
// one" workspace. When empty, the literal string "default" is used as a
// last-resort fallback (matching the behavior of the API's default
// workspace for new accounts).
type Settings struct {
	Credentials    Credentials
	OrganizationID string
	Workspace      string
	// AllowDestructive, when true, lets destructive operations (e.g.
	// `connectors delete`) run without an interactive confirmation
	// prompt. Intended as a one-time permission grant for agent harnesses
	// that cannot answer a prompt.
	AllowDestructive bool
	// TelemetryEnabled controls whether the CLI emits anonymous usage
	// events to Segment. Defaults to true when the file is missing the
	// key. AIRBYTE_TELEMETRY_MODE=disabled forces this off regardless of
	// the saved value.
	TelemetryEnabled bool
	// IsInternalUser marks the invocation as an Airbyte employee so
	// internal events can be filtered out of customer analytics. Defaults
	// to false when the file is missing the key. AIRBYTE_INTERNAL_USER
	// overrides the saved value when set.
	IsInternalUser bool
	// VersionCheckEnabled controls whether the CLI checks GitHub
	// releases for a newer version once per day and prints a nudge.
	// Defaults to true when the file is missing the key.
	// AIRBYTE_VERSION_CHECK=disabled forces this off regardless of the
	// saved value.
	VersionCheckEnabled bool
}

// ResolveSettings returns the Settings to use for the current invocation.
// Resolution order:
//  1. Environment variables (all three of AIRBYTE_CLIENT_ID,
//     AIRBYTE_CLIENT_SECRET, AIRBYTE_ORGANIZATION_ID must be set).
//  2. ~/.airbyte-cli/settings.json (all three fields must be populated).
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

	return nil, fmt.Errorf("no settings found: set AIRBYTE_CLIENT_ID, AIRBYTE_CLIENT_SECRET, and AIRBYTE_ORGANIZATION_ID environment variables, or create ~/.airbyte-cli/settings.json")
}

func fromEnv() (*Settings, bool) {
	id := os.Getenv("AIRBYTE_CLIENT_ID")
	secret := os.Getenv("AIRBYTE_CLIENT_SECRET")
	orgID := os.Getenv("AIRBYTE_ORGANIZATION_ID")
	if id == "" || secret == "" || orgID == "" {
		return nil, false
	}
	return &Settings{
		Credentials:         Credentials{ClientID: id, ClientSecret: secret},
		OrganizationID:      orgID,
		Workspace:           os.Getenv("AIRBYTE_WORKSPACE"),
		AllowDestructive:    parseBoolEnv(os.Getenv("AIRBYTE_ALLOW_DESTRUCTIVE")),
		TelemetryEnabled:    !telemetryDisabledFromEnv(),
		IsInternalUser:      parseBoolEnv(os.Getenv("AIRBYTE_INTERNAL_USER")),
		VersionCheckEnabled: !versionCheckDisabledFromEnv(),
	}, true
}

// telemetryDisabledFromEnv reports whether AIRBYTE_TELEMETRY_MODE explicitly
// disables telemetry. Any other value (empty, "basic", unrecognized) leaves
// the decision to the settings file / default.
func telemetryDisabledFromEnv() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AIRBYTE_TELEMETRY_MODE")), "disabled")
}

// versionCheckDisabledFromEnv reports whether AIRBYTE_VERSION_CHECK
// explicitly disables the version-check nudge. Any other value leaves the
// decision to the settings file / default. The naming mirrors
// AIRBYTE_TELEMETRY_MODE so the two opt-outs feel parallel.
func versionCheckDisabledFromEnv() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AIRBYTE_VERSION_CHECK")), "disabled")
}

// parseBoolEnv treats common truthy strings ("1", "true", "yes", "on",
// case-insensitive) as true and everything else as false. Used for
// optional boolean environment variables.
func parseBoolEnv(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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
	// Env var wins over the file when set to a non-empty value;
	// otherwise inherit the file's value (already populated by
	// ReadSettingsFile). An empty string is treated as "unset" since it
	// can't express a meaningful boolean.
	if v := os.Getenv("AIRBYTE_ALLOW_DESTRUCTIVE"); v != "" {
		s.AllowDestructive = parseBoolEnv(v)
	}
	if v := os.Getenv("AIRBYTE_INTERNAL_USER"); v != "" {
		s.IsInternalUser = parseBoolEnv(v)
	}
	if telemetryDisabledFromEnv() {
		s.TelemetryEnabled = false
	}
	if versionCheckDisabledFromEnv() {
		s.VersionCheckEnabled = false
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
