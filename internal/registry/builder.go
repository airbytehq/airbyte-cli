package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

// paramBinding holds the flag-name and the bound pointer for a single schema
// parameter. Only the field matching `kind` is populated.
type paramBinding struct {
	flagName string
	kind     string
	strVal   *string
	boolVal  *bool
	intVal   *int
	floatVal *float64
	sliceVal *[]string
}

// flagNameFor maps a snake_case schema key to a kebab-case CLI flag.
func flagNameFor(schemaKey string) string {
	return strings.ReplaceAll(schemaKey, "_", "-")
}

func buildOperationCmd(op *Operation, c *client.Client, flags flagAccessor) *cobra.Command {
	var jsonInput string
	bindings := map[string]*paramBinding{}

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
			params, err := collectParams(cmd, jsonInput, bindings, op.Schema)
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

	cmd.Flags().StringVar(&jsonInput, "json", "", "Input parameters as JSON (or @filename to read from file). Cannot be combined with parameter flags.")

	keys := make([]string, 0, len(op.Schema.Params))
	for k := range op.Schema.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ps := op.Schema.Params[key]
		flagName := flagNameFor(key)
		desc := ps.Description
		if ps.Required {
			desc += " (required)"
		}
		b := &paramBinding{flagName: flagName, kind: ps.Type}
		switch ps.Type {
		case "string":
			b.strVal = new(string)
			cmd.Flags().StringVar(b.strVal, flagName, "", desc)
		case "bool", "boolean":
			b.boolVal = new(bool)
			cmd.Flags().BoolVar(b.boolVal, flagName, false, desc)
		case "int", "integer":
			b.intVal = new(int)
			cmd.Flags().IntVar(b.intVal, flagName, 0, desc)
		case "number":
			b.floatVal = new(float64)
			cmd.Flags().Float64Var(b.floatVal, flagName, 0, desc)
		case "array":
			b.sliceVal = new([]string)
			cmd.Flags().StringSliceVar(b.sliceVal, flagName, nil, desc+" (comma-separated, or repeat the flag)")
		default:
			// "object" or unknown types have no flag form — caller must use --json.
			continue
		}
		bindings[key] = b
	}

	return cmd
}

// collectParams resolves the operation's parameters from either --json or the
// per-parameter flags, enforcing that the two modes are mutually exclusive.
func collectParams(cmd *cobra.Command, jsonInput string, bindings map[string]*paramBinding, schema OperationSchema) (map[string]any, error) {
	jsonSet := cmd.Flags().Changed("json")

	var setParamFlags []string
	for _, b := range bindings {
		if cmd.Flags().Changed(b.flagName) {
			setParamFlags = append(setParamFlags, "--"+b.flagName)
		}
	}
	sort.Strings(setParamFlags)

	if jsonSet && len(setParamFlags) > 0 {
		writeStderrJSON(map[string]any{
			"error":   "validation_error",
			"message": fmt.Sprintf("--json cannot be combined with parameter flags (%s)", strings.Join(setParamFlags, ", ")),
			"hint":    "pass parameters either as --json or as individual flags, not both",
		})
		osExit(client.ExitValidation)
		return nil, fmt.Errorf("validation error")
	}

	params := map[string]any{}

	if jsonSet {
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
	} else {
		for key, b := range bindings {
			if !cmd.Flags().Changed(b.flagName) {
				continue
			}
			switch b.kind {
			case "string":
				params[key] = *b.strVal
			case "bool", "boolean":
				params[key] = *b.boolVal
			case "int", "integer":
				params[key] = *b.intVal
			case "number":
				params[key] = *b.floatVal
			case "array":
				arr := make([]any, len(*b.sliceVal))
				for i, v := range *b.sliceVal {
					arr[i] = v
				}
				params[key] = arr
			}
		}
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
