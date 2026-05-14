package registry

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/spf13/cobra"
)

func runUnknownCommandTest(t *testing.T, root *cobra.Command, args []string) (int, string) {
	t.Helper()

	exitCode, restore := captureExit(t)
	defer restore()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	root.SetArgs(args)
	func() {
		defer func() { _ = recover() }()
		_ = root.Execute()
	}()

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return *exitCode, buf.String()
}

func TestUnknownRootCommandEmitsJSON(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("connectors", "Manage connectors", newMockOperation("list")))
	Register(newMockResource("workspaces", "Manage workspaces", newMockOperation("list")))

	root := newTestRoot()
	root.Args = UnknownSubcommandArgs
	root.Run = func(*cobra.Command, []string) {}
	Build(root, stubClient(), &stubFlags{format: "json"})

	exit, stderr := runUnknownCommandTest(t, root, []string{"nothing"})

	if exit != client.ExitValidation {
		t.Fatalf("expected exit %d, got %d", client.ExitValidation, exit)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("parsing stderr: %v (raw: %s)", err, stderr)
	}
	if payload["type"] != "unknown_command" {
		t.Errorf("expected type=unknown_command, got %v", payload["type"])
	}
	msg, _ := payload["message"].(string)
	if !strings.Contains(msg, `"nothing"`) || !strings.Contains(msg, "airbyte-agent") {
		t.Errorf("message missing context: %q", msg)
	}
	hint, _ := payload["hint"].(string)
	if !strings.Contains(hint, "--help") {
		t.Errorf("expected hint to point to --help, got %q", hint)
	}
	available, _ := payload["available_commands"].([]any)
	names := make(map[string]bool)
	for _, v := range available {
		if s, ok := v.(string); ok {
			names[s] = true
		}
	}
	for _, want := range []string{"connectors", "workspaces"} {
		if !names[want] {
			t.Errorf("expected %q in available_commands, got %v", want, available)
		}
	}
	if _, ok := payload["did_you_mean"]; ok {
		t.Errorf("did_you_mean should be omitted when no close match exists, got %v", payload["did_you_mean"])
	}
}

func TestUnknownRootCommandIncludesSuggestion(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("connectors", "Manage connectors", newMockOperation("list")))

	root := newTestRoot()
	root.Args = UnknownSubcommandArgs
	root.Run = func(*cobra.Command, []string) {}
	Build(root, stubClient(), &stubFlags{format: "json"})

	exit, stderr := runUnknownCommandTest(t, root, []string{"connector"})

	if exit != client.ExitValidation {
		t.Fatalf("expected exit %d, got %d", client.ExitValidation, exit)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("parsing stderr: %v (raw: %s)", err, stderr)
	}
	suggestions, ok := payload["did_you_mean"].([]any)
	if !ok || len(suggestions) == 0 {
		t.Fatalf("expected did_you_mean to be populated, got %v", payload["did_you_mean"])
	}
	if suggestions[0] != "connectors" {
		t.Errorf("expected did_you_mean[0]=connectors, got %v", suggestions[0])
	}
}

func TestUnknownSubcommandScopesPayloadToResource(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("connectors", "Manage connectors",
		newMockOperation("list"),
		newMockOperation("describe"),
		newMockOperation("execute"),
	))
	Register(newMockResource("workspaces", "Manage workspaces", newMockOperation("list")))

	root := newTestRoot()
	root.Args = UnknownSubcommandArgs
	root.Run = func(*cobra.Command, []string) {}
	Build(root, stubClient(), &stubFlags{format: "json"})

	exit, stderr := runUnknownCommandTest(t, root, []string{"connectors", "notarealthing"})

	if exit != client.ExitValidation {
		t.Fatalf("expected exit %d, got %d", client.ExitValidation, exit)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("parsing stderr: %v (raw: %s)", err, stderr)
	}
	msg, _ := payload["message"].(string)
	if !strings.Contains(msg, "airbyte-agent connectors") {
		t.Errorf("expected message scoped to 'airbyte-agent connectors', got %q", msg)
	}
	available, _ := payload["available_commands"].([]any)
	names := make(map[string]bool)
	for _, v := range available {
		if s, ok := v.(string); ok {
			names[s] = true
		}
	}
	for _, want := range []string{"list", "describe", "execute"} {
		if !names[want] {
			t.Errorf("expected %q in scoped available_commands, got %v", want, available)
		}
	}
	if names["workspaces"] {
		t.Errorf("available_commands should not leak other resources: %v", available)
	}
}

func TestUnknownSubcommandSuggestsCloseMatch(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("connectors", "Manage connectors",
		newMockOperation("list"),
		newMockOperation("describe"),
	))

	root := newTestRoot()
	root.Args = UnknownSubcommandArgs
	root.Run = func(*cobra.Command, []string) {}
	Build(root, stubClient(), &stubFlags{format: "json"})

	exit, stderr := runUnknownCommandTest(t, root, []string{"connectors", "lits"})

	if exit != client.ExitValidation {
		t.Fatalf("expected exit %d, got %d", client.ExitValidation, exit)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("parsing stderr: %v (raw: %s)", err, stderr)
	}
	suggestions, ok := payload["did_you_mean"].([]any)
	if !ok || len(suggestions) == 0 {
		t.Fatalf("expected did_you_mean to suggest 'list', got %v", payload["did_you_mean"])
	}
	if suggestions[0] != "list" {
		t.Errorf("expected did_you_mean[0]=list, got %v", suggestions[0])
	}
}

func TestBareResourceShowsHelp(t *testing.T) {
	t.Cleanup(func() { Reset() })

	Register(newMockResource("connectors", "Manage connectors", newMockOperation("list")))

	root := newTestRoot()
	root.Args = UnknownSubcommandArgs
	root.Run = func(*cobra.Command, []string) {}
	Build(root, stubClient(), &stubFlags{format: "json"})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	root.SetArgs([]string{"connectors"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if !strings.Contains(buf.String(), "list") {
		t.Errorf("expected bare 'connectors' to show help listing 'list', got: %s", buf.String())
	}
}
