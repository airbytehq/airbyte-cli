package telemetry

import (
	"os"
	"runtime"
)

// EventName is the single Segment event emitted by the CLI.
const EventName = "CLI Command Executed"

// CommandEvent is the payload for a single tracked CLI invocation.
// Mirrors the connector-sdk's OperationEvent, condensed for a one-shot
// CLI where session = invocation = one event.
type CommandEvent struct {
	// Command is the leaf command name, e.g. "connectors execute",
	// "configure".
	Command string
	// Success is false when the operation returned an error or exited
	// via a non-zero exit code.
	Success bool
	// DurationMs is the wall-clock time the RunE handler took.
	DurationMs int64
	// ErrorType is the APIError.Type for failures sourced from the
	// API client, or a CLI-side label ("validation_error", "timeout",
	// etc.) for failures sourced before the network call.
	ErrorType string
	// StatusCode is the final HTTP status when known.
	StatusCode int
	// Entity/Action are only populated for `connectors execute`.
	Entity string
	Action string
}

// ToProperties converts the event + the surrounding context (org_id,
// cli_version, environment) into the Segment properties map. The
// returned map's organization_id field carries identity — Segment's
// userId is intentionally left empty.
func (e CommandEvent) ToProperties(org string, cliVersion string, internal bool) map[string]any {
	props := map[string]any{
		"organization_id":   org,
		"command":           e.Command,
		"success":           e.Success,
		"duration_ms":       e.DurationMs,
		"cli_version":       cliVersion,
		"os_name":           runtime.GOOS,
		"os_version":        runtime.GOARCH,
		"go_version":        runtime.Version(),
		"execution_context": executionContext(),
		"is_internal_user":  internal,
	}
	if e.ErrorType != "" {
		props["error_type"] = e.ErrorType
	}
	if e.StatusCode != 0 {
		props["status_code"] = e.StatusCode
	}
	if e.Entity != "" {
		props["entity"] = e.Entity
	}
	if e.Action != "" {
		props["action"] = e.Action
	}
	return props
}

// executionContext returns the caller's self-reported context (e.g.
// "mcp", "agent") so analytics can distinguish direct human invocations
// from MCP/agent-driven ones. Defaults to "direct" when unset.
func executionContext() string {
	if v := os.Getenv("AIRBYTE_EXECUTION_CONTEXT"); v != "" {
		return v
	}
	return "direct"
}
