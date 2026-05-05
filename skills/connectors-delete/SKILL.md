---
name: connectors-delete
description: Permanently delete a connector from a workspace.
command: airbyte connectors delete
---

# connectors delete

Permanently delete a connector from a workspace.

> [!IMPORTANT]
> Deletion is irreversible. Confirm with the user before running this command unless they have explicitly authorized it.

## Usage

```
airbyte connectors delete --json '{"workspace": "my-workspace", "name": "my-source"}'
airbyte connectors delete --id <connector-id>
```

## Error recovery

- **Not found** (exit 3): run `connectors list` to confirm the name exists in the workspace.
- **Ambiguous name** (exit 4): two connectors share a name — use `--id` instead.

## Do NOT

- Do NOT delete a connector without explicit user confirmation.
- Do NOT use this command to "reset" a connector's credentials — instead, delete and recreate, or update credentials directly via the connector configuration flow.
