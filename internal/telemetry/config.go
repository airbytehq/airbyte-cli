// Package telemetry emits anonymous CLI usage events to Segment.
//
// One event per tracked CLI invocation: command name, success/failure,
// timing, error_type, and a small set of identifying properties
// (organization_id, cli_version, os, etc.). See events.go for the full
// schema.
//
// The tracker is best-effort — every public method recovers from panics
// and swallows errors so telemetry failures never break the user's
// command.
package telemetry

import (
	"os"
	"strings"
)

// segmentWriteKey is the project-specific Segment write key. It's not a
// secret: it has write-only scope and only authorizes the binary to send
// events to the Airbyte CLI Segment source. Get the value from the
// Segment dashboard (Connections → Sources → API Keys → Write Key) and
// paste it here.
//
// When this constant is empty the tracker no-ops, which is the right
// behavior for forks and local builds where the project's Segment source
// isn't relevant.
const segmentWriteKey = "sFM7q98HtHTMmCW3d6nsPWYCIdrbs7gq"

// Mode describes whether telemetry is on. Mirrors the connector-sdk's
// TelemetryMode enum.
type Mode string

const (
	ModeBasic    Mode = "basic"
	ModeDisabled Mode = "disabled"
)

// ResolveMode reads AIRBYTE_TELEMETRY_MODE. An explicit "disabled" wins
// regardless of the settingsEnabled argument; otherwise the per-user
// setting from settings.json decides.
func ResolveMode(settingsEnabled bool) Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIRBYTE_TELEMETRY_MODE"))) {
	case "disabled":
		return ModeDisabled
	}
	if settingsEnabled {
		return ModeBasic
	}
	return ModeDisabled
}
