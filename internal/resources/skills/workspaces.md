# Workspaces

Discover and manage workspaces within an organization.

> [!IMPORTANT]
> **Single workspace shortcut.** If only one workspace exists, use it directly without prompting the user to choose. Most accounts have a single workspace.

> [!NOTE]
> **Pagination is automatic.** The CLI follows cursor-based pagination internally and returns all workspaces in a single response, regardless of how many pages exist on the server. You do not need to handle pagination.

## When to Use

Use workspace commands when you need to:
- Discover available workspaces during initial setup
- Find the correct `workspace` to pass to connector commands
- Filter workspaces by name or status

Workspace names are the primary identifier used by other commands (e.g., `connectors list`, `connectors execute`). Always resolve the workspace name before proceeding with connector operations.

## Discovery Flow

1. `airbyte workspaces list --format table` -- list all workspaces with human-readable output
2. Note the exact `name` value from the output
3. Use that name in subsequent connector commands: `--json '{"workspace": "<name>"}'`

## Common Workflows

### List all workspaces
```
airbyte workspaces list --format table
```

### Filter workspaces by name
```
airbyte workspaces list --json '{"name_contains": "production"}'
```

### Filter workspaces by status
```
airbyte workspaces list --json '{"status": "active"}'
```

### Full discovery flow (workspace -> connectors)
```
airbyte workspaces list --format table
airbyte connectors list --json '{"workspace": "<name from above>"}'
airbyte connectors describe --json '{"workspace": "<name>", "name": "<connector>"}'
```

## Do NOT

- **Do NOT prompt the user to select a workspace if only one exists.** Use it automatically.
- **Do NOT assume workspace names.** Always discover them via `workspaces list` first.
- **Do NOT use workspace IDs in commands that accept `workspace`.** The CLI expects the human-readable name, not the UUID.

## Hints

- Workspace names are used as identifiers in other commands -- note the exact name from `list` output
- Use `name_contains` for partial matching when unsure of the exact workspace name
- Use `--format table` for readable output, `--format json` (default) for programmatic use
- The `limit` parameter controls page size but all results are returned regardless
