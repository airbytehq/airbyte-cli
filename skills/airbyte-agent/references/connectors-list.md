# connectors list

List the connectors that already exist in a given workspace.

## Usage

```bash
airbyte-agent connectors list --json '{"workspace": "my-workspace"}'

# workspace defaults to "default" when omitted
airbyte-agent connectors list --json '{}'
```

`workspace` is optional. If omitted, the command falls back to the workspace named `default` and prints a JSON notice on stderr — the API call still proceeds. To target a different workspace, set `"workspace": "<name>"` in the JSON payload.

## When to use

- Confirming a connector exists before calling `describe` or `execute`.
- Discovering exact connector names to pass to other commands.
- Checking the status of existing connectors.

## Filtering output

```bash
airbyte-agent connectors list --fields id,name --json '{}'              # short form
airbyte-agent connectors list --fields data.id,data.name --json '{}'    # long form

# Mixed top-level and row-level paths — use the long form for the row paths
airbyte-agent connectors list --fields data.id,next --json '{}'
```

## Related commands

- `connectors list-available` — list templates available to install (different command, different purpose).
- `connectors describe` — inspect a specific connector's entities and actions.
- `connectors create` — install a new connector from a template.

## Hints

- Names returned here can be matched in subsequent commands by connector instance name, template display name, OR template slug — all case-insensitive.
- If two connectors share a name, `execute`/`describe`/`delete` will return a validation error — pass `"id": "<uuid>"` in the JSON payload instead.
