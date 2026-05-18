package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/auth"
	"github.com/airbytehq/airbyte-cli/internal/auth/browserlogin"
	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/config"
	outputpkg "github.com/airbytehq/airbyte-cli/internal/output"
	"github.com/airbytehq/airbyte-cli/internal/telemetry"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	loginManualFlag bool
	loginOrgIDFlag  string
)

// loginParams holds the values consumed by the login runners so the
// browser/manual entry points can be unit-tested without going through
// Cobra (which calls os.Exit deep in the failure branches).
type loginParams struct {
	manual     bool
	orgID      string
	in         io.Reader
	out        io.Writer
	stderr     io.Writer
	stdinIsTTY bool
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in to airbyte.ai and save credentials locally",
	Long: `Sign in to airbyte.ai. By default this opens your browser to complete
the Keycloak login, then bootstraps your client_id, client_secret, and
organization_id from the airbyte.ai bootstrap endpoints and writes them to
~/.airbyte-cli/settings.json with 0600 permissions.

Use --manual for the legacy prompt-based flow (useful when no browser is
available, e.g. on a headless server). Use --org-id <uuid> to skip the
multi-organization picker when you belong to more than one organization.

The workspace is used as the fallback for any command that takes a
'workspace' parameter when one isn't supplied. The browser flow does not
prompt for a workspace; if you need to change yours, edit
~/.airbyte-cli/settings.json directly or use 'workspaces use'.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		p := loginParams{
			manual:     loginManualFlag,
			orgID:      loginOrgIDFlag,
			in:         os.Stdin,
			out:        os.Stdout,
			stderr:     os.Stderr,
			stdinIsTTY: stdinIsTTY(),
		}

		var (
			fresh *auth.Settings
			err   error
		)
		if p.manual {
			fresh, err = runManualLogin(ctx, p)
		} else {
			fresh, err = runBrowserLogin(ctx, p)
		}
		if err != nil {
			emitLoginTelemetry(start, "", false, classifyError(err))
			outputpkg.WriteError(map[string]any{"type": "error", "message": err.Error()})
			os.Exit(exitCodeFor(err))
		}

		existing, _ := auth.ReadSettingsFile() // nil when the file doesn't exist
		merged := mergePreservedSettings(existing, fresh)
		fmt.Fprintln(os.Stderr, "Saving credentials to ~/.airbyte-cli/settings.json…")
		if werr := auth.WriteSettingsFile(merged); werr != nil {
			emitLoginTelemetry(start, merged.OrganizationID, false, "write_settings")
			outputpkg.WriteError(map[string]any{"type": "error", "message": werr.Error()})
			os.Exit(client.ExitGeneral)
		}

		emitLoginTelemetry(start, merged.OrganizationID, true, "")

		return outputpkg.Write(map[string]string{
			"status":  "saved",
			"message": "Settings written to ~/.airbyte-cli/settings.json",
		}, output)
	},
}

var loginShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the saved settings (with the client secret obfuscated)",
	Long: `Read ~/.airbyte-cli/settings.json and print its contents as JSON. The
client_secret is obfuscated — only the trailing characters are visible —
so the output is safe to paste into a bug report or share for debugging.

This command reads the file directly, not the runtime resolved settings.
If you have AIRBYTE_* environment variables set, they may override what's
shown here when the CLI actually makes API calls.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := auth.ReadSettingsFile()
		if err != nil {
			if os.IsNotExist(err) {
				outputpkg.WriteError(map[string]any{
					"type":    "not_found",
					"message": "settings file does not exist",
					"hint":    "run 'airbyte agents login' to create ~/.airbyte-cli/settings.json",
				})
				os.Exit(client.ExitNotFound)
			}
			outputpkg.WriteError(map[string]any{"type": "error", "message": err.Error()})
			os.Exit(client.ExitGeneral)
		}

		return outputpkg.Write(map[string]string{
			"client_id":       settings.Credentials.ClientID,
			"client_secret":   obfuscateSecret(settings.Credentials.ClientSecret),
			"organization_id": settings.OrganizationID,
			"workspace":       settings.Workspace,
		}, output)
	},
}

// obfuscateSecret replaces all but the last 4 characters of s with asterisks.
// Short secrets (<= 4 chars) are fully obfuscated. Empty input passes through.
// Pattern matches the AWS / GCP convention so users can confirm they're
// looking at the right credential without leaking it.
func obfuscateSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}

func init() {
	loginCmd.AddCommand(loginShowCmd)
	loginCmd.Flags().BoolVar(&loginManualFlag, "manual", false, "Use the legacy prompt-based flow instead of the browser")
	loginCmd.Flags().StringVar(&loginOrgIDFlag, "org-id", "", "Skip the multi-org picker by specifying the organization UUID")
	agentsCmd.AddCommand(loginCmd)
}

// runBrowserLogin opens the user's browser to Keycloak, exchanges the
// authorization code for a Keycloak access token, then calls the sonar
// bootstrap endpoints to retrieve (client_id, client_secret,
// organization_id). The returned Settings is NOT merged with anything on
// disk — the caller does that via mergePreservedSettings.
func runBrowserLogin(ctx context.Context, p loginParams) (*auth.Settings, error) {
	fmt.Fprintln(p.stderr, "Opening your browser to log in at app.airbyte.ai…")
	fmt.Fprintln(p.stderr, "If the browser doesn't open, run `airbyte agents login --manual`.")

	keycloakBase := os.Getenv("AIRBYTE_KEYCLOAK_URL")
	if keycloakBase == "" {
		keycloakBase = browserlogin.DefaultKeycloakBase
	}

	tokens, err := browserlogin.RunOAuthFlow(ctx, &browserlogin.Options{
		KeycloakBase: keycloakBase,
		Stderr:       p.stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("browser login: %w", err)
	}
	fmt.Fprintln(p.stderr, "Login successful.")
	fmt.Fprintln(p.stderr, "Obtaining credentials from airbyte.ai…")

	apiHost := config.Load().APIHost
	result, err := browserlogin.Bootstrap(ctx, &browserlogin.BootstrapOptions{
		APIHost:       apiHost,
		AccessToken:   tokens.AccessToken,
		OrgIDOverride: p.orgID,
		Stdin:         p.in,
		Stderr:        p.stderr,
		StdinIsTTY:    p.stdinIsTTY,
		CLIVersion:    Version,
	})
	if err != nil {
		return nil, err
	}

	return &auth.Settings{
		Credentials: auth.Credentials{
			ClientID:     result.ClientID,
			ClientSecret: result.ClientSecret,
		},
		OrganizationID: result.OrganizationID,
		// Workspace intentionally empty — mergePreservedSettings carries
		// forward the existing value or falls back to "default".
	}, nil
}

// runManualLogin is the legacy prompt-driven flow. Refactored out of the
// previous loginCmd.RunE body so the browser path can replace it as the
// default.
func runManualLogin(ctx context.Context, p loginParams) (*auth.Settings, error) {
	_ = ctx // current prompt helpers are blocking; ctx threading is a follow-up.
	fmt.Fprintln(p.stderr, "Find your Client ID, Client Secret, and Organization ID at Settings -> Profile in the airbyte.ai app")
	reader := bufio.NewReader(p.in)

	clientID, err := promptRequired(reader, "Client ID")
	if err != nil {
		return nil, err
	}
	clientSecret, err := promptSecret(reader, "Client Secret")
	if err != nil {
		return nil, err
	}
	orgID, err := promptRequired(reader, "Organization ID")
	if err != nil {
		return nil, err
	}
	workspace, err := promptWithDefault(reader, "Workspace", "default")
	if err != nil {
		return nil, err
	}
	return &auth.Settings{
		Credentials:    auth.Credentials{ClientID: clientID, ClientSecret: clientSecret},
		OrganizationID: orgID,
		Workspace:      workspace,
	}, nil
}

// mergePreservedSettings combines freshly-collected credentials with the
// non-credential fields from any prior settings file. Behavior:
//   - Credentials / OrganizationID always come from `fresh`.
//   - Workspace: use `fresh.Workspace` when non-empty (manual path); else
//     preserve existing; else default to "default".
//   - AllowDestructive, TelemetryEnabled, IsInternalUser: preserve existing
//     when present, else use the documented defaults (false, true, false).
//
// Closes the long-standing carry-forward gap where re-running login reset
// Workspace and AllowDestructive even though TelemetryEnabled and
// IsInternalUser were preserved.
func mergePreservedSettings(existing, fresh *auth.Settings) *auth.Settings {
	merged := &auth.Settings{
		Credentials:      fresh.Credentials,
		OrganizationID:   fresh.OrganizationID,
		Workspace:        fresh.Workspace,
		AllowDestructive: false,
		TelemetryEnabled: true,
		IsInternalUser:   false,
	}
	if existing != nil {
		if merged.Workspace == "" {
			merged.Workspace = existing.Workspace
		}
		merged.AllowDestructive = existing.AllowDestructive
		merged.TelemetryEnabled = existing.TelemetryEnabled
		merged.IsInternalUser = existing.IsInternalUser
	}
	if merged.Workspace == "" {
		merged.Workspace = "default"
	}
	return merged
}

// emitLoginTelemetry sends a single `login` event. Reads the existing
// settings file (if any) for telemetry preference + internal-user marker —
// matching the long-standing behavior where the freshly-written file's
// telemetry mode is the source of truth. On a fresh install (no file)
// defaults to telemetry enabled, non-internal.
func emitLoginTelemetry(start time.Time, orgID string, success bool, errorType string) {
	telemetryEnabled := true
	isInternalUser := false
	if s, err := auth.ReadSettingsFile(); err == nil {
		telemetryEnabled = s.TelemetryEnabled
		isInternalUser = s.IsInternalUser
	}
	mode := telemetry.ResolveMode(telemetryEnabled)
	t := telemetry.New(mode, orgID, Version, isInternalUser)
	t.TrackCommand(telemetry.CommandEvent{
		Command:    "login",
		Success:    success,
		ErrorType:  errorType,
		DurationMs: time.Since(start).Milliseconds(),
	})
	t.Flush()
}

// exitCodeFor maps an error to a CLI exit code. *client.APIError carries
// its own mapping (401/403→2, 404→3, 400/422→4, else 1); everything else
// falls through to ExitGeneral.
func exitCodeFor(err error) int {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ExitCode()
	}
	return client.ExitGeneral
}

// classifyError returns a coarse error_type label for telemetry. Prefers
// the *client.APIError.Type when present, otherwise inspects the error
// string for known OAuth / network markers.
func classifyError(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Type
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "state mismatch"):
		return "oauth_state_mismatch"
	case strings.Contains(msg, "context deadline"), strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "calling /"), strings.Contains(msg, "token exchange"):
		return "network"
	default:
		return "unknown"
	}
}

// stdinIsTTY reports whether os.Stdin is attached to a terminal. Uses
// os.ModeCharDevice on stdin's file info — works on macOS, Linux, and
// Windows without pulling in go-isatty.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func promptRequired(reader *bufio.Reader, label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		outputpkg.WriteError(map[string]any{
			"type":    "validation_error",
			"message": fmt.Sprintf("%s is required", label),
		})
		os.Exit(4)
	}
	return value, nil
}

// promptSecret prompts for a value and masks the input when stdin is a
// terminal. Falls back to a plain line read when stdin is piped (e.g.
// scripted setup), since there's nothing to mask in that case.
func promptSecret(reader *bufio.Reader, label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	fd := int(os.Stdin.Fd())
	var value string
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", label, err)
		}
		value = strings.TrimSpace(string(b))
	} else {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", label, err)
		}
		value = strings.TrimSpace(line)
	}
	if value == "" {
		outputpkg.WriteError(map[string]any{
			"type":    "validation_error",
			"message": fmt.Sprintf("%s is required", label),
		})
		os.Exit(4)
	}
	return value, nil
}

// promptWithDefault prints "<label> [<defaultValue>]: " and returns the
// user's input — or defaultValue if they hit Enter.
func promptWithDefault(reader *bufio.Reader, label, defaultValue string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultValue)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}
