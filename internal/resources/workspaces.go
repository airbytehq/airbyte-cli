package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

type workspacesResource struct{}

func (w *workspacesResource) Name() string        { return "workspaces" }
func (w *workspacesResource) Description() string { return "Manage workspaces" }
func (w *workspacesResource) Operations() []registry.Operation {
	return []registry.Operation{
		{
			Name:        "list",
			Description: "List workspaces",
			Schema: registry.OperationSchema{
				Description: "List all workspaces with cursor pagination",
				Params: map[string]registry.ParamSchema{
					"name_contains": {Type: "string", Required: false, Description: "Filter by name substring"},
					"status":        {Type: "string", Required: false, Description: "Filter by status"},
					"limit":         {Type: "integer", Required: false, Description: "Max results per page"},
				},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/workspaces", Method: "GET"},
			Run:     listWorkspaces,
		},
	}
}

func listWorkspaces(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	qp := make(map[string]string)
	if v, ok := params["name_contains"].(string); ok && v != "" {
		qp["name_contains"] = v
	}
	if v, ok := params["status"].(string); ok && v != "" {
		qp["status"] = v
	}
	if v, ok := params["limit"]; ok {
		qp["limit"] = fmt.Sprintf("%v", v)
	}

	var allData []json.RawMessage

	raw, err := c.Get(ctx, "/api/v1/workspaces", qp)
	if err != nil {
		return nil, err
	}

	for {
		var page struct {
			Data []json.RawMessage `json:"data"`
			Next *string           `json:"next"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return raw, nil
		}

		allData = append(allData, page.Data...)

		if page.Next == nil || *page.Next == "" {
			break
		}

		raw, err = c.GetURL(ctx, *page.Next)
		if err != nil {
			return nil, err
		}
	}

	return map[string]any{"data": allData}, nil
}
