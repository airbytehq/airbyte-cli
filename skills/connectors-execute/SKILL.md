---
name: connectors-execute
description: Run an action (read/write) against an entity on a connector.
command: airbyte connectors execute
---

# connectors execute

Run an action against an entity on a connector — the workhorse command for actually moving data.

> [!IMPORTANT]
> Always run `connectors describe` first to discover valid entities and actions. Do NOT guess.

## Usage

```
airbyte connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "users",
  "action": "read"
}'
```

`workspace` + `name` (or `--id`), `entity`, and `action` are required.

## Limiting response size

For large reads, restrict fields to keep output manageable:

```
airbyte connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "contacts",
  "action": "read",
  "select_fields": ["id", "email", "name"]
}'
```

Or exclude specific fields:

```
{ ..., "exclude_fields": ["raw_html", "internal_notes"] }
```

## Error recovery

| Error | Likely cause | Fix |
| --- | --- | --- |
| `not_found` (exit 3) | Connector name not found | Run `connectors list` to see exact names |
| `validation_error` (exit 4) on entity/action | Guessed name | Run `connectors describe` first |
| Ambiguous name (exit 4) | Two connectors share a name | Use `--id` instead |
| `auth_error` (exit 2) | Credentials invalid or expired | Re-check `enrollment status` |

## Do NOT

- Do NOT guess entity or action names.
- Do NOT pass credentials in the `execute` payload — credentials live on the connector and are set via `connectors create`.
- Do NOT request unbounded reads on large entities without `select_fields` / `exclude_fields`.
