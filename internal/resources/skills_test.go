package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/airbytehq/airbyte-cli/internal/client"
)

func TestSkillsList(t *testing.T) {
	res := &skillsResource{}
	if res.Name() != "skills" {
		t.Errorf("expected name 'skills', got %q", res.Name())
	}

	ops := res.Operations()
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}
	if ops[0].Name != "list" {
		t.Errorf("expected first operation 'list', got %q", ops[0].Name)
	}

	result, err := ops[0].Run(context.Background(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	data, ok := resultMap["data"].([]skillEntry)
	if !ok {
		t.Fatalf("expected []skillEntry, got %T", resultMap["data"])
	}

	expectedSkills := map[string]bool{
		"connectors":      false,
		"discovery":       false,
		"getting-started": false,
		"workspaces":      false,
	}

	for _, entry := range data {
		if _, exists := expectedSkills[entry.Name]; exists {
			expectedSkills[entry.Name] = true
		}
		if entry.Description == "" {
			t.Errorf("skill %q has empty description", entry.Name)
		}
	}

	for name, found := range expectedSkills {
		if !found {
			t.Errorf("expected skill %q not found in list", name)
		}
	}
}

func TestSkillsShowConnectors(t *testing.T) {
	res := &skillsResource{}
	ops := res.Operations()
	showOp := ops[1]
	if showOp.Name != "show" {
		t.Fatalf("expected second operation 'show', got %q", showOp.Name)
	}

	result, err := showOp.Run(context.Background(), nil, map[string]any{"name": "connectors"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if resultMap["name"] != "connectors" {
		t.Errorf("expected name 'connectors', got %v", resultMap["name"])
	}

	content, ok := resultMap["content"].(string)
	if !ok || content == "" {
		t.Fatal("expected non-empty content string")
	}

	if !strings.Contains(content, "# Connectors") {
		t.Error("expected content to contain '# Connectors' heading")
	}
}

func TestSkillsShowNonexistent(t *testing.T) {
	res := &skillsResource{}
	ops := res.Operations()
	showOp := ops[1]

	_, err := showOp.Run(context.Background(), nil, map[string]any{"name": "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}

	if apiErr.Type != "not_found" {
		t.Errorf("expected error type 'not_found', got %q", apiErr.Type)
	}

	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}

	if apiErr.ExitCode() != client.ExitNotFound {
		t.Errorf("expected exit code %d, got %d", client.ExitNotFound, apiErr.ExitCode())
	}
}

func TestSkillsShowMissingName(t *testing.T) {
	res := &skillsResource{}
	ops := res.Operations()
	showOp := ops[1]

	_, err := showOp.Run(context.Background(), nil, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing name")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}

	if apiErr.Type != "validation_error" {
		t.Errorf("expected error type 'validation_error', got %q", apiErr.Type)
	}
}
