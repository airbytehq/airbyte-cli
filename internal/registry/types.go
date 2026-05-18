package registry

import (
	"context"
	"strings"

	"github.com/airbytehq/airbyte-cli/internal/client"
)

type Resource interface {
	Name() string
	Description() string
	Operations() []Operation
}

type Operation struct {
	Name        string
	Description string
	Schema      OperationSchema
	SpecRef     SpecRef
	Run         func(ctx context.Context, client *client.Client, params map[string]any) (any, error)
	Hooks       OperationHooks
}

// SpecRef points an Operation at the OpenAPI route that backs it. The
// extract-schemas generator uses these to emit only the routes actually used
// by the CLI; the runtime uses them to look up request/response schemas for
// `airbyte agents schema`.
type SpecRef struct {
	Path   string
	Method string
}

// Key is the canonical map key used by the generated schema map.
func (s SpecRef) Key() string {
	return s.Method + " " + s.Path
}

// IsZero reports whether the ref is unset (operation has no OpenAPI mapping).
func (s SpecRef) IsZero() bool {
	return s.Path == "" && s.Method == ""
}

// IsInternal reports whether the ref targets a non-public API route. Internal
// routes are excluded from the embedded spec map and from the `schema`
// command's output — callers fall back to `--help` for argument details.
func (s SpecRef) IsInternal() bool {
	return strings.HasPrefix(s.Path, "/api/v1/internal/")
}

type OperationHooks struct {
	PreRun               func(ctx context.Context, client *client.Client, params map[string]any) (map[string]any, error)
	Interactive          func(ctx context.Context, client *client.Client, params map[string]any) (any, error)
	AllowUnauthenticated bool
}

type OperationSchema struct {
	Params      map[string]ParamSchema `json:"params"`
	Description string                 `json:"description"`
	Examples    []string               `json:"examples,omitempty"`
}

type ParamSchema struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}
