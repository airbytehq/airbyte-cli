package registry

import (
	"encoding/json"

	"github.com/airbytehq/airbyte-agent-cli/internal/spec"
)

// SchemaOutput is the payload returned by `--describe` and `airbyte-agent schema
// <resource> <operation>`. It exposes both the CLI-facing parameter shape
// (what the user passes via --json or flags) and the underlying OpenAPI
// route's request/response schemas (what the HTTP API accepts and returns).
//
// The CLI surface and the API surface are intentionally separate: the CLI
// applies conveniences like the workspace fallback, name/id alternation, and
// flatter parameter shapes. Agents that want to know "what bytes go on the
// wire" should consult the `api` section; agents that want to know "what
// flags can I pass" should consult `params`.
type SchemaOutput struct {
	Description string                 `json:"description,omitempty"`
	Params      map[string]ParamSchema `json:"params"`
	Examples    []string               `json:"examples,omitempty"`
	API         *APISchema             `json:"api,omitempty"`
}

// APISchema mirrors spec.RouteSchema with JSON-friendly tags. Fields that
// aren't populated for a route (e.g. RequestBody on a GET) are omitted.
type APISchema struct {
	Path        string          `json:"path"`
	Method      string          `json:"method"`
	Summary     string          `json:"summary,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	RequestBody json.RawMessage `json:"request_body,omitempty"`
	Response    json.RawMessage `json:"response,omitempty"`
}

// BuildSchemaOutput assembles the full describe payload for an operation.
// Returns the CLI-level schema unconditionally; appends the OpenAPI section
// when SpecRef is set and the route exists in the embedded spec map.
func BuildSchemaOutput(op Operation) SchemaOutput {
	out := SchemaOutput{
		Description: op.Schema.Description,
		Params:      op.Schema.Params,
		Examples:    op.Schema.Examples,
	}
	if op.SpecRef.IsZero() {
		return out
	}
	route, ok := spec.Lookup(op.SpecRef.Key())
	if !ok {
		return out
	}
	out.API = &APISchema{
		Path:        route.Path,
		Method:      route.Method,
		Summary:     route.Summary,
		Description: route.Description,
		Parameters:  route.Parameters,
		RequestBody: route.RequestBody,
		Response:    route.Response,
	}
	return out
}
