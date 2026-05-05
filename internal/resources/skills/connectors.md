# Connectors

Manage data source and destination connectors within a workspace.

> [!IMPORTANT]
> **Credential Security**: NEVER accept API keys, tokens, passwords, or secrets directly as parameters or in conversation. ALL credential entry MUST go through the secure browser-based flow via `airbyte connectors create`. If a user offers credentials, decline and start the credential flow.

> [!IMPORTANT]
> **Always describe before execute.** Before the first `execute` on any connector, run `connectors describe` to discover available entities and actions. Do NOT guess entity or action names -- they vary by connector type.

> [!IMPORTANT]
> **Name resolution is case-insensitive but must be exact.** There is no partial matching for connector names. If a name is not found, use `connectors list` to see exact names. Ambiguous names (multiple connectors with the same name) will return a validation error.

## When to Use

Use connector commands when you need to access, create, or manage data source connectors. All connector operations that target a specific connector require either `name` + `workspace` or `--id`.

## Discovery Flow

1. `airbyte connectors list-available` -- browse all available connector templates
2. `airbyte connectors list --json '{"workspace": "..."}'` -- see what is already connected
3. `airbyte connectors describe --json '{"workspace": "...", "name": "..."}'` -- inspect a connector's configuration and available entities/actions

## Common Workflows

### List and inspect existing connectors
```
airbyte connectors list --json '{"workspace": "my-workspace"}' --format table
airbyte connectors describe --json '{"workspace": "my-workspace", "name": "my-source"}'
```

### Create a new connector (secure credential flow)
```
airbyte connectors list-available --format table
airbyte connectors create --json '{"workspace": "my-workspace", "template_name": "source-postgres"}'
```
This opens a browser for secure credential entry. The CLI polls until credentials are provided or the flow times out (default: 5 minutes).

### Execute a connector operation
```
# First, discover available entities and actions:
airbyte connectors describe --json '{"workspace": "my-workspace", "name": "my-source"}'

# Then execute with discovered entity and action:
airbyte connectors execute --json '{"workspace": "my-workspace", "name": "my-source", "entity": "users", "action": "read"}'
```

### Filter response fields
```
airbyte connectors execute --json '{"workspace": "my-workspace", "name": "my-source", "entity": "users", "action": "read", "select_fields": ["id", "email", "name"]}'
```

### Delete a connector
```
airbyte connectors delete --json '{"workspace": "my-workspace", "name": "my-source"}'
```

## Error Recovery

- **Name resolution failures** (exit code 3): The connector name was not found. Run `connectors list` to see available connectors. Names are case-insensitive but must match exactly.
- **Ambiguous name** (exit code 4): Multiple connectors share the same name. Use `--id` instead, or rename one of the connectors.
- **Auth errors** (exit code 2): Verify credentials are configured. Run `airbyte enrollment status` to confirm the account is active.
- **Validation errors** (exit code 4): Use `--describe` to check the expected parameter schema before retrying.
- **Credential flow timeout**: The default timeout is 5 minutes. Set `AIRBYTE_CREDENTIAL_TIMEOUT` (in seconds) to increase it.

## Do NOT

- **Do NOT pass credentials as parameters.** Use `connectors create` for the browser-based flow.
- **Do NOT guess entity or action names.** Always run `describe` first to discover the connector's schema.
- **Do NOT skip the describe step.** Entity and action names vary by connector type and are not predictable.
- **Do NOT embed raw connector names in API paths.** The CLI handles name-to-ID resolution automatically via the `resolveConnectorID` hook.

## Hints

- Use `list-available` to find template names before calling `create`
- Use `describe` before the first `execute` to understand available entities and actions
- `workspace` is required for all connector operations (unless using `--id`)
- Use `--format table` for human-readable output; `--format json` for machine parsing
- Use `select_fields` or `exclude_fields` to limit response size on large datasets
