# Getting Started

Initial setup and authentication configuration for the Airbyte CLI.

> [!IMPORTANT]
> **Always check enrollment first.** Before running any other command, verify the account is enrolled and provisioned with `airbyte enrollment status`. If `provisioning_state` is `IN_PROGRESS`, poll with backoff until it completes. Do NOT proceed with other commands until enrollment shows `is_enrolled: true`.

> [!IMPORTANT]
> **Never ask users for credentials directly.** Do not accept API keys, tokens, passwords, or secrets as parameters or in conversation. All credential entry goes through the secure browser-based flow via `airbyte connectors create`. If a user offers credentials, decline and start the credential flow instead.

## Auth Configuration

Set credentials using environment variables:
```
export AIRBYTE_CLIENT_ID="your-client-id"
export AIRBYTE_CLIENT_SECRET="your-client-secret"
```

Or create a credentials file at `~/.airbyte/credentials`:
```json
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret"
}
```

Optionally set the organization ID:
```
export AIRBYTE_ORGANIZATION_ID="your-org-id"
```

Or include it in the credentials file:
```json
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret",
  "organization_id": "your-org-id"
}
```

## First Commands

Follow this exact sequence for first-time setup:

1. **Check enrollment**: `airbyte enrollment status`
   - Expected: `is_enrolled: true`, `provisioning_state: COMPLETED`
   - If `provisioning_state` is `IN_PROGRESS`, wait and retry (see discovery skill)
   - If `provisioning_state` is `FAILED`, stop -- manual intervention required

2. **List workspaces**: `airbyte workspaces list --format table`
   - If only one workspace exists, use it directly for all subsequent commands
   - Note the exact `name` value -- it is used as an identifier in other commands

3. **List connectors**: `airbyte connectors list --json '{"workspace": "..."}'`
   - Shows all connectors in the workspace
   - If no connectors exist, use `connectors create` to set one up

## Schema Discovery

Every command supports `--describe` to return its input schema without executing the operation:
```
airbyte connectors execute --describe
airbyte workspaces list --describe
```

This returns a JSON schema describing the accepted parameters, their types, and whether they are required. Always use `--describe` before calling an unfamiliar command.

## Error Handling

All errors are returned as JSON on stderr:
```json
{"type": "<error_type>", "message": "...", "status_code": 400, "retryable": false}
```

Exit codes:
- `0` -- success
- `1` -- general error
- `2` -- authentication error
- `3` -- not found
- `4` -- validation error

## Do NOT

- **Do NOT skip the enrollment check.** Commands will fail with auth errors if the account is not provisioned.
- **Do NOT hardcode workspace names.** Always discover them via `workspaces list` first.
- **Do NOT retry auth errors (exit code 2) without re-checking credentials.** Auth failures indicate misconfigured credentials, not transient issues.
- **Do NOT guess parameter names.** Use `--describe` on any command to see the exact schema.
- **Do NOT pass `--format table` when the output will be parsed programmatically.** Use `--format json` (default) for machine consumption.

## Hints

- Use `--format table` for human-readable output
- Use `--format json` (default) for machine-parseable output
- Use `--json @file.json` to load complex parameters from a file
- Use `--describe` on any command to inspect its parameter schema before calling it
- Use `--id` as a convenience flag when you already have a resource ID
