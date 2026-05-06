package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/airbytehq/airbyte-cli/internal/auth"
	outputpkg "github.com/airbytehq/airbyte-cli/internal/output"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure credentials and organization id interactively",
	Long: `Prompt for client_id, client_secret, and organization_id, then save them to
~/.airbyte/settings.json with 0600 permissions. Run this once on a new machine
or whenever your credentials change.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

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

		settings := &auth.Settings{
			Credentials: auth.Credentials{
				ClientID:     clientID,
				ClientSecret: clientSecret,
			},
			OrganizationID: orgID,
		}
		if err := auth.WriteSettingsFile(settings); err != nil {
			outputpkg.WriteError(map[string]any{"type": "error", "message": err.Error()})
			os.Exit(1)
		}

		return outputpkg.WriteJSON(os.Stdout, map[string]string{
			"status":  "saved",
			"message": "Settings written to ~/.airbyte/settings.json",
		})
	},
}

func init() {
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
