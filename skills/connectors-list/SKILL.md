---
name: connectors-list
description: List connectors configured in a workspace.
command: airbyte-agents connectors list
---

# connectors list

> [!NOTE]
> Requires the `airbyte-agents` CLI on `PATH`. Install via `brew install airbytehq/tap/airbyte-agents` or see the [project README](https://github.com/airbytehq/airbyte-agents-cli#install).

List the connectors that already exist in a given workspace.

> [!IMPORTANT]
> Always pass parameters as `--json '{...}'`. Per-parameter flags exist for human use; agents should use a single JSON payload for predictable, reviewable inputs.

## Usage

```bash
airbyte-agents connectors list --json '{"workspace": "my-workspace"}'

# workspace defaults to "default" when omitted
airbyte-agents connectors list --json '{}'
```

`workspace` is optional. If omitted, the command falls back to the workspace named `default` and prints a JSON notice on stderr — the API call still proceeds. To target a different workspace, set `"workspace": "<name>"` in the JSON payload.

## When to use

- Confirming a connector exists before calling `describe` or `execute`.
- Discovering exact connector names to pass to other commands.
- Checking the status of existing connectors.

## Filtering output

> [!IMPORTANT]
> When you already know which fields you need, **always pass `--fields`**. Unfiltered list responses waste context window on data you will discard.

Use the global `--fields` flag to trim the response. Both forms work because list responses are wrapped in `{"data": [...]}` and the CLI auto-broadcasts row-level paths:

```bash
airbyte-agents connectors list --fields id,name --json '{}'              # short form
airbyte-agents connectors list --fields data.id,data.name --json '{}'    # long form
```

If you mix top-level and row-level paths (e.g. include the cursor), use the long form for the row-level fields:

```bash
airbyte-agents connectors list --fields data.id,next --json '{}'
```

## Related commands

- `connectors list-available` — list templates available to install (different command, different purpose).
- `connectors describe` — inspect a specific connector's entities and actions.
- `connectors create` — install a new connector from a template.

## Hints

- Names returned here can be matched in subsequent commands by connector instance name, template display name, OR template slug — all case-insensitive.
- If two connectors share a name, `execute`/`describe`/`delete` will return a validation error — pass `"id": "<uuid>"` in the JSON payload instead.
