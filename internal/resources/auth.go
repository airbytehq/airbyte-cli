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
				Description: "Prompt for client credentials and save them to ~/.airbyte/credentials",
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

	fmt.Fprint(os.Stderr, "Client ID: ")
	clientID, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading client ID: %w", err)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, &client.APIError{
			Type:       "validation_error",
			Message:    "client ID is required",
			StatusCode: 400,
		}
	}

	fmt.Fprint(os.Stderr, "Client Secret: ")
	clientSecret, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading client secret: %w", err)
	}
	clientSecret = strings.TrimSpace(clientSecret)
	if clientSecret == "" {
		return nil, &client.APIError{
			Type:       "validation_error",
			Message:    "client secret is required",
			StatusCode: 400,
		}
	}

	fmt.Fprint(os.Stderr, "Organization ID (leave blank to skip): ")
	orgID, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading organization ID: %w", err)
	}
	orgID = strings.TrimSpace(orgID)

	creds := &auth.Credentials{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		OrganizationID: orgID,
	}

	if err := auth.WriteCredentialsFile(creds); err != nil {
		return nil, fmt.Errorf("saving credentials: %w", err)
	}

	result := map[string]string{
		"status":  "saved",
		"message": "Credentials written to ~/.airbyte/credentials",
	}
	return result, nil
}
