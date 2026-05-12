package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/airbytehq/airbyte-agent-cli/internal/auth"
	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	outputpkg "github.com/airbytehq/airbyte-agent-cli/internal/output"
	"github.com/airbytehq/airbyte-agent-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure credentials and organization id interactively",
	Long: `Prompt for client_id, client_secret, organization_id, and a default
workspace, then save them to ~/.airbyte-agent/settings.json with 0600 permissions.
Run this once on a new machine or whenever your credentials change.

The workspace is used as the fallback for any command that takes a
'workspace' parameter when one isn't supplied. Press Enter to accept
'default'.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		reader := bufio.NewReader(os.Stdin)

		fmt.Fprintln(os.Stderr, "Find your Client ID, Client Secret, and Organization ID at Settings -> Profile in the airbyte.ai app")

		clientID, err := promptRequired(reader, "Client ID")
		if err != nil {
			return err
		}
		clientSecret, err := promptRequired(reader, "Client Secret")
		if err != nil {
			return err
		}
		orgID, err := promptRequired(reader, "Organization ID")
		if err != nil {
			return err
		}
		workspace, err := promptWithDefault(reader, "Workspace", "default")
		if err != nil {
			return err
		}

		// Preserve telemetry / internal-user values from any prior
		// settings file so re-running configure doesn't reset what the
		// user previously set. Missing file → use the documented
		// defaults (telemetry on, internal off).
		telemetryEnabled := true
		isInternalUser := false
		if existing, rerr := auth.ReadSettingsFile(); rerr == nil {
			telemetryEnabled = existing.TelemetryEnabled
			isInternalUser = existing.IsInternalUser
		}

		settings := &auth.Settings{
			Credentials: auth.Credentials{
				ClientID:     clientID,
				ClientSecret: clientSecret,
			},
			OrganizationID:   orgID,
			Workspace:        workspace,
			TelemetryEnabled: telemetryEnabled,
			IsInternalUser:   isInternalUser,
		}
		if err := auth.WriteSettingsFile(settings); err != nil {
			outputpkg.WriteError(map[string]any{"type": "error", "message": err.Error()})
			os.Exit(1)
		}

		// Emit the configure event with the just-entered org_id. We
		// can't reuse the global tracker (built before settings.json
		// existed for a fresh install), so spin up a one-shot tracker
		// here and flush it before returning.
		mode := telemetry.ResolveMode(settings.TelemetryEnabled)
		t := telemetry.New(mode, settings.OrganizationID, Version, settings.IsInternalUser)
		t.TrackCommand(telemetry.CommandEvent{
			Command:    "configure",
			Success:    true,
			DurationMs: time.Since(start).Milliseconds(),
		})
		t.Flush()

		return outputpkg.WriteJSON(os.Stdout, map[string]string{
			"status":  "saved",
			"message": "Settings written to ~/.airbyte-agent/settings.json",
		})
	},
}

var configureShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the saved settings (with the client secret obfuscated)",
	Long: `Read ~/.airbyte-agent/settings.json and print its contents as JSON. The
client_secret is obfuscated — only the trailing characters are visible —
so the output is safe to paste into a bug report or share for debugging.

This command reads the file directly, not the runtime resolved settings.
If you have AIRBYTE_* environment variables set, they may override what's
shown here when the CLI actually makes API calls.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := auth.ReadSettingsFile()
		if err != nil {
			if os.IsNotExist(err) {
				outputpkg.WriteError(map[string]any{
					"type":    "not_found",
					"message": "settings file does not exist",
					"hint":    "run 'airbyte-agent configure' to create ~/.airbyte-agent/settings.json",
				})
				os.Exit(client.ExitNotFound)
			}
			outputpkg.WriteError(map[string]any{"type": "error", "message": err.Error()})
			os.Exit(client.ExitGeneral)
		}

		return outputpkg.WriteJSON(os.Stdout, map[string]string{
			"client_id":       settings.Credentials.ClientID,
			"client_secret":   obfuscateSecret(settings.Credentials.ClientSecret),
			"organization_id": settings.OrganizationID,
			"workspace":       settings.Workspace,
		})
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
	configureCmd.AddCommand(configureShowCmd)
	rootCmd.AddCommand(configureCmd)
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
