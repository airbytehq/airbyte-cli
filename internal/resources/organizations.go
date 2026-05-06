package resources

import (
	"context"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
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
			Run: func(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
				raw, err := c.Get(ctx, "/api/v1/internal/account/organizations", nil)
				if err != nil {
					return nil, err
				}
				return raw, nil
			},
		},
	}
}
