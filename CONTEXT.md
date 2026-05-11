# Airbyte Agents CLI Context

This document tells AI agents how to use the `airbyte-agent` CLI. For development/architecture details, see `AGENTS.md`.

## Rules of Engagement

> [!IMPORTANT]
> **Schema Discovery**: If you don't know the exact JSON payload structure for a command, run it with `--describe` first. This returns the parameter schema without executing the operation.

> [!IMPORTANT]
> **Always filter responses to the fields you need.** Whenever you know which fields will satisfy the user's request, pass `--fields` to trim the output. This applies to **every command** — list, describe, execute, etc. Unfiltered responses waste context window and bandwidth on data you will discard anyway. The only time to skip the filter is when you genuinely need the full payload (e.g. one-shot debugging, or you don't yet know which fields exist — in which case run `--describe` or do a small probe call first).
>
> For row-level reads via `connectors execute`, also pass `select_fields` (API-side) to reduce upstream work. `select_fields` and `--fields` are complementary: the first stops the source connector from emitting columns you don't need; the second trims what the CLI prints to stdout.

> [!IMPORTANT]
> **Discover before executing**: Always run `connectors describe` before the first `execute` on any connector. Entity and action names vary by connector type and are not guessable.

## Core Syntax

```bash
airbyte-agent <resource> <operation> [flags]
```

All parameters are passed via `--json '<JSON>'` or `--id '<ID>'`. Output goes to stdout as JSON (default) or table format.

```bash
airbyte-agent --help                        # List all resources
airbyte-agent <resource> --help             # List operations for a resource
airbyte-agent <resource> <operation> --describe  # Show parameter schema
```

### Key Flags

| Flag | Description | Default |
| --- | --- | --- |
| `--json` | Inline JSON parameters | -- |
| `--id` | Convenience flag for resource ID | -- |
| `--format` | Output format: `json` or `table` | `json` |
| `--describe` | Print operation schema and exit (do not execute) | `false` |
| `--output, -o` | Write output to file instead of stdout | -- |
| `--verbose, -v` | Enable debug logging | `false` |
| `--fields` | Filter response to listed fields (comma-separated dotted paths, e.g. `data.id,data.name`). Client-side; not applied to errors. | -- |

## Usage Patterns

### 1. First-Time Setup

```bash
# Configure credentials interactively
airbyte-agent configure

# Verify enrollment
airbyte-agent enroll

# Find your workspace
airbyte-agent workspaces list --format table
```

### 2. Listing and Discovering Connectors

```bash
# List connectors in a workspace
airbyte-agent connectors list --json '{"workspace": "my-workspace"}'

# List available connector templates (for creating new connectors)
airbyte-agent connectors list-available --format table

# Describe a connector to see its entities and actions
airbyte-agent connectors describe --json '{"workspace": "my-workspace", "name": "my-source"}'

# Or by ID
airbyte-agent connectors describe --id 'f24fb2b0-c054-48f1-9e0f-cfb62e12f878'
```

### 3. Executing Connector Actions

Always `describe` first to discover available entities and actions.

```bash
# Read data from a connector
airbyte-agent connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "users",
  "action": "read"
}'

# With parameters
airbyte-agent connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "deals",
  "action": "search",
  "params": {"query": "status:open"}
}'

# Limit response fields to protect context window
airbyte-agent connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "contacts",
  "action": "read",
  "select_fields": ["id", "email", "name"]
}'

# Exclude heavy fields
airbyte-agent connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "messages",
  "action": "read",
  "exclude_fields": ["body_html", "attachments"]
}'
```

### 4. Creating a New Connector

```bash
# Browse available templates
airbyte-agent connectors list-available --format table

# Create (opens browser for secure credential entry)
airbyte-agent connectors create --json '{
  "workspace": "my-workspace",
  "name": "hubspot"
}'
```

### 5. Deleting a Connector

```bash
airbyte-agent connectors delete --json '{"workspace": "my-workspace", "name": "old-source"}'
```

Delete is destructive and prompts for an interactive `"Type 'yes' to confirm:"` on a TTY. Without a TTY (e.g. piped agent input), the command refuses with a `validation_error` whose hint tells you to set `"allow_destructive": true` in `~/.airbyte-agent/settings.json` (or `AIRBYTE_ALLOW_DESTRUCTIVE=true`). Once that permission is granted, the prompt is skipped.

### 6. Schema Introspection

Use `--describe` on any command to see its parameter schema before calling it:

```bash
airbyte-agent connectors execute --describe
# Returns:
# {
#   "description": "Execute an action on a connector",
#   "params": {
#     "name": {"type": "string", "required": false, "description": "Connector name (requires workspace)"},
#     "workspace": {"type": "string", "required": false, "description": "Workspace name (required when using name)"},
#     "id": {"type": "string", "required": false, "description": "Connector ID (alternative to name)"},
#     "entity": {"type": "string", "required": true, "description": "Entity name"},
#     "action": {"type": "string", "required": true, "description": "Action name"},
#     ...
#   }
# }
```

### 7. Loading Parameters from a File

For complex JSON payloads, use `@filename`:

```bash
echo '{"workspace": "my-workspace", "name": "my-source", "entity": "users", "action": "read"}' > params.json
airbyte-agent connectors execute --json @params.json
```

## Error Handling

All errors are JSON on stderr with an exit code:

| Exit Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | General error |
| `2` | Authentication error |
| `3` | Not found |
| `4` | Validation error |

Errors include a `hint` field with actionable guidance:

```json
{
  "error": "not_found",
  "message": "connector \"gong\" not found in workspace \"default\"",
  "status_code": 404,
  "hint": "run 'airbyte-agent connectors list --json '{\"workspace\": \"default\"}'' to see available connectors"
}
```

API errors (400/422) include the full server response in `detail`:

```json
{
  "error": "validation_error",
  "message": "Invalid configuration",
  "status_code": 400,
  "detail": {"errors": [{"field": "host", "message": "is required"}]}
}
```

When you see a validation error with missing fields, use `--describe` to check the schema:

```json
{
  "error": "validation_error",
  "fields": {"entity": "required", "action": "required"},
  "hint": "run this command with --describe to see the expected parameter schema"
}
```
