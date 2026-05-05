package main

import (
	"os"

	"github.com/airbytehq/airbyte-cli/cmd"
	"github.com/airbytehq/airbyte-cli/internal/auth"
	"github.com/airbytehq/airbyte-cli/internal/client"
	"github.com/airbytehq/airbyte-cli/internal/config"
	"github.com/airbytehq/airbyte-cli/internal/registry"
	"github.com/airbytehq/airbyte-cli/internal/resources"
)

func main() {
	cfg := config.Load()

	var c *client.Client
	if creds, err := auth.ResolveCredentials(); err == nil {
		tm := auth.NewTokenManager(cfg.APIHost, creds.OrganizationID, creds)
		c = client.New(cfg.APIHost, creds.OrganizationID, cmd.Version, tm, client.WithDebugFunc(cmd.GetVerbose))
	}

	resources.RegisterAll()
	registry.Build(cmd.GetRootCmd(), c, cmd.FlagAccessor())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
