package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/output"
	"github.com/spf13/cobra"
)

var osExit = os.Exit

type flagAccessor interface {
	GetFormat() string
	GetOutput() string
	GetDescribe() bool
}

func Build(rootCmd *cobra.Command, c *client.Client, flags flagAccessor) {
	for _, res := range All() {
		resCmd := &cobra.Command{
			Use:   res.Name(),
			Short: res.Description(),
			Run: func(cmd *cobra.Command, args []string) {
				_ = cmd.Help()
			},
		}

		for i := range res.Operations() {
			op := res.Operations()[i]
			opCmd := buildOperationCmd(&op, c, flags)
			resCmd.AddCommand(opCmd)
		}

		rootCmd.AddCommand(resCmd)
	}
}

func buildOperationCmd(op *Operation, c *client.Client, flags flagAccessor) *cobra.Command {
	var jsonInput string
	var idFlag string

	cmd := &cobra.Command{
		Use:   op.Name,
		Short: op.Description,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if flags.GetDescribe() {
				if err := output.WriteJSON(os.Stdout, op.Schema); err != nil {
					writeStderrError("output_error", err.Error())
					osExit(client.ExitGeneral)
				}
				osExit(client.ExitSuccess)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseAndValidate(jsonInput, idFlag, op.Schema)
			if err != nil {
				return err
			}

			ctx := context.Background()

			if op.Hooks.Interactive != nil {
				result, err := op.Hooks.Interactive(ctx, c, params)
				if err != nil {
					return handleRunError(err)
				}
				return writeResult(result, flags)
			}

			if c == nil {
				return handleRunError(&client.APIError{
					Type:       "auth_error",
					Message:    "no credentials configured: set AIRBYTE_CLIENT_ID and AIRBYTE_CLIENT_SECRET environment variables, or create ~/.airbyte/credentials",
					StatusCode: 401,
				})
			}

			if op.Hooks.PreRun != nil {
				params, err = op.Hooks.PreRun(ctx, c, params)
				if err != nil {
					return handleRunError(err)
				}
			}

			result, err := op.Run(ctx, c, params)
			if err != nil {
				return handleRunError(err)
			}

			return writeResult(result, flags)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.Flags().StringVar(&jsonInput, "json", "", "Input parameters as JSON (or @filename to read from file)")
	cmd.Flags().StringVar(&idFlag, "id", "", "Resource ID (convenience for --json '{\"id\": \"...\"}')")

	return cmd
}

func parseAndValidate(jsonInput, idFlag string, schema OperationSchema) (map[string]any, error) {
	params := make(map[string]any)

	if jsonInput != "" {
		raw, err := resolveJSONInput(jsonInput)
		if err != nil {
			writeStderrError("input_error", err.Error())
			osExit(client.ExitValidation)
			return nil, fmt.Errorf("input error")
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			writeStderrError("input_error", fmt.Sprintf("invalid JSON: %s", err.Error()))
			osExit(client.ExitValidation)
			return nil, fmt.Errorf("invalid JSON")
		}
	}

	if idFlag != "" {
		params["id"] = idFlag
	}

	if err := validateParams(params, schema); err != nil {
		return nil, err
	}

	return params, nil
}

func resolveJSONInput(input string) ([]byte, error) {
	if strings.HasPrefix(input, "@") {
		filename := input[1:]
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", filename, err)
		}
		return data, nil
	}
	return []byte(input), nil
}

func validateParams(params map[string]any, schema OperationSchema) error {
	missing := make(map[string]string)
	for name, ps := range schema.Params {
		if !ps.Required {
			continue
		}
		if _, ok := params[name]; !ok {
			missing[name] = "required"
		}
	}

	if len(missing) > 0 {
		errPayload := map[string]any{
			"error":  "validation_error",
			"fields": missing,
			"hint":   "run this command with --describe to see the expected parameter schema",
		}
		writeStderrJSON(errPayload)
		osExit(client.ExitValidation)
		return fmt.Errorf("validation error")
	}

	return nil
}

func handleRunError(err error) error {
	if apiErr, ok := err.(*client.APIError); ok {
		errPayload := map[string]any{
			"error":       apiErr.Type,
			"message":     apiErr.Message,
			"status_code": apiErr.StatusCode,
		}
		if apiErr.Detail != nil {
			errPayload["detail"] = apiErr.Detail
		}
		if apiErr.Hint != "" {
			errPayload["hint"] = apiErr.Hint
		}
		writeStderrJSON(errPayload)
		osExit(apiErr.ExitCode())
		return err
	}
	writeStderrError("error", err.Error())
	osExit(client.ExitGeneral)
	return err
}

func writeResult(result any, flags flagAccessor) error {
	return output.Write(result, flags.GetFormat(), flags.GetOutput())
}

func writeStderrError(errType, message string) {
	writeStderrJSON(map[string]any{
		"error":   errType,
		"message": message,
	})
}

func writeStderrJSON(payload map[string]any) {
	output.WriteError(payload)
}
