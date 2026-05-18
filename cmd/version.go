package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version              = "dev"
	Commit               = "none"
	Date                 = "unknown"
	ExpectedSkillVersion = "dev"
)

var versionCmd = &cobra.Command{
	Use:          "version",
	Short:        "Print the CLI version",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		line := Version + "\n"
		if output != "" {
			return os.WriteFile(output, []byte(line), 0o644)
		}
		fmt.Print(line)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("{{.Version}}\n")
}
