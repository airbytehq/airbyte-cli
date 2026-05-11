---
name: connectors-list-available
description: List connector templates that can be installed via `connectors create`.
command: airbyte-agents connectors list-available
---

# connectors list-available

> [!NOTE]
> Requires the `airbyte-agents` CLI on `PATH`. Install via `brew install airbytehq/tap/airbyte-agents` or see the [project README](https://github.com/airbytehq/airbyte-agents-cli#install).

List the connector templates available to install in this account. Each template has a `name` (e.g. `salesforce`, `hubspot`) that you pass to `connectors create --json '{"name": "<name>"}'`.

> [!IMPORTANT]
> Always pass parameters as `--json '{...}'` (even when the payload is empty: `--json '{}'`). Agents should not use per-parameter flags.

## Usage

```bash
airbyte-agents connectors list-available --json '{}'
```

## When to use

Always run this **before** `connectors create` to discover the exact template `name` to use. Template names are stable identifiers — do not guess them.

## Workflow

```bash
airbyte-agents connectors list-available --json '{}'
airbyte-agents connectors create --json '{"workspace": "my-workspace", "name": "salesforce"}'
```

## Filtering output

> [!IMPORTANT]
> When you already know which fields you need, **always pass `--fields`**. Unfiltered list responses waste context window on data you will discard.

Use the global `--fields` flag to trim the response. Both forms work because list responses are wrapped in `{"data": [...]}` and the CLI auto-broadcasts row-level paths:

```bash
airbyte-agents connectors list-available --fields id,name --json '{}'              # short form
airbyte-agents connectors list-available --fields data.id,data.name --json '{}'    # long form
```

## Hints

- The list is filtered to what your account has access to — it will not show every connector that exists in Airbyte's catalog.
