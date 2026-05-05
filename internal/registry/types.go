package registry

import (
	"context"

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
	Run         func(ctx context.Context, client *client.Client, params map[string]any) (any, error)
	Hooks       OperationHooks
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
