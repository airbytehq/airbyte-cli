package resources

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/airbytehq/airbyte-cli/internal/auth"
	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

type authResource struct{}

func (a *authResource) Name() string        { return "auth" }
func (a *authResource) Description() string { return "Manage authentication credentials" }
func (a *authResource) Operations() []registry.Operation {
	return []registry.Operation{
		{
			Name:        "login",
			Description: "Configure credentials interactively",
			Schema: registry.OperationSchema{
				Description: "Prompt for client credentials and organization id and save them to ~/.airbyte/settings.json",
				Params:      map[string]registry.ParamSchema{},
			},
			Hooks: registry.OperationHooks{
				Interactive:          authLoginInteractive,
				AllowUnauthenticated: true,
			},
		},
	}
}

func authLoginInteractive(ctx context.Context, _ *client.Client, params map[string]any) (any, error) {
	reader := bufio.NewReader(os.Stdin)

	clientID, err := promptRequired(reader, "Client ID")
	if err != nil {
		return nil, err
	}
	clientSecret, err := promptRequired(reader, "Client Secret")
	if err != nil {
		return nil, err
	}
	orgID, err := promptRequired(reader, "Organization ID")
	if err != nil {
		return nil, err
	}

	settings := &auth.Settings{
		Credentials: auth.Credentials{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		},
		OrganizationID: orgID,
	}

	if err := auth.WriteSettingsFile(settings); err != nil {
		return nil, fmt.Errorf("saving settings: %w", err)
	}

	return map[string]string{
		"status":  "saved",
		"message": "Settings written to ~/.airbyte/settings.json",
	}, nil
}

func promptRequired(reader *bufio.Reader, label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", &client.APIError{
			Type:       "validation_error",
			Message:    fmt.Sprintf("%s is required", label),
			StatusCode: 400,
		}
	}
	return value, nil
}
