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
airbyte connectors delete --workspace my-workspace --name my-source
airbyte connectors delete --name my-source                # workspace defaults to "default"
airbyte connectors delete --id <connector-id>
```

`workspace` is optional. If omitted while using `--name`, the command falls back to the workspace named `default` and prints a JSON notice on stderr. **Confirm with the user before relying on the fallback for a delete** — operating on the wrong workspace's connector is hard to recover from.

## Error recovery

- **Not found** (exit 3): run `connectors list` to confirm the name exists in the workspace.
- **Ambiguous name** (exit 4): two connectors share a name — use `--id` instead.

## Do NOT

- Do NOT delete a connector without explicit user confirmation.
- Do NOT use this command to "reset" a connector's credentials — instead, delete and recreate, or update credentials directly via the connector configuration flow.
