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
airbyte connectors execute --workspace my-workspace --name my-source \
  --entity users --action read

# workspace defaults to "default" when omitted
airbyte connectors execute --name my-source --entity users --action read

# JSON form (mutually exclusive with the per-flag form)
airbyte connectors execute --json '{
  "workspace": "my-workspace",
  "name": "my-source",
  "entity": "users",
  "action": "read"
}'
```

`name` (or `--id`), `entity`, and `action` are required. `workspace` is optional and defaults to `default` when used with `--name`; a JSON notice is printed on stderr when the fallback engages.

## Limiting response size

> [!IMPORTANT]
> When you already know which fields you need, **always pass both `select_fields` (API-side) and `--fields` (CLI-side)**. `execute` reads can be huge — unfiltered responses waste both bandwidth and context window.

Two complementary mechanisms:

- **`select_fields` / `exclude_fields` (API-side)** — passed to the source connector to reduce upstream work and bandwidth.
- **`--fields` (client-side)** — shapes the JSON the CLI prints to stdout, after the API responds.

For row-level reads (entities like `contacts`, `users`), responses come wrapped in `{"data": [...]}`. Both forms below work — the CLI auto-broadcasts when no path matches a top-level key:

```bash
# Short form — auto-broadcasts through the data wrapper
airbyte connectors execute --workspace default --name hubspot \
  --entity contacts --action read --fields id,email

# Long form — explicit dotted paths
airbyte connectors execute --workspace default --name hubspot \
  --entity contacts --action read --fields data.id,data.email
```

If you want top-level fields like `next` (cursor) AND row-level fields, you must use the long form for the row-level paths — once any path matches top-level, auto-broadcast is disabled:

```bash
airbyte connectors execute --workspace default --name hubspot \
  --entity contacts --action read \
  --select-fields id,email,name \
  --fields data.id,data.email,next
```

For large reads, restrict API-side fields to keep output manageable:

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
