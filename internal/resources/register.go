package resources

import "github.com/airbytehq/airbyte-cli/internal/registry"

func RegisterAll() {
	registry.Register(&authResource{})
	registry.Register(&enrollmentResource{})
	registry.Register(&organizationsResource{})
	registry.Register(&workspacesResource{})
	registry.Register(&connectorsResource{})
	registry.Register(&skillsResource{})
}
