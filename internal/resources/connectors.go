package resources

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/airbytehq/airbyte-agent-cli/internal/registry"
)

type connectorsResource struct{}

func (cr *connectorsResource) Name() string        { return "connectors" }
func (cr *connectorsResource) Description() string { return "Create, manage, and execute connectors" }
func (cr *connectorsResource) Operations() []registry.Operation {
	return []registry.Operation{
		connectorsCreateOperation(),
		{
			Name:        "list",
			Description: "List connectors in a workspace",
			Schema: registry.OperationSchema{
				Description: "List all connectors for a workspace. If 'workspace' is omitted, falls back to 'default'.",
				Params: map[string]registry.ParamSchema{
					"workspace": {Type: "string", Required: false, Description: "Workspace name (defaults to 'default' when omitted)"},
				},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/integrations/connectors", Method: "GET"},
			Run:     connectorsList,
		},
		{
			Name:        "list-available",
			Description: "List available connector templates",
			Schema: registry.OperationSchema{
				Description: "List all available source connector templates",
				Params:      map[string]registry.ParamSchema{},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/integrations/templates/sources", Method: "GET"},
			Run:     connectorsListAvailable,
		},
		{
			Name:        "describe",
			Description: "Describe a connector's schema",
			Schema: registry.OperationSchema{
				Description: "Get connector details and schema description",
				Params: map[string]registry.ParamSchema{
					"name":      {Type: "string", Required: false, Description: "Connector name (requires workspace)"},
					"workspace": {Type: "string", Required: false, Description: "Workspace name (defaults to 'default' when used with name)"},
					"id":        {Type: "string", Required: false, Description: "Connector ID (alternative to name)"},
				},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/integrations/connectors/{id}", Method: "GET"},
			Run:     connectorsDescribe,
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
					"skip_truncation": {Type: "boolean", Required: false, Description: "Disable automatic truncation of long text fields in list/search responses"},
				},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/integrations/connectors/{id}/execute", Method: "POST"},
			Run:     connectorsExecute,
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
					"workspace": {Type: "string", Required: false, Description: "Workspace name (defaults to 'default' when used with name)"},
					"id":        {Type: "string", Required: false, Description: "Connector ID (alternative to name)"},
				},
			},
			SpecRef: registry.SpecRef{Path: "/api/v1/integrations/connectors/{id}", Method: "DELETE"},
			Run:     connectorsDelete,
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
			"run 'airbyte-agent connectors list --json '{\"workspace\": \"...\"}'' to find connector names, or use --id with a connector ID",
		)
	}

	if hasID {
		return params, nil
	}

	workspaceName := applyDefaultWorkspace(c, params)

	raw, err := c.Get(ctx, "/api/v1/integrations/connectors", map[string]string{
		"workspace_name": workspaceName,
	})
	if err != nil {
		return nil, err
	}

	var resp connectorLookupResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parsing connectors list: %w", err)
	}

	// Accept matches against the connector instance name, the template's
	// display name, OR the template's slug — users may type any of these.
	// Deduplicate so a single connector matched by multiple fields counts
	// once.
	seen := map[string]bool{}
	var matches []string
	for _, conn := range resp.Data {
		candidates := []string{
			conn.Name,
			conn.SummarizedSourceTemplate.Name,
			conn.SummarizedSourceTemplate.ConnectorName,
		}
		for _, candidate := range candidates {
			if candidate != "" && strings.EqualFold(candidate, name) {
				if !seen[conn.ID] {
					matches = append(matches, conn.ID)
					seen[conn.ID] = true
				}
				break
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, client.NewNotFoundError(
			fmt.Sprintf("connector %q not found in workspace %q", name, workspaceName),
			fmt.Sprintf("run 'airbyte-agent connectors list --json '{\"workspace\": \"%s\"}'' to see available connectors", workspaceName),
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

// fallbackWorkspaceName is the last-resort default applied when neither
// the caller nor the user's settings.json supply a workspace. It matches
// the API's own default-workspace convention for new accounts.
const fallbackWorkspaceName = "default"

// statusWriter is the stream where the connectors resource prints user-facing
// status messages (e.g. the workspace fallback notice). Tests override it to
// capture output without touching os.Stderr.
var statusWriter io.Writer = os.Stderr

// applyDefaultWorkspace resolves params["workspace"], falling back to the
// user's configured default (from ~/.airbyte-agent/settings.json, exposed on the
// client) and ultimately to the literal "default" if neither is set. When
// the fallback engages, a JSON notice is printed to stderr so users can see
// which workspace was actually used.
func applyDefaultWorkspace(c *client.Client, params map[string]any) string {
	name, _ := params["workspace"].(string)
	if name != "" {
		return name
	}
	resolved := configuredDefaultWorkspace(c)
	notice, _ := json.Marshal(map[string]string{
		"message":   fmt.Sprintf("no workspace provided; falling back to %q", resolved),
		"workspace": resolved,
	})
	fmt.Fprintln(statusWriter, string(notice))
	params["workspace"] = resolved
	return resolved
}

func configuredDefaultWorkspace(c *client.Client) string {
	if c != nil {
		if name := c.DefaultWorkspace(); name != "" {
			return name
		}
	}
	return fallbackWorkspaceName
}

func connectorsList(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	workspaceName := applyDefaultWorkspace(c, params)
	raw, err := c.Get(ctx, "/api/v1/integrations/connectors", map[string]string{
		"workspace_name": workspaceName,
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
	path := connectorPath(id)
	execPath := path + "/execute"

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

	if describeResult.err != nil {
		return nil, describeResult.err
	}

	var schema any
	if err := json.Unmarshal(describeResult.data, &schema); err != nil {
		return nil, fmt.Errorf("parsing connector schema: %w", err)
	}
	connector["schema"] = schema

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
	if st, ok := params["skip_truncation"]; ok {
		body["skip_truncation"] = st
	}

	execPath := connectorPath(id) + "/execute"
	raw, err := c.Post(ctx, execPath, body)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func connectorsDelete(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	id, _ := params["id"].(string)

	if !c.AllowDestructive() {
		if err := confirmDestructive(deletePromptFor(params)); err != nil {
			return nil, err
		}
	}

	path := connectorPath(id)
	raw, err := c.Delete(ctx, path)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// deletePromptFor crafts the user-facing description of the connector
// being deleted. Includes name + workspace when available (more readable
// than a bare UUID) and always includes the ID as the authoritative
// identifier.
func deletePromptFor(params map[string]any) string {
	id, _ := params["id"].(string)
	name, _ := params["name"].(string)
	workspace, _ := params["workspace"].(string)

	switch {
	case name != "" && workspace != "":
		return fmt.Sprintf("Delete connector %q (id %s) from workspace %q?", name, id, workspace)
	case name != "":
		return fmt.Sprintf("Delete connector %q (id %s)?", name, id)
	default:
		return fmt.Sprintf("Delete connector with id %s?", id)
	}
}

// confirmReader is the source of confirmation input. Tests swap this out
// to inject canned responses.
var confirmReader io.Reader = os.Stdin

// confirmWriter is where the confirmation prompt is printed. Defaults to
// stderr so stdout stays clean for JSON output.
var confirmWriter io.Writer = os.Stderr

// isTerminal reports whether stdin is connected to a TTY. Tests override
// it to simulate piped input.
var isTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// confirmDestructive is the package-level confirmation hook. It is a
// variable (not a function) so tests can replace the prompt+read logic
// wholesale without monkey-patching multiple primitives. The default
// implementation reads a line from confirmReader after writing prompt to
// confirmWriter, and only accepts an exact "yes" (case-insensitive,
// trimmed) — anything else is treated as a cancel.
var confirmDestructive = func(prompt string) error {
	if !isTerminal() {
		return client.NewValidationError(
			"destructive action requires confirmation but no TTY is available",
			"set \"allow_destructive\": true in ~/.airbyte-agent/settings.json (or AIRBYTE_ALLOW_DESTRUCTIVE=true) to allow non-interactive destructive operations",
		)
	}

	fmt.Fprintf(confirmWriter, "%s Type 'yes' to confirm: ", prompt)
	line, err := bufio.NewReader(confirmReader).ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(line)) != "yes" {
		return client.NewValidationError(
			"destructive action cancelled by user",
			"re-run the command and type 'yes' to confirm, or set \"allow_destructive\": true in settings.json",
		)
	}
	return nil
}

func connectorPath(id string) string {
	return "/api/v1/integrations/connectors/" + url.PathEscape(id)
}
