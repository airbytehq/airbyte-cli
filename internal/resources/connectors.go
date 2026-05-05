package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

type connectorsResource struct{}

func (cr *connectorsResource) Name() string        { return "connectors" }
func (cr *connectorsResource) Description() string { return "Manage connectors" }
func (cr *connectorsResource) Operations() []registry.Operation {
	return []registry.Operation{
		connectorsCreateOperation(),
		{
			Name:        "list",
			Description: "List connectors in a workspace",
			Schema: registry.OperationSchema{
				Description: "List all connectors for a workspace",
				Params: map[string]registry.ParamSchema{
					"workspace": {Type: "string", Required: true, Description: "Workspace name"},
				},
			},
			Run: connectorsList,
		},
		{
			Name:        "list-available",
			Description: "List available connector templates",
			Schema: registry.OperationSchema{
				Description: "List all available source connector templates",
				Params:      map[string]registry.ParamSchema{},
			},
			Run: connectorsListAvailable,
		},
		{
			Name:        "describe",
			Description: "Describe a connector's schema",
			Schema: registry.OperationSchema{
				Description: "Get connector details and schema description",
				Params: map[string]registry.ParamSchema{
					"name":      {Type: "string", Required: false, Description: "Connector name (requires workspace)"},
					"workspace": {Type: "string", Required: false, Description: "Workspace name (required when using name)"},
					"id":        {Type: "string", Required: false, Description: "Connector ID (alternative to name)"},
				},
			},
			Run: connectorsDescribe,
			Hooks: registry.OperationHooks{
				PreRun: resolveConnectorID,
			},
		},
		{
			Name:        "execute",
			Description: "Execute a connector action",
			Schema: registry.OperationSchema{
				Description: "Execute an action on a connector",
				Params: map[string]registry.ParamSchema{
					"name":           {Type: "string", Required: false, Description: "Connector name (requires workspace)"},
					"workspace":      {Type: "string", Required: false, Description: "Workspace name (required when using name)"},
					"id":             {Type: "string", Required: false, Description: "Connector ID (alternative to name)"},
					"entity":         {Type: "string", Required: true, Description: "Entity name"},
					"action":         {Type: "string", Required: true, Description: "Action name"},
					"params":         {Type: "object", Required: false, Description: "Action parameters"},
					"select_fields":  {Type: "array", Required: false, Description: "Fields to include in response"},
					"exclude_fields": {Type: "array", Required: false, Description: "Fields to exclude from response"},
				},
			},
			Run: connectorsExecute,
			Hooks: registry.OperationHooks{
				PreRun: resolveConnectorID,
			},
		},
		{
			Name:        "delete",
			Description: "Delete a connector",
			Schema: registry.OperationSchema{
				Description: "Delete a connector by name or ID",
				Params: map[string]registry.ParamSchema{
					"name":      {Type: "string", Required: false, Description: "Connector name (requires workspace)"},
					"workspace": {Type: "string", Required: false, Description: "Workspace name (required when using name)"},
					"id":        {Type: "string", Required: false, Description: "Connector ID (alternative to name)"},
				},
			},
			Run: connectorsDelete,
			Hooks: registry.OperationHooks{
				PreRun: resolveConnectorID,
			},
		},
	}
}

func resolveConnectorID(ctx context.Context, c *client.Client, params map[string]any) (map[string]any, error) {
	id, hasID := params["id"].(string)
	name, hasName := params["name"].(string)
	hasID = hasID && id != ""
	hasName = hasName && name != ""

	if hasID && hasName {
		return nil, client.NewValidationError(
			"provide either 'id' or 'name', not both",
			"provide only one of 'id' or 'name'",
		)
	}
	if !hasID && !hasName {
		return nil, client.NewValidationError(
			"either 'name' + 'workspace' or 'id' is required",
			"run 'airbyte connectors list --json '{\"workspace\": \"...\"}'' to find connector names, or use --id with a connector ID",
		)
	}

	if hasID {
		return params, nil
	}

	workspaceName, _ := params["workspace"].(string)
	if workspaceName == "" {
		return nil, client.NewValidationError(
			"workspace is required when using name",
			"run 'airbyte workspaces list' to find workspace names",
		)
	}

	raw, err := c.Get(ctx, "/api/v1/integrations/connectors", map[string]string{
		"customer_name": workspaceName,
	})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parsing connectors list: %w", err)
	}

	var matches []string
	for _, conn := range resp.Data {
		if strings.EqualFold(conn.Name, name) {
			matches = append(matches, conn.ID)
		}
	}

	switch len(matches) {
	case 0:
		return nil, client.NewNotFoundError(
			fmt.Sprintf("connector %q not found in workspace %q", name, workspaceName),
			fmt.Sprintf("run 'airbyte connectors list --json '{\"workspace\": \"%s\"}'' to see available connectors", workspaceName),
		)
	case 1:
		params["id"] = matches[0]
		return params, nil
	default:
		return nil, client.NewValidationError(
			fmt.Sprintf("ambiguous: %d connectors named %q in workspace %q", len(matches), name, workspaceName),
			"use 'id' instead to target a specific connector",
		)
	}
}

func connectorsList(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	workspaceName, _ := params["workspace"].(string)
	raw, err := c.Get(ctx, "/api/v1/integrations/connectors", map[string]string{
		"customer_name": workspaceName,
	})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func connectorsListAvailable(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	raw, err := c.Get(ctx, "/api/v1/integrations/templates/sources/global", nil)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func connectorsDescribe(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	id, _ := params["id"].(string)
	path := fmt.Sprintf("/api/v1/integrations/connectors/%s", id)
	execPath := fmt.Sprintf("/api/v1/integrations/connectors/%s/execute", id)

	type result struct {
		data json.RawMessage
		err  error
	}

	var wg sync.WaitGroup
	getCh := make(chan result, 1)
	describeCh := make(chan result, 1)

	wg.Add(2)
	go func() {
		defer wg.Done()
		raw, err := c.Get(ctx, path, nil)
		getCh <- result{raw, err}
	}()
	go func() {
		defer wg.Done()
		raw, err := c.Post(ctx, execPath, map[string]string{
			"entity": "",
			"action": "describe",
		})
		describeCh <- result{raw, err}
	}()
	wg.Wait()

	getResult := <-getCh
	describeResult := <-describeCh

	if getResult.err != nil {
		return nil, getResult.err
	}

	var connector map[string]any
	if err := json.Unmarshal(getResult.data, &connector); err != nil {
		return nil, fmt.Errorf("parsing connector: %w", err)
	}

	if describeResult.err == nil {
		var schema any
		if err := json.Unmarshal(describeResult.data, &schema); err == nil {
			connector["schema"] = schema
		}
	}

	return connector, nil
}

func connectorsExecute(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	id, _ := params["id"].(string)
	entity, _ := params["entity"].(string)
	action, _ := params["action"].(string)

	body := map[string]any{
		"entity": entity,
		"action": action,
	}
	if p, ok := params["params"]; ok {
		body["params"] = p
	}
	if sf, ok := params["select_fields"]; ok {
		body["select_fields"] = sf
	}
	if ef, ok := params["exclude_fields"]; ok {
		body["exclude_fields"] = ef
	}

	execPath := fmt.Sprintf("/api/v1/integrations/connectors/%s/execute", id)
	raw, err := c.Post(ctx, execPath, body)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func connectorsDelete(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	id, _ := params["id"].(string)
	path := fmt.Sprintf("/api/v1/integrations/connectors/%s", id)
	raw, err := c.Delete(ctx, path)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
