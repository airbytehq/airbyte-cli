package main

import (
	"os"

	"github.com/airbytehq/airbyte-agent-cli/cmd"
	"github.com/airbytehq/airbyte-agent-cli/internal/auth"
	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/airbytehq/airbyte-agent-cli/internal/config"
	"github.com/airbytehq/airbyte-agent-cli/internal/registry"
	"github.com/airbytehq/airbyte-agent-cli/internal/resources"
	"github.com/airbytehq/airbyte-agent-cli/internal/telemetry"
)

func main() {
	cfg := config.Load()

	var c *client.Client
	var t *telemetry.Tracker
	if settings, err := auth.ResolveSettings(); err == nil {
		creds := settings.Credentials
		tm := auth.NewTokenManager(cfg.APIHost, settings.OrganizationID, &creds)
		c = client.New(cfg.APIHost, settings.OrganizationID, cmd.Version, tm,
			client.WithDebugFunc(cmd.GetVerbose),
			client.WithDefaultWorkspace(settings.Workspace),
			client.WithAllowDestructive(settings.AllowDestructive),
		)
		t = telemetry.New(
			telemetry.ResolveMode(settings.TelemetryEnabled),
			settings.OrganizationID,
			cmd.Version,
			settings.IsInternalUser,
		)
	}

	registry.SetTracker(t)
	resources.RegisterAll()
	registry.Build(cmd.GetRootCmd(), c, cmd.FlagAccessor())

	err := cmd.Execute()
	t.Flush()
	if err != nil {
		os.Exit(1)
	}
}
