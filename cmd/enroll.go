package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	outputpkg "github.com/airbytehq/airbyte-agent-cli/internal/output"
	"github.com/airbytehq/airbyte-agent-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const enrollPath = "/api/v1/internal/account/enrollment-status"

// apiClient is set by main.go after credentials are resolved. Top-level
// commands that need authenticated HTTP access read it here.
var apiClient *client.Client

// tracker is set by main.go after settings are resolved. Top-level
// commands (`enroll`, `configure`) read it here. Nil is safe — every
// method on *telemetry.Tracker is nil-safe.
var tracker *telemetry.Tracker

// SetAPIClient wires the authenticated HTTP client into the top-level
// commands that need it. main.go calls this once during startup.
func SetAPIClient(c *client.Client) {
	apiClient = c
}

// SetTracker wires the telemetry tracker into the top-level commands.
// main.go calls this once during startup.
func SetTracker(t *telemetry.Tracker) {
	tracker = t
}

var enrollCmd = &cobra.Command{
	Use:   "enroll",
	Short: "Verify and trigger account enrollment",
	Long: `Check the account's enrollment and provisioning status. If the account has
not been enrolled yet, the API begins provisioning automatically when this
command is called — keep invoking until 'is_enrolled: true' and
'provisioning_state: COMPLETED'.

Returns a JSON object with 'is_enrolled' (bool) and 'provisioning_state'
(one of 'IN_PROGRESS', 'COMPLETED', 'FAILED').`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		event := &telemetry.CommandEvent{Command: "enroll"}
		emitted := false
		emit := func() {
			if emitted {
				return
			}
			emitted = true
			event.DurationMs = time.Since(start).Milliseconds()
			if event.ErrorType == "" {
				event.Success = true
			}
			tracker.TrackCommand(*event)
			tracker.Flush()
		}
		defer emit()

		if apiClient == nil {
			event.ErrorType = "auth_error"
			event.StatusCode = client.ExitAuth
			outputpkg.WriteError(map[string]any{
				"type":    "auth_error",
				"message": "no credentials configured: run 'airbyte-agent configure' or set AIRBYTE_CLIENT_ID, AIRBYTE_CLIENT_SECRET, and AIRBYTE_ORGANIZATION_ID",
			})
			emit()
			os.Exit(client.ExitAuth)
		}

		raw, err := apiClient.Get(context.Background(), enrollPath, nil)
		if err != nil {
			return handleAPIError(err, event, emit)
		}

		var value any = raw
		if decoded, derr := decodeJSON(raw); derr == nil {
			value = decoded
		}
		if fields := fields; len(fields) > 0 {
			value = outputpkg.Filter(value, fields)
		}
		return outputpkg.Write(value, format, output)
	},
}

func init() {
	rootCmd.AddCommand(enrollCmd)
}

func decodeJSON(raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// handleAPIError writes the error payload to stderr, annotates the
// in-flight telemetry event (type + status), emits + flushes, then
// terminates. The emit closure is provided by the caller so the
// success/failure decision tree stays in one place.
func handleAPIError(err error, event *telemetry.CommandEvent, emit func()) error {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		event.ErrorType = apiErr.Type
		event.StatusCode = apiErr.StatusCode
		payload := map[string]any{
			"type":        apiErr.Type,
			"message":     apiErr.Message,
			"status_code": apiErr.StatusCode,
			"retryable":   apiErr.Retryable,
		}
		if apiErr.Detail != nil {
			payload["detail"] = apiErr.Detail
		}
		if apiErr.Hint != "" {
			payload["hint"] = apiErr.Hint
		}
		outputpkg.WriteError(payload)
		emit()
		os.Exit(apiErr.ExitCode())
	}
	event.ErrorType = "error"
	outputpkg.WriteError(map[string]any{"type": "error", "message": err.Error()})
	emit()
	os.Exit(client.ExitGeneral)
	return err
}
