---
name: connectors-list
description: List connectors configured in a workspace.
command: airbyte connectors list
---

# connectors list

List the connectors that already exist in a given workspace.

## Usage

```
airbyte connectors list --json '{"workspace": "my-workspace"}'
airbyte connectors list --json '{"workspace": "my-workspace"}' --format table
```

`workspace` is required. Run with `--describe` to see the full parameter schema (filter, status, etc.).

## When to use

- Confirming a connector exists before calling `describe` or `execute`.
- Discovering exact connector names to pass to other commands.
- Checking the status of existing connectors.

## Related commands

- `connectors list-available` — list templates available to install (different command, different purpose).
- `connectors describe` — inspect a specific connector's entities and actions.
- `connectors create` — install a new connector from a template.

## Hints

- Names listed here are case-insensitive but must match exactly when used elsewhere.
- If two connectors share a name, `execute`/`describe`/`delete` will return a validation error — use `--id` instead.
