---
name: workspaces-list
description: List and filter workspaces. Workspace names are the primary identifier for connector commands.
command: airbyte workspaces list
---

# workspaces list

List workspaces in the organization. Workspace names are the identifier passed to almost every connector command, so this is typically the second command in a session (after `enrollment status`).

> [!IMPORTANT]
> If only one workspace exists, use it directly without prompting the user. Most accounts have a single workspace.

> [!NOTE]
> Pagination is automatic — all workspaces are returned in a single response regardless of server-side page size.

## Usage

```
airbyte workspaces list --format table
airbyte workspaces list --json '{"name_contains": "production"}'
airbyte workspaces list --json '{"status": "active"}'
```

Run `airbyte workspaces list --describe` to see the full parameter schema.

## Discovery flow

1. `airbyte workspaces list --format table` — see all workspaces.
2. Note the exact `name` value.
3. Pass that name into subsequent commands: `--json '{"workspace": "<name>"}'`.

## Do NOT

- Do NOT prompt the user to pick a workspace if only one exists.
- Do NOT assume workspace names — always discover them first.
- Do NOT pass workspace UUIDs to commands that accept `workspace` — the CLI expects the human-readable name.

## Hints

- Use `name_contains` for partial matching when the exact name is unknown.
- The `limit` parameter controls server-side page size; results are still returned in full.
