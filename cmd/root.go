package cmd

import (
	"os"

	"github.com/airbytehq/airbyte-cli/internal/registry"
	"github.com/spf13/cobra"
)

var (
	output  string
	verbose bool
	fields  []string
)

var rootCmd = &cobra.Command{
	Use:   "airbyte",
	Short: "Airbyte CLI",
	Long:  "Command-line interface for interacting with the Airbyte platform.",
	Args:  registry.UnknownSubcommandArgs,
	Run: func(cmd *cobra.Command, args []string) {
		printSplash(os.Stdout)
	},
	SilenceUsage: true,
}

// agentsCmd is the `airbyte agents …` namespace under which all
// resource/operation commands (connectors, workspaces, organizations,
// login, schema, version, …) are mounted. The top-level `airbyte`
// binary is reserved for future namespaces; today everything lives
// under `agents`.
var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Airbyte Agents commands",
	Long:  "Manage Airbyte connectors, workspaces, organizations, and other agent resources.",
	Args:  registry.UnknownSubcommandArgs,
	Run: func(cmd *cobra.Command, args []string) {
		printSplash(os.Stdout)
	},
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "", "Output file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")
	rootCmd.PersistentFlags().StringSliceVar(&fields, "fields", nil, "Filter response to only the listed fields (comma-separated, dotted paths, e.g. 'data.id,data.name')")
	rootCmd.AddCommand(agentsCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func GetRootCmd() *cobra.Command {
	return rootCmd
}

// GetAgentsCmd returns the `agents` namespace command. The registry
// and the per-package init() functions in cmd/ mount their commands
// here so the surface is `airbyte agents <resource> <operation>`.
func GetAgentsCmd() *cobra.Command {
	return agentsCmd
}

func GetVerbose() bool {
	return verbose
}

func GetOutput() string {
	return output
}

type flags struct{}

func (f flags) GetOutput() string   { return output }
func (f flags) GetFields() []string { return fields }

func FlagAccessor() flags {
	return flags{}
}
