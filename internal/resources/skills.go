package resources

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/registry"
)

//go:embed skills/*.md
var skillsFS embed.FS

type skillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func parseSkillDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}

func listSkillEntries() ([]skillEntry, error) {
	entries, err := skillsFS.ReadDir("skills")
	if err != nil {
		return nil, fmt.Errorf("reading embedded skills: %w", err)
	}
	var skills []skillEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := skillsFS.ReadFile(filepath.Join("skills", e.Name()))
		if err != nil {
			continue
		}
		desc := parseSkillDescription(string(data))
		skills = append(skills, skillEntry{Name: name, Description: desc})
	}
	return skills, nil
}

type skillsResource struct{}

func (s *skillsResource) Name() string        { return "skills" }
func (s *skillsResource) Description() string { return "Agent skill documents" }
func (s *skillsResource) Operations() []registry.Operation {
	return []registry.Operation{
		{
			Name:        "list",
			Description: "List available skills",
			Schema: registry.OperationSchema{
				Description: "List all available agent skill documents",
				Params:      map[string]registry.ParamSchema{},
			},
			Run: func(ctx context.Context, _ *client.Client, params map[string]any) (any, error) {
				entries, err := listSkillEntries()
				if err != nil {
					return nil, err
				}
				return map[string]any{"data": entries}, nil
			},
		},
		{
			Name:        "show",
			Description: "Show a skill document",
			Schema: registry.OperationSchema{
				Description: "Show the content of a specific skill document",
				Params: map[string]registry.ParamSchema{
					"name": {Type: "string", Required: true, Description: "Skill name (e.g., connectors, workspaces, discovery, getting-started)"},
				},
			},
			Run: func(ctx context.Context, _ *client.Client, params map[string]any) (any, error) {
				name, _ := params["name"].(string)
				if name == "" {
					return nil, &client.APIError{
						Type:       "validation_error",
						Message:    "name parameter is required",
						StatusCode: 400,
					}
				}

				filename := filepath.Join("skills", name+".md")
				data, err := skillsFS.ReadFile(filename)
				if err != nil {
					return nil, &client.APIError{
						Type:       "not_found",
						Message:    fmt.Sprintf("skill %q not found", name),
						StatusCode: 404,
					}
				}

				return map[string]any{
					"name":    name,
					"content": string(data),
				}, nil
			},
		},
	}
}
