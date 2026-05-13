package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const splashLogo = ` █████╗ ██╗██████╗ ██████╗ ██╗   ██╗████████╗███████╗      █████╗  ██████╗ ███████╗███╗   ██╗████████╗
██╔══██╗██║██╔══██╗██╔══██╗╚██╗ ██╔╝╚══██╔══╝██╔════╝     ██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝
███████║██║██████╔╝██████╔╝ ╚████╔╝    ██║   █████╗   ██╗ ███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║
██╔══██║██║██╔══██╗██╔══██╗  ╚██╔╝     ██║   ██╔══╝   ╚═╝ ██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║
██║  ██║██║██║  ██║██████╔╝   ██║      ██║   ███████╗     ██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║
╚═╝  ╚═╝╚═╝╚═╝  ╚═╝╚═════╝    ╚═╝      ╚═╝   ╚══════╝     ╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝`

const splashTagline = "Command-line interface for the Airbyte Agent platform"

type splashEntry struct {
	cmd  string
	desc string
}

var splashGroups = [][]splashEntry{
	{
		{"airbyte-agent login", "Authenticate and save credentials"},
		{"airbyte-agent help", "Show command usage instructions"},
	},
	{
		{"airbyte-agent connectors list", "List connectors in a workspace"},
		{"airbyte-agent connectors describe", "Inspect a connector's entities and actions"},
		{"airbyte-agent connectors execute", "Run an action against a connector entity"},
		{"airbyte-agent connectors create", "Install a new connector"},
		{"airbyte-agent connectors delete", "Remove a connector from a workspace"},
	},
	{
		{"airbyte-agent workspaces list", "List workspaces in your organization"},
		{"airbyte-agent workspaces use", "Set the default workspace"},
	},
	{
		{"airbyte-agent schema", "Print request/response schema for an operation"},
		{"airbyte-agent version", "Print the CLI version"},
	},
}

const splashTry = "airbyte-agent login"

func printSplash(w io.Writer) {
	color := splashColors(w)

	fmt.Fprintln(w)
	for _, line := range strings.Split(splashLogo, "\n") {
		fmt.Fprintln(w, color.logo+line+color.reset)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.tagline+splashTagline+color.reset)
	fmt.Fprintln(w)

	maxPrefix := 0
	for _, group := range splashGroups {
		for _, e := range group {
			if l := len("  $ " + e.cmd); l > maxPrefix {
				maxPrefix = l
			}
		}
	}
	pad := maxPrefix + 3

	for i, group := range splashGroups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		for _, e := range group {
			prefix := "  $ " + e.cmd
			spaces := strings.Repeat(" ", pad-len(prefix))
			fmt.Fprintln(w,
				color.dollar+"  $ "+color.reset+
					color.cmd+e.cmd+color.reset+
					spaces+
					color.desc+e.desc+color.reset)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, color.tagline+"try: "+color.reset+color.accent+splashTry+color.reset)
}

type splashPalette struct {
	reset, logo, tagline, dollar, cmd, desc, accent string
}

func splashColors(w io.Writer) splashPalette {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return splashPalette{}
	}
	return splashPalette{
		reset:   "\033[0m",
		logo:    "\033[38;5;141m",
		tagline: "\033[38;5;244m",
		dollar:  "\033[38;5;244m",
		cmd:     "\033[1;38;5;255m",
		desc:    "\033[38;5;244m",
		accent:  "\033[38;5;99m",
	}
}
