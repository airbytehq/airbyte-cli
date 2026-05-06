package resources

// Models for the connectors resource (connectors.go and connectors_create.go).
// Keep struct tags aligned with the real API field names — verified against
// api/*.json. When the API surface changes, this file is the single place to
// update.

// connectorLookupResponse is the minimal projection of GET /api/v1/integrations/connectors
// used by resolveConnectorID. The full response has many more fields; we only
// need id and name to map a user-supplied connector name to its ID.
type connectorLookupResponse struct {
	Data []connectorLookupItem `json:"data"`
}

type connectorLookupItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// templateLookupResponse mirrors the structure returned by the templates list
// endpoint used in resolveTemplateID. Same shape as connector list, different
// domain (templates vs configured connectors).
type templateLookupResponse struct {
	Data []templateLookupItem `json:"data"`
}

type templateLookupItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// templateDetail is the projection of GET /api/v1/integrations/templates/sources/{id}
// needed to drive the OAuth flow.
//
//   - ActorDefinitionID / SourceDefinitionID: canonical / legacy IDs used when
//     creating an OAuth session.
//   - OrganizationID: null for global (built-in) templates; controls whether
//     useGlobalTemplates is set on the bridge URL.
//   - Name: used as connector_type in the create request.
//   - PartialDefaultConfig: starting point for the connector's
//     replication_config. Merged with OAuth credentials before the create call.
//   - UserConfigSpec: declares the config schema; used to discover advanced_auth
//     paths and to apply defaults for required fields.
type templateDetail struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	ActorDefinitionID    string         `json:"actor_definition_id"`
	SourceDefinitionID   string         `json:"source_definition_id"`
	OrganizationID       *string        `json:"organization_id"`
	PartialDefaultConfig map[string]any `json:"partial_default_config"`
	UserConfigSpec       map[string]any `json:"user_config_spec"`
}

// widgetTokenPayload is the decoded form of the base64 widget token returned
// by POST /api/v1/account/applications/widget-token. The token is a base64
// JSON object whose widgetUrl carries query parameters (notably workspaceId)
// that the bridge URL needs.
type widgetTokenPayload struct {
	WidgetURL string `json:"widgetUrl"`
	Token     string `json:"token"`
}

// widgetTokenResponse is the projection of POST /api/v1/account/applications/widget-token.
type widgetTokenResponse struct {
	Token string `json:"token"`
}

// oauthSessionResponse is the projection of POST /api/v1/internal/mcp_oauth/sessions.
type oauthSessionResponse struct {
	SessionID string `json:"session_id"`
}

// oauthSessionStatus is the projection of GET /api/v1/internal/mcp_oauth/sessions/{id}
// used while polling. AuthPayload holds the raw OAuth tokens that need to be
// merged into the connector config (see mergeOAuthCredentials). SourceID, when
// non-empty, signals that a previous poll already created the connector — in
// that case, return early with that ID instead of creating a duplicate.
type oauthSessionStatus struct {
	Status           string         `json:"status"`
	Error            string         `json:"error"`
	SourceID         string         `json:"source_id"`
	SourceTemplateID string         `json:"source_template_id"`
	AuthPayload      map[string]any `json:"auth_payload"`
}
