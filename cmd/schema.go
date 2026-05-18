package cmd

import (
	"fmt"
	"os"

	"github.com/airbytehq/airbyte-cli/internal/client"
	outputpkg "github.com/airbytehq/airbyte-cli/internal/output"
	"github.com/airbytehq/airbyte-cli/internal/registry"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema <resource> <operation>",
	Short: "Print the full schema (CLI params + OpenAPI request/response) for an operation",
	Long: `Print the merged schema for an operation: the CLI-level parameter shape and,
when an OpenAPI mapping is declared, the API route's parameters, request body,
and response schema.

Use it when you want agents to find the introspection surface for an operation
without making an API call.`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		resourceName, opName := args[0], args[1]
		res, ok := registry.Get(resourceName)
		if !ok {
			return writeSchemaError("not_found", fmt.Sprintf("resource %q not found", resourceName), client.ExitNotFound)
		}
		for _, op := range res.Operations() {
			if op.Name == opName {
				if op.SpecRef.IsInternal() {
					return writeSchemaError(
						"not_supported",
						fmt.Sprintf("no published schema for %q %q; run `airbyte agents %s %s --help` for argument details", resourceName, opName, resourceName, opName),
						client.ExitNotFound,
					)
				}
				return outputpkg.Write(registry.BuildSchemaOutput(op), output)
			}
		}
		return writeSchemaError("not_found", fmt.Sprintf("operation %q not found on resource %q", opName, resourceName), client.ExitNotFound)
	},
}

func init() {
	agentsCmd.AddCommand(schemaCmd)
}

func writeSchemaError(errType, message string, exitCode int) error {
	outputpkg.WriteError(map[string]any{"type": errType, "message": message})
	os.Exit(exitCode)
	return fmt.Errorf("%s", message) // unreachable; satisfies signature
}
