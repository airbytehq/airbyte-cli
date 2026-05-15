package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version              = "dev"
	Commit               = "none"
	Date                 = "unknown"
	ExpectedSkillVersion = "dev"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("{{.Version}}\n")
}
