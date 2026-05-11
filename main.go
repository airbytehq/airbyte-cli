package main

import (
	"os"

	"github.com/airbytehq/airbyte-agents-cli/cmd"
	"github.com/airbytehq/airbyte-agents-cli/internal/auth"
	"github.com/airbytehq/airbyte-agents-cli/internal/client"
	"github.com/airbytehq/airbyte-agents-cli/internal/config"
	"github.com/airbytehq/airbyte-agents-cli/internal/registry"
	"github.com/airbytehq/airbyte-agents-cli/internal/resources"
)

func main() {
	cfg := config.Load()

	var c *client.Client
	if settings, err := auth.ResolveSettings(); err == nil {
		creds := settings.Credentials
		tm := auth.NewTokenManager(cfg.APIHost, settings.OrganizationID, &creds)
		c = client.New(cfg.APIHost, settings.OrganizationID, cmd.Version, tm,
			client.WithDebugFunc(cmd.GetVerbose),
			client.WithDefaultWorkspace(settings.Workspace),
			client.WithAllowDestructive(settings.AllowDestructive),
		)
	}

	cmd.SetAPIClient(c)
	resources.RegisterAll()
	registry.Build(cmd.GetRootCmd(), c, cmd.FlagAccessor())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
