package registry

import (
	"context"
	"sync"
	"testing"

	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/airbytehq/airbyte-agent-cli/internal/telemetry"
	analytics "github.com/segmentio/analytics-go/v3"
)

// fakeAnalyticsClient records tracks for assertions. Mirrors the one in
// internal/telemetry/tracker_test.go but lives here so the registry
// test doesn't reach into another package.
type fakeAnalyticsClient struct {
	mu     sync.Mutex
	tracks []analytics.Track
}

func (f *fakeAnalyticsClient) Enqueue(msg analytics.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := msg.(analytics.Track); ok {
		f.tracks = append(f.tracks, t)
	}
	return nil
}

func (f *fakeAnalyticsClient) Close() error { return nil }

func (f *fakeAnalyticsClient) snapshot() []analytics.Track {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]analytics.Track, len(f.tracks))
	copy(out, f.tracks)
	return out
}

// installFakeTracker wires a fake-backed tracker into the package and
// returns it + a cleanup that restores the prior state.
func installFakeTracker(t *testing.T) (*fakeAnalyticsClient, func()) {
	t.Helper()
	fake := &fakeAnalyticsClient{}
	tr := telemetry.Wrap(fake, "org-test", "test-version", false)
	prev := tracker
	tracker = tr
	return fake, func() { tracker = prev }
}

func TestRegistryEmitsEventOnSuccess(t *testing.T) {
	t.Cleanup(func() { Reset() })
	fake, restoreTracker := installFakeTracker(t)
	defer restoreTracker()

	Register(newMockResource("widgets", "Widgets",
		Operation{
			Name:        "list",
			Description: "list widgets",
			Schema: OperationSchema{
				Description: "list widgets",
				Params:      map[string]ParamSchema{},
			},
			Run: func(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
				return map[string]string{"ok": "true"}, nil
			},
		},
	))

	root := newTestRoot()
	flags := &stubFlags{format: "json", output: t.TempDir() + "/out.json"}
	Build(root, stubClient(), flags)

	root.SetArgs([]string{"widgets", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	tracks := fake.snapshot()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tracks))
	}
	props := tracks[0].Properties
	if props["command"] != "widgets list" {
		t.Errorf("command = %v, want \"widgets list\"", props["command"])
	}
	if props["success"] != true {
		t.Errorf("success = %v, want true", props["success"])
	}
	if props["organization_id"] != "org-test" {
		t.Errorf("organization_id = %v, want org-test", props["organization_id"])
	}
	if _, ok := props["error_type"]; ok {
		t.Errorf("error_type unexpectedly set on success event: %v", props["error_type"])
	}
}

func TestRegistryEmitsEventOnValidationError(t *testing.T) {
	t.Cleanup(func() { Reset() })
	fake, restoreTracker := installFakeTracker(t)
	defer restoreTracker()

	Register(newMockResource("widgets", "Widgets",
		Operation{
			Name:        "create",
			Description: "create a widget",
			Schema: OperationSchema{
				Description: "create",
				Params: map[string]ParamSchema{
					"name": {Type: "string", Required: true, Description: "name"},
				},
			},
			Run: func(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
				t.Error("Run should not be called when required param is missing")
				return nil, nil
			},
		},
	))

	exitCode, restoreExit := captureExit(t)
	defer restoreExit()

	root := newTestRoot()
	Build(root, stubClient(), &stubFlags{format: "json"})

	// Missing the required `name` param triggers validateParams →
	// writeStderrJSON → osExit (the wrapped one that flushes telemetry).
	root.SetArgs([]string{"widgets", "create", "--json", "{}"})
	func() {
		defer func() { _ = recover() }()
		_ = root.Execute()
	}()

	if *exitCode != client.ExitValidation {
		t.Fatalf("exit code = %d, want %d", *exitCode, client.ExitValidation)
	}

	tracks := fake.snapshot()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tracks))
	}
	props := tracks[0].Properties
	if props["success"] != false {
		t.Errorf("success = %v, want false", props["success"])
	}
	if props["error_type"] != "validation_error" {
		t.Errorf("error_type = %v, want validation_error", props["error_type"])
	}
	if props["command"] != "widgets create" {
		t.Errorf("command = %v, want \"widgets create\"", props["command"])
	}
}

func TestRegistryAnnotatesEntityActionForExecute(t *testing.T) {
	t.Cleanup(func() { Reset() })
	fake, restoreTracker := installFakeTracker(t)
	defer restoreTracker()

	Register(newMockResource("connectors", "Connectors",
		Operation{
			Name:        "execute",
			Description: "execute",
			Schema: OperationSchema{
				Description: "execute",
				Params: map[string]ParamSchema{
					"entity": {Type: "string", Required: true, Description: "entity"},
					"action": {Type: "string", Required: true, Description: "action"},
				},
			},
			Run: func(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
				return map[string]string{"ok": "true"}, nil
			},
		},
	))

	root := newTestRoot()
	flags := &stubFlags{format: "json", output: t.TempDir() + "/out.json"}
	Build(root, stubClient(), flags)

	root.SetArgs([]string{"connectors", "execute", "--entity", "customers", "--action", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	tracks := fake.snapshot()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tracks))
	}
	props := tracks[0].Properties
	if props["entity"] != "customers" {
		t.Errorf("entity = %v, want customers", props["entity"])
	}
	if props["action"] != "list" {
		t.Errorf("action = %v, want list", props["action"])
	}
}
