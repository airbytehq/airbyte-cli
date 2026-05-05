package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

const (
	defaultCredentialTimeout = 5 * time.Minute
	defaultWebAppBaseURL     = "https://cloud.airbyte.com"
)

func connectorsCreateOperation() registry.Operation {
	return registry.Operation{
		Name:        "create",
		Description: "Create a new connector",
		Schema: registry.OperationSchema{
			Description: "Create a connector from a template with interactive credential flow",
			Params: map[string]registry.ParamSchema{
				"template_id":    {Type: "string", Required: false, Description: "Source template ID"},
				"template_name":  {Type: "string", Required: false, Description: "Source template name (alternative to template_id)"},
				"workspace": {Type: "string", Required: true, Description: "Workspace name"},
			},
		},
		Hooks: registry.OperationHooks{
			Interactive: connectorsCreateInteractive,
		},
	}
}

func connectorsCreateInteractive(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	workspaceName, _ := params["workspace"].(string)

	templateID, err := resolveTemplateID(ctx, c, params)
	if err != nil {
		return nil, err
	}

	templateRaw, err := c.Get(ctx, fmt.Sprintf("/api/v1/integrations/templates/sources/%s", templateID), nil)
	if err != nil {
		return nil, err
	}

	var template struct {
		SourceDefinitionID string `json:"source_definition_id"`
	}
	if err := json.Unmarshal(templateRaw, &template); err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	workspaceID, err := resolveWorkspaceID(ctx, c, workspaceName)
	if err != nil {
		return nil, err
	}

	widgetTokenRaw, err := c.Post(ctx, "/api/v1/account/applications/widget-token", map[string]any{
		"customer_name":                          workspaceName,
		"allowed_origin":                         webAppBaseURL(),
		"selected_source_template_tags":          []string{},
		"selected_source_template_tags_mode":     "any",
		"selected_connection_template_tags":      []string{},
		"selected_connection_template_tags_mode": "any",
	})
	if err != nil {
		return nil, err
	}

	var widgetToken struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(widgetTokenRaw, &widgetToken); err != nil {
		return nil, fmt.Errorf("parsing widget token: %w", err)
	}

	sessionRaw, err := c.Post(ctx, "/api/v1/internal/mcp_oauth/sessions", map[string]any{
		"source_definition_id": template.SourceDefinitionID,
		"workspace_id":         workspaceID,
		"source_template_id":   templateID,
	})
	if err != nil {
		return nil, err
	}

	var session struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(sessionRaw, &session); err != nil {
		return nil, fmt.Errorf("parsing session: %w", err)
	}

	credURL := fmt.Sprintf("%s/embedded-widget/credentials?session_id=%s&token=%s",
		webAppBaseURL(), session.SessionID, widgetToken.Token)

	startResult := map[string]string{
		"credentials_url": credURL,
		"session_id":      session.SessionID,
		"message":         "Opening browser to complete credential setup. Waiting for credentials...",
	}
	startJSON, _ := json.MarshalIndent(startResult, "", "  ")
	fmt.Fprintln(os.Stderr, string(startJSON))

	openBrowser(credURL)

	timeout := credentialTimeout()
	deadline := time.Now().Add(timeout)

	delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	attempt := 0

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		delay := 16 * time.Second
		if attempt < len(delays) {
			delay = delays[attempt]
		}
		attempt++

		remaining := time.Until(deadline)
		if delay > remaining {
			delay = remaining
		}
		if delay <= 0 {
			break
		}

		time.Sleep(delay)

		pollURL := fmt.Sprintf("/api/v1/internal/mcp_oauth/sessions/%s", session.SessionID)
		pollRaw, err := c.Get(ctx, pollURL, nil)
		if err != nil {
			continue
		}

		var pollResult struct {
			Status      string          `json:"status"`
			Credentials json.RawMessage `json:"credentials"`
		}
		if err := json.Unmarshal(pollRaw, &pollResult); err != nil {
			continue
		}

		if pollResult.Status == "completed" {
			return createConnectorWithCredentials(ctx, c, templateID, workspaceName, templateRaw, pollResult.Credentials)
		}

		if pollResult.Status == "failed" {
			return nil, &client.APIError{
				Type:       "credential_error",
				Message:    "credential flow failed",
				StatusCode: 400,
			}
		}
	}

	return map[string]string{
		"error":      "timeout",
		"message":    fmt.Sprintf("Credential flow timed out after %s", timeout),
		"session_id": session.SessionID,
	}, nil
}

func resolveTemplateID(ctx context.Context, c *client.Client, params map[string]any) (string, error) {
	if id, ok := params["template_id"].(string); ok && id != "" {
		return id, nil
	}
	name, ok := params["template_name"].(string)
	if !ok || name == "" {
		return "", client.NewValidationError(
			"either template_id or template_name is required",
			"run 'airbyte connectors list-available' to see available templates",
		)
	}

	raw, err := c.Get(ctx, "/api/v1/integrations/templates/sources/global", nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parsing templates: %w", err)
	}

	for _, t := range resp.Data {
		if equalFold(t.Name, name) {
			return t.ID, nil
		}
	}

	return "", client.NewNotFoundError(
		fmt.Sprintf("template %q not found", name),
		"run 'airbyte connectors list-available' to see available template names",
	)
}

func resolveWorkspaceID(ctx context.Context, c *client.Client, name string) (string, error) {
	raw, err := c.Get(ctx, "/api/v1/workspaces", map[string]string{
		"name_contains": name,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		Data []struct {
			ID   string `json:"workspace_id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parsing workspaces: %w", err)
	}

	for _, ws := range resp.Data {
		if equalFold(ws.Name, name) {
			return ws.ID, nil
		}
	}

	return "", client.NewNotFoundError(
		fmt.Sprintf("workspace %q not found", name),
		"run 'airbyte workspaces list' to see available workspace names",
	)
}

func createConnectorWithCredentials(ctx context.Context, c *client.Client, templateID, workspaceName string, templateRaw, credentials json.RawMessage) (any, error) {
	var templateBody map[string]any
	if err := json.Unmarshal(templateRaw, &templateBody); err != nil {
		return nil, fmt.Errorf("parsing template for connector creation: %w", err)
	}

	var creds map[string]any
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	body := map[string]any{
		"source_template_id": templateID,
		"customer_name":      workspaceName,
		"credentials":        creds,
	}

	raw, err := c.Post(ctx, "/api/v1/integrations/connectors", body)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func equalFold(a, b string) bool {
	return len(a) == len(b) && foldEqual(a, b)
}

func foldEqual(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func webAppBaseURL() string {
	if v := os.Getenv("AIRBYTE_WEBAPP_URL"); v != "" {
		return v
	}
	return defaultWebAppBaseURL
}

func credentialTimeout() time.Duration {
	if v := os.Getenv("AIRBYTE_CREDENTIAL_TIMEOUT"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultCredentialTimeout
}

var openBrowserFunc = openBrowserDefault

func openBrowser(url string) {
	openBrowserFunc(url)
}

func openBrowserDefault(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
