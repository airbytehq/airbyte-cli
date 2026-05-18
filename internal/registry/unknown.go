package registry

import (
	"fmt"
	"sort"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/spf13/cobra"
)

// EmitUnknownCommand writes an unknown_command JSON error to stderr scoped
// to parent's subcommands, then exits with ExitValidation. The payload is
// designed for AI agents to self-correct in one round trip: it carries the
// Levenshtein-distance suggestions Cobra already computes plus the full
// list of valid subcommands at this level.
func EmitUnknownCommand(parent *cobra.Command, attempted string) {
	writeStderrJSON(unknownCommandPayload(parent, attempted))
	osExit(client.ExitValidation)
}

// UnknownSubcommandArgs is a cobra.PositionalArgs that emits an
// unknown_command error when extra positional args are present. Intended
// for parent commands (root, resources) that route to subcommands; leaf
// operation commands have their own param-flag handling.
func UnknownSubcommandArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		EmitUnknownCommand(cmd, args[0])
	}
	return nil
}

func unknownCommandPayload(parent *cobra.Command, attempted string) map[string]any {
	payload := map[string]any{
		"type":               "unknown_command",
		"message":            fmt.Sprintf("unknown command %q for %q", attempted, parent.CommandPath()),
		"available_commands": availableSubcommandNames(parent),
		"hint":               fmt.Sprintf("run '%s --help' to see available commands", parent.CommandPath()),
	}
	// Cobra's built-in `findSuggestions` lazily defaults this to 2 when
	// emitting its own unknown-command error. SuggestionsFor doesn't
	// apply that default itself, so callers get an empty list unless we
	// set it here.
	if parent.SuggestionsMinimumDistance <= 0 {
		parent.SuggestionsMinimumDistance = 2
	}
	if s := parent.SuggestionsFor(attempted); len(s) > 0 {
		payload["did_you_mean"] = s
	}
	return payload
}

func availableSubcommandNames(parent *cobra.Command) []string {
	names := make([]string, 0, len(parent.Commands()))
	for _, c := range parent.Commands() {
		if c.IsAvailableCommand() {
			names = append(names, c.Name())
		}
	}
	sort.Strings(names)
	return names
}
