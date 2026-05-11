---
name: workspaces-list
description: List and filter workspaces. Workspace names are the primary identifier for connector commands.
command: airbyte-agent workspaces list
---

# workspaces list

> [!NOTE]
> Requires the `airbyte-agent` CLI on `PATH`. Install via `brew install airbytehq/tap/airbyte-agent` or see the [project README](https://github.com/airbytehq/airbyte-agent-cli#install).

List workspaces in the organization. Workspace names are the identifier passed to almost every connector command, so this is typically the second command in a session (after `airbyte-agent enroll`).

> [!IMPORTANT]
> Always pass parameters as `--json '{...}'` (use `--json '{}'` for an unfiltered list). Agents should not use per-parameter flags.

> [!IMPORTANT]
> If only one workspace exists, use it directly without prompting the user. Most accounts have a single workspace.

> [!NOTE]
> Pagination is automatic — all workspaces are returned in a single response regardless of server-side page size.

## Usage

```bash
airbyte-agent workspaces list --json '{}'
airbyte-agent workspaces list --json '{"name_contains": "production"}'
airbyte-agent workspaces list --json '{"status": "active"}'
```

Run `airbyte-agent schema workspaces list` to see the full parameter schema.

## Filtering output

> [!IMPORTANT]
> When you already know which fields you need, **always pass `--fields`**. Unfiltered list responses waste context window on data you will discard.

Use the global `--fields` flag to trim the response. Both forms work because list responses are wrapped in `{"data": [...]}` and the CLI auto-broadcasts row-level paths:

```bash
airbyte-agent workspaces list --fields name,status --json '{}'              # short form
airbyte-agent workspaces list --fields data.name,data.status --json '{}'    # long form
```

If you mix top-level and row-level paths, use the long form for row-level fields:

```bash
airbyte-agent workspaces list --fields data.name,next --json '{}'
```

## Discovery flow

1. `airbyte-agent workspaces list --json '{}'` — see all workspaces.
2. Note the exact `name` value.
3. Either:
   - Pass that name into each command: `--json '{"workspace": "<name>"}'`, or
   - Persist it as the default once: `airbyte-agent workspaces use --json '{"name": "<name>"}'`. Subsequent commands will fall back to this when `workspace` is omitted.

## Do NOT

- Do NOT prompt the user to pick a workspace if only one exists.
- Do NOT assume workspace names — always discover them first.
- Do NOT pass workspace UUIDs to commands that accept `workspace` — the CLI expects the human-readable name.

## Hints

- Use `name_contains` for partial matching when the exact name is unknown.
- The `limit` parameter controls server-side page size; results are still returned in full.
