package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/output"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

const (
	defaultCredentialTimeout = 3 * time.Minute
	defaultWebAppBaseURL     = "https://app.airbyte.ai"
)

// Polling cadence for the credential flow. OAuth flows typically take
// 30s–2min in the browser, so we don't poll at all during the first
// initialCredentialPollDelay seconds, then poll on a fixed interval. Both are
// vars (not consts) so tests can override them for fast iteration.
var (
	initialCredentialPollDelay = 30 * time.Second
	credentialPollInterval     = 3 * time.Second
)

func connectorsCreateOperation() registry.Operation {
	return registry.Operation{
		Name:        "create",
		Description: "Create a new connector",
		Schema: registry.OperationSchema{
			Description: "Create a connector from a template with interactive credential flow",
			Params: map[string]registry.ParamSchema{
				"id":        {Type: "string", Required: false, Description: "Source template ID"},
				"name":      {Type: "string", Required: false, Description: "Source template name (alternative to id)"},
				"workspace": {Type: "string", Required: false, Description: "Workspace name (defaults to 'default' when omitted)"},
			},
		},
		SpecRef: registry.SpecRef{Path: "/api/v1/integrations/connectors", Method: "POST"},
		Hooks: registry.OperationHooks{
			Interactive: connectorsCreateInteractive,
		},
	}
}

func connectorsCreateInteractive(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	workspaceName := applyDefaultWorkspace(c, params)

	templateID, err := resolveTemplateID(ctx, c, params)
	if err != nil {
		return nil, err
	}

	templateRaw, err := c.Get(ctx, "/api/v1/integrations/templates/sources/"+url.PathEscape(templateID), nil)
	if err != nil {
		return nil, err
	}

	var template templateDetail
	if err := json.Unmarshal(templateRaw, &template); err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	sourceDefinitionID := template.ActorDefinitionID
	if sourceDefinitionID == "" {
		sourceDefinitionID = template.SourceDefinitionID
	}
	if sourceDefinitionID == "" {
		return nil, fmt.Errorf("template %q has no actor_definition_id or source_definition_id", templateID)
	}

	widgetTokenRaw, err := c.Post(ctx, "/api/v1/account/applications/widget-token", map[string]any{
		"workspace_name":                         workspaceName,
		"allowed_origin":                         webAppBaseURL(),
		"selected_source_template_tags":          []string{},
		"selected_source_template_tags_mode":     "any",
		"selected_connection_template_tags":      []string{},
		"selected_connection_template_tags_mode": "any",
	})
	if err != nil {
		return nil, err
	}

	var widgetToken widgetTokenResponse
	if err := json.Unmarshal(widgetTokenRaw, &widgetToken); err != nil {
		return nil, fmt.Errorf("parsing widget token: %w", err)
	}

	// The workspace_id is encoded inside the widget token's widgetUrl query
	// string, not exposed via the workspaces endpoint with the field name we
	// were assuming. Decode the token and extract it from there — this is the
	// approach the agent-engine-mcp service uses, and it avoids a round-trip.
	decoded, err := decodeWidgetToken(widgetToken.Token)
	if err != nil {
		return nil, fmt.Errorf("decoding widget token: %w", err)
	}
	workspaceID, err := extractWorkspaceID(decoded.WidgetURL)
	if err != nil {
		return nil, fmt.Errorf("extracting workspace_id from widget token: %w", err)
	}

	sessionRaw, err := c.Post(ctx, "/api/v1/internal/mcp_oauth/sessions", map[string]any{
		"source_definition_id": sourceDefinitionID,
		"workspace_id":         workspaceID,
		"source_template_id":   templateID,
	})
	if err != nil {
		return nil, err
	}

	var session oauthSessionResponse
	if err := json.Unmarshal(sessionRaw, &session); err != nil {
		return nil, fmt.Errorf("parsing session: %w", err)
	}

	useGlobalTemplates := template.OrganizationID == nil
	credURL, err := credentialURL(webAppBaseURL(), credentialURLParams{
		WidgetTokenB64:     widgetToken.Token,
		SessionID:          session.SessionID,
		TemplateID:         templateID,
		UseGlobalTemplates: useGlobalTemplates,
	})
	if err != nil {
		return nil, err
	}

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

	// Render a TTY spinner while we wait. No-op when stderr isn't a terminal
	// (piped, MCP, CI), so machine-readable streams stay clean.
	spinner := output.NewSpinner(os.Stderr, timeout)
	spinner.SetLabel("Waiting for credentials in browser")
	spinner.Start(ctx)
	defer spinner.Stop()

	// Wait initialCredentialPollDelay before the first poll — the user is
	// almost certainly still in the browser, and polling sooner just burns
	// API calls. After that, poll on a fixed interval until deadline.
	if err := waitFor(ctx, initialCredentialPollDelay, deadline); err != nil {
		return nil, err
	}

	spinner.SetLabel("Polling for completion")

	pollURL := "/api/v1/internal/mcp_oauth/sessions/" + url.PathEscape(session.SessionID)
	for time.Now().Before(deadline) {
		pollRaw, err := c.Get(ctx, pollURL, nil)
		if err == nil {
			var pollResult oauthSessionStatus
			if err := json.Unmarshal(pollRaw, &pollResult); err == nil {
				switch pollResult.Status {
				case "completed":
					return finalizeConnector(ctx, c, workspaceName, session.SessionID, &pollResult)
				case "error", "failed":
					msg := pollResult.Error
					if msg == "" {
						msg = "credential flow failed"
					}
					return nil, &client.APIError{
						Type:       "credential_error",
						Message:    msg,
						StatusCode: 400,
					}
				}
			}
		}

		if err := waitFor(ctx, credentialPollInterval, deadline); err != nil {
			return nil, err
		}
	}

	return map[string]string{
		"error":      "timeout",
		"message":    fmt.Sprintf("Credential flow timed out after %s", timeout),
		"session_id": session.SessionID,
	}, nil
}

func resolveTemplateID(ctx context.Context, c *client.Client, params map[string]any) (string, error) {
	if id, ok := params["id"].(string); ok && id != "" {
		return id, nil
	}
	name, ok := params["name"].(string)
	if !ok || name == "" {
		return "", client.NewValidationError(
			"either 'id' or 'name' is required",
			"run 'airbyte connectors list-available' to see available templates",
		)
	}

	raw, err := c.Get(ctx, "/api/v1/integrations/templates/sources/global", nil)
	if err != nil {
		return "", err
	}

	var resp templateLookupResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parsing templates: %w", err)
	}

	for _, t := range resp.Data {
		if strings.EqualFold(t.Name, name) {
			return t.ID, nil
		}
	}

	return "", client.NewNotFoundError(
		fmt.Sprintf("template %q not found", name),
		"run 'airbyte connectors list-available' to see available template names",
	)
}

// waitFor sleeps for d, but returns early if ctx is canceled or the
// deadline is reached. Returns ctx.Err() on cancellation; nil otherwise
// (including when the wait is clamped against the deadline).
func waitFor(ctx context.Context, d time.Duration, deadline time.Time) error {
	if remaining := time.Until(deadline); d > remaining {
		d = remaining
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// decodeWidgetToken decodes the base64 widget token returned by the
// widget-token endpoint into a typed payload. Mirrors the agent-engine-mcp
// reference implementation.
func decodeWidgetToken(b64 string) (widgetTokenPayload, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return widgetTokenPayload{}, fmt.Errorf("base64 decode: %w", err)
	}
	var payload widgetTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return widgetTokenPayload{}, fmt.Errorf("json parse: %w", err)
	}
	return payload, nil
}

// extractWorkspaceID pulls the workspaceId query parameter out of a widget
// URL. The widget URL is the embeddable credential page URL handed to us by
// the widget-token endpoint, with workspaceId already encoded in its query
// string by the server — that's the canonical way to know which workspace
// we're operating in.
func extractWorkspaceID(widgetURL string) (string, error) {
	if widgetURL == "" {
		return "", fmt.Errorf("widget url is empty")
	}
	u, err := url.Parse(widgetURL)
	if err != nil {
		return "", fmt.Errorf("parsing widget url: %w", err)
	}
	id := u.Query().Get("workspaceId")
	if id == "" {
		return "", fmt.Errorf("no workspaceId in widget url query")
	}
	return id, nil
}

// finalizeConnector handles a completed OAuth session: it fetches the
// template's config spec, merges the auth payload into the connector
// configuration via mergeOAuthCredentials, creates the connector, and PATCHes
// the session so it can't be re-consumed. If the session already references a
// previously-created connector (source_id), returns that one instead of
// creating a duplicate.
func finalizeConnector(ctx context.Context, c *client.Client, workspaceName, sessionID string, sess *oauthSessionStatus) (any, error) {
	if sess.SourceID != "" {
		return map[string]any{
			"id":         sess.SourceID,
			"status":     "completed",
			"message":    "Connector was created for this session",
			"session_id": sessionID,
		}, nil
	}

	if sess.SourceTemplateID == "" {
		return nil, &client.APIError{
			Type:       "credential_error",
			Message:    "OAuth session is missing source_template_id; restart the credential flow",
			StatusCode: 400,
		}
	}

	templateRaw, err := c.Get(ctx, "/api/v1/integrations/templates/sources/"+url.PathEscape(sess.SourceTemplateID), nil)
	if err != nil {
		return nil, fmt.Errorf("fetching template after completion: %w", err)
	}
	var template templateDetail
	if err := json.Unmarshal(templateRaw, &template); err != nil {
		return nil, fmt.Errorf("parsing template after completion: %w", err)
	}

	authPayload := sess.AuthPayload
	if authPayload == nil {
		authPayload = map[string]any{}
	}
	replicationConfig := mergeOAuthCredentials(template.UserConfigSpec, template.PartialDefaultConfig, authPayload)

	body := map[string]any{
		"source_template_id": sess.SourceTemplateID,
		"workspace_name":     workspaceName,
	}
	if template.Name != "" {
		body["connector_type"] = template.Name
		body["name"] = template.Name
	}
	if len(replicationConfig) > 0 {
		body["replication_config"] = replicationConfig
	}

	raw, err := c.Post(ctx, "/api/v1/integrations/connectors", body)
	if err != nil {
		return nil, err
	}

	// Best-effort: mark the session consumed so a re-poll doesn't try to
	// create the connector again. Failure is non-fatal.
	if id := extractConnectorID(raw); id != "" {
		_, _ = c.Patch(ctx, "/api/v1/internal/mcp_oauth/sessions/"+url.PathEscape(sessionID), map[string]any{
			"source_id": id,
		})
	}

	return raw, nil
}

// extractConnectorID returns the "id" string from a connector-create response
// body, or "" if the response shape doesn't match.
func extractConnectorID(raw json.RawMessage) string {
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ""
	}
	return resp.ID
}

// credentialURLParams collects the inputs to credentialURL. WidgetTokenB64
// is the raw (still base64-encoded) widget token from the widget-token
// endpoint, NOT the inner decoded token field.
type credentialURLParams struct {
	WidgetTokenB64     string
	SessionID          string
	TemplateID         string
	UseGlobalTemplates bool
}

// credentialURL assembles the bridge URL the user opens in their browser to
// enter credentials. The path and query parameters mirror the
// agent-engine-mcp implementation; older paths like /embedded-widget/credentials
// are no longer accepted by the web app.
func credentialURL(baseURL string, p credentialURLParams) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing web app URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/widget-bridge"
	q := u.Query()
	q.Set("widget_token", p.WidgetTokenB64)
	q.Set("session", p.SessionID)
	q.Set("selectedTemplateId", p.TemplateID)
	q.Set("showEntityPicker", "true")
	if p.UseGlobalTemplates {
		q.Set("useGlobalTemplates", "true")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
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
