package registry

import (
	"context"
	"testing"

	"github.com/airbytehq/airbyte-agents-cli/internal/client"
)

type mockResource struct {
	name        string
	description string
	operations  []Operation
}

func (m *mockResource) Name() string {
	return m.name
}

func (m *mockResource) Description() string {
	return m.description
}

func (m *mockResource) Operations() []Operation {
	return m.operations
}

func newMockResource(name, desc string, ops ...Operation) *mockResource {
	return &mockResource{name: name, description: desc, operations: ops}
}

func newMockOperation(name string) Operation {
	return Operation{
		Name:        name,
		Description: "Test operation " + name,
		Schema: OperationSchema{
			Description: "Schema for " + name,
			Params: map[string]ParamSchema{
				"id": {Type: "string", Required: true, Description: "Resource ID"},
			},
		},
		Run: func(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
			return map[string]string{"status": "ok"}, nil
		},
	}
}

func TestRegisterAndGet(t *testing.T) {
	t.Cleanup(func() { Reset() })

	r := newMockResource("widgets", "Manage widgets", newMockOperation("list"))
	Register(r)

	got, ok := Get("widgets")
	if !ok {
		t.Fatal("expected resource 'widgets' to be registered")
	}
	if got.Name() != "widgets" {
		t.Errorf("expected name 'widgets', got %q", got.Name())
	}
	if got.Description() != "Manage widgets" {
		t.Errorf("expected description 'Manage widgets', got %q", got.Description())
	}
}

func TestGetMissing(t *testing.T) {
	t.Cleanup(func() { Reset() })

	_, ok := Get("nonexistent")
	if ok {
		t.Fatal("expected resource 'nonexistent' to not be found")
	}
}

func TestAll(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("alpha", "Alpha resource"))
	Register(newMockResource("beta", "Beta resource"))
	Register(newMockResource("gamma", "Gamma resource"))

	all := All()
	if len(all) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(all))
	}

	names := make([]string, len(all))
	for i, r := range all {
		names[i] = r.Name()
	}
	expected := []string{"alpha", "beta", "gamma"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected resource %d to be %q, got %q", i, name, names[i])
		}
	}
}

func TestRegisterDuplicate(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("dup", "First"))
	Register(newMockResource("dup", "Second"))

	all := All()
	if len(all) != 1 {
		t.Fatalf("expected 1 resource after duplicate register, got %d", len(all))
	}
	if all[0].Description() != "First" {
		t.Errorf("expected first registration to win, got description %q", all[0].Description())
	}
}

func TestReset(t *testing.T) {
	Register(newMockResource("temp", "Temporary"))
	Reset()

	all := All()
	if len(all) != 0 {
		t.Fatalf("expected 0 resources after reset, got %d", len(all))
	}

	_, ok := Get("temp")
	if ok {
		t.Fatal("expected 'temp' to not be found after reset")
	}
}

func TestOperationSchema(t *testing.T) {
	op := newMockOperation("get")
	if op.Schema.Description != "Schema for get" {
		t.Errorf("unexpected schema description: %q", op.Schema.Description)
	}

	ps, ok := op.Schema.Params["id"]
	if !ok {
		t.Fatal("expected 'id' param in schema")
	}
	if ps.Type != "string" {
		t.Errorf("expected param type 'string', got %q", ps.Type)
	}
	if !ps.Required {
		t.Error("expected param 'id' to be required")
	}
}
