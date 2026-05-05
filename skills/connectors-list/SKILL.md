---
name: connectors-list
description: List connectors configured in a workspace.
command: airbyte connectors list
---

# connectors list

List the connectors that already exist in a given workspace.

## Usage

```
airbyte connectors list --workspace my-workspace
airbyte connectors list --workspace my-workspace --format table
airbyte connectors list                              # falls back to workspace="default"
```

`workspace` is optional. If omitted, the command falls back to the workspace named `default` and prints a JSON notice on stderr — the API call still proceeds. To target a different workspace, pass `--workspace <name>` (or use `--json '{"workspace": "..."}'`).

## When to use

- Confirming a connector exists before calling `describe` or `execute`.
- Discovering exact connector names to pass to other commands.
- Checking the status of existing connectors.

## Related commands

- `connectors list-available` — list templates available to install (different command, different purpose).
- `connectors describe` — inspect a specific connector's entities and actions.
- `connectors create` — install a new connector from a template.

## Filtering output

> [!IMPORTANT]
> When you already know which fields you need, **always pass `--fields`**. Unfiltered list responses waste context window on data you will discard.

Use the global `--fields` flag to trim the response. Both forms work because list responses are wrapped in `{"data": [...]}` and the CLI auto-broadcasts row-level paths:

```
airbyte connectors list --fields id,name              # short form
airbyte connectors list --fields data.id,data.name    # long form
```

If you mix top-level and row-level paths (e.g. include the cursor), use the long form for the row-level fields:

```
airbyte connectors list --fields data.id,next
```

## Hints

- Names listed here are case-insensitive but must match exactly when used elsewhere.
- If two connectors share a name, `execute`/`describe`/`delete` will return a validation error — use `--id` instead.
