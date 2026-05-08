package resources

import "github.com/airbytehq/airbyte-agents-cli/internal/registry"

func RegisterAll() {
	registry.Register(&organizationsResource{})
	registry.Register(&workspacesResource{})
	registry.Register(&connectorsResource{})
}
