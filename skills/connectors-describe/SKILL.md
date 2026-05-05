---
name: connectors-describe
description: Inspect a connector's entities and actions. Always run before the first `execute`.
command: airbyte connectors describe
---

# connectors describe

Show a connector's available entities (e.g. `users`, `contacts`, `orders`) and the actions supported on each (e.g. `read`, `write`).

> [!IMPORTANT]
> **Always describe before execute.** Entity and action names vary by connector type and are not predictable. Do NOT guess them.

## Usage

```
airbyte connectors describe --workspace my-workspace --name my-source
airbyte connectors describe --name my-source                  # workspace defaults to "default"
airbyte connectors describe --id <connector-id>
```

`workspace` is optional. If omitted while using `--name`, the command falls back to the workspace named `default` and prints a JSON notice on stderr.

## When to use

- Before the first `execute` on any connector.
- When you need to discover what fields an entity exposes.
- When debugging a `not_found` or `validation_error` from `execute`.

## Workflow

```
# 1. Find the connector
airbyte connectors list --json '{"workspace": "my-workspace"}' --format table

# 2. Describe it
airbyte connectors describe --json '{"workspace": "my-workspace", "name": "my-source"}'

# 3. Execute the discovered entity + action
airbyte connectors execute --json '{"workspace": "my-workspace", "name": "my-source", "entity": "users", "action": "read"}'
```

## Do NOT

- Do NOT skip this step before the first `execute` — guessing entity/action names wastes API calls and confuses the user.
- Do NOT cache describe output across CLI versions — the schema can change when connectors update.
