package resources

import (
	"context"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

type enrollmentResource struct{}

func (e *enrollmentResource) Name() string        { return "enrollment" }
func (e *enrollmentResource) Description() string { return "Manage account enrollment" }
func (e *enrollmentResource) Operations() []registry.Operation {
	return []registry.Operation{
		{
			Name:        "status",
			Description: "Get enrollment status",
			Schema: registry.OperationSchema{
				Description: "Check account enrollment status",
				Params:      map[string]registry.ParamSchema{},
			},
			Run: func(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
				raw, err := c.Get(ctx, "/api/v1/internal/account/enrollment-status", nil)
				if err != nil {
					return nil, err
				}
				return raw, nil
			},
		},
	}
}
