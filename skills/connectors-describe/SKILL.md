---
name: connectors-describe
description: Inspect a connector's entities and actions. Always run before the first `execute`.
command: airbyte-agent connectors describe
---

# connectors describe

> [!NOTE]
> Requires the `airbyte-agent` CLI on `PATH`. Install via `brew install airbytehq/tap/airbyte-agent` or see the [project README](https://github.com/airbytehq/airbyte-agent-cli#install).

Show a connector's available entities (e.g. `users`, `contacts`, `orders`) and the actions supported on each (e.g. `read`, `write`).

> [!IMPORTANT]
> Always pass parameters as `--json '{...}'`. Agents should not use per-parameter flags.

> [!IMPORTANT]
> **Always describe before execute.** Entity and action names vary by connector type and are not predictable. Do NOT guess them.

## Usage

```bash
airbyte-agent connectors describe --json '{"workspace": "my-workspace", "name": "my-source"}'

# workspace defaults to "default" when omitted
airbyte-agent connectors describe --json '{"name": "my-source"}'

# By connector ID instead of name
airbyte-agent connectors describe --json '{"id": "<connector-id>"}'
```

`workspace` is optional; if omitted while using `name`, the command falls back to the workspace named `default` and prints a JSON notice on stderr.

## When to use

- Before the first `execute` on any connector.
- When you need to discover what fields an entity exposes.
- When debugging a `not_found` or `validation_error` from `execute`.

## Workflow

```bash
# 1. Find the connector
airbyte-agent connectors list --json '{"workspace": "my-workspace"}'

# 2. Describe it
airbyte-agent connectors describe --json '{"workspace": "my-workspace", "name": "my-source"}'

# 3. Execute the discovered entity + action
airbyte-agent connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "users",
  "action": "context_store_search",
  "select_fields": ["id", "email"]
}'
```

## Do NOT

- Do NOT skip this step before the first `execute` — guessing entity/action names wastes API calls and confuses the user.
- Do NOT cache describe output across CLI versions — the schema can change when connectors update.
