package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/airbytehq/airbyte-agent-cli/internal/auth"
	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/airbytehq/airbyte-agent-cli/internal/registry"
)

type organizationsResource struct{}

func (o *organizationsResource) Name() string        { return "organizations" }
func (o *organizationsResource) Description() string { return "Manage organizations" }
func (o *organizationsResource) Operations() []registry.Operation {
	return []registry.Operation{
		{
			Name:        "list",
			Description: "List organizations",
			Schema: registry.OperationSchema{
				Description: "List all organizations for the current account",
				Params:      map[string]registry.ParamSchema{},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/internal/account/organizations", Method: "GET"},
			Run:     listOrganizations,
		},
		{
			Name:        "use",
			Description: "Set the default organization stored in ~/.airbyte-agent/settings.json",
			Schema: registry.OperationSchema{
				Description: "Update settings.json so subsequent commands scope to this organization. The organization must exist on the authenticated account (the command verifies via the API before writing).",
				Params: map[string]registry.ParamSchema{
					"id": {Type: "string", Required: true, Description: "Organization UUID to use as the default"},
				},
			},
			Run: useOrganization,
		},
	}
}

func listOrganizations(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	raw, err := c.Get(ctx, "/api/v1/internal/account/organizations", nil)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// useOrganization updates Settings.OrganizationID in ~/.airbyte-agent/settings.json.
// It verifies the organization exists on the authenticated account first,
// so users can't save a typo'd or unauthorized UUID. The ID is persisted
// exactly as it appears in the API response (UUIDs are commonly written
// in either case; we accept either and normalize).
func useOrganization(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, client.NewValidationError(
			"organization id is required",
			`pass --json '{"id": "<uuid>"}'`,
		)
	}

	canonical, err := lookupOrganizationID(ctx, c, id)
	if err != nil {
		return nil, err
	}

	settings, err := auth.ReadSettingsFile()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, client.NewNotFoundError(
				"settings file does not exist",
				"run 'airbyte-agent login' first to create ~/.airbyte-agent/settings.json",
			)
		}
		return nil, fmt.Errorf("reading settings: %w", err)
	}

	settings.OrganizationID = canonical
	if err := auth.WriteSettingsFile(settings); err != nil {
		return nil, fmt.Errorf("writing settings: %w", err)
	}

	return map[string]any{
		"status":          "saved",
		"organization_id": canonical,
		"message":         fmt.Sprintf("default organization set to %q in ~/.airbyte-agent/settings.json", canonical),
	}, nil
}

// lookupOrganizationID confirms an organization with the given UUID
// exists on the authenticated account and returns the ID as it appears
// in the API response. Match is case-insensitive (UUIDs are commonly
// written in either case). The response is parsed defensively because
// the public-API gateway and the sonar endpoint use different envelope
// keys (`data` vs `organizations`).
func lookupOrganizationID(ctx context.Context, c *client.Client, id string) (string, error) {
	raw, err := c.Get(ctx, "/api/v1/internal/account/organizations", nil)
	if err != nil {
		return "", err
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", fmt.Errorf("parsing organizations: %w", err)
	}

	var items []json.RawMessage
	for _, key := range []string{"data", "organizations"} {
		if v, ok := envelope[key]; ok {
			if err := json.Unmarshal(v, &items); err != nil {
				return "", fmt.Errorf("parsing organizations: %w", err)
			}
			break
		}
	}

	for _, item := range items {
		var org struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(item, &org); err != nil {
			continue
		}
		if strings.EqualFold(org.ID, id) {
			return org.ID, nil
		}
	}

	return "", client.NewNotFoundError(
		fmt.Sprintf("organization %q not found", id),
		"run 'airbyte-agent organizations list' to see available organizations",
	)
}
