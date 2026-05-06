// Package spec exposes OpenAPI request/response schemas for each CLI
// operation. Schemas are extracted at build time from api/*.json by the
// `cmd/extract-schemas` generator and embedded into the binary as a static
// map. Runtime lookup is a hash hit — no spec parsing, no $ref resolution.
package spec

//go:generate go run ../../cmd/extract-schemas

import "encoding/json"

// RouteSchema describes a single OpenAPI route. All schema fields hold
// JSON-serialized OpenAPI fragments with $refs already resolved into
// self-contained subtrees by the generator.
type RouteSchema struct {
	Path        string          `json:"path"`
	Method      string          `json:"method"`
	Summary     string          `json:"summary,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	RequestBody json.RawMessage `json:"request_body,omitempty"`
	Response    json.RawMessage `json:"response,omitempty"`
}

// Lookup returns the schema for the given method+path key (e.g. "GET /api/v1/workspaces").
// Returns ok=false if no matching route was extracted.
func Lookup(key string) (RouteSchema, bool) {
	r, ok := schemas[key]
	return r, ok
}

// All returns every extracted route schema. Useful for tests that verify
// every operation's SpecRef resolves.
func All() map[string]RouteSchema {
	out := make(map[string]RouteSchema, len(schemas))
	for k, v := range schemas {
		out[k] = v
	}
	return out
}
