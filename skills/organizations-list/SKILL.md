---
name: organizations-list
description: List organizations the authenticated user belongs to.
command: airbyte-agents organizations list
---

# organizations list

> [!NOTE]
> Requires the `airbyte-agents` CLI on `PATH`. Install via `brew install airbytehq/tap/airbyte-agents` or see the [project README](https://github.com/airbytehq/airbyte-agents-cli#install).

List the organizations that the authenticated principal has access to.

> [!IMPORTANT]
> Always pass parameters as `--json '{...}'` (even when the payload is empty: `--json '{}'`). Agents should not use per-parameter flags.

## Usage

```bash
airbyte-agents organizations list --json '{}'
```

## Filtering output

> [!IMPORTANT]
> When you already know which fields you need, **always pass `--fields`**. Unfiltered list responses waste context window on data you will discard.

Use the global `--fields` flag to trim the response to specific fields. Both forms work because list responses are wrapped in `{"data": [...]}` and the CLI auto-broadcasts row-level paths:

```bash
airbyte-agents organizations list --fields id,organization_name --json '{}'              # short form
airbyte-agents organizations list --fields data.id,data.organization_name --json '{}'    # long form
```

If you mix top-level and row-level paths (e.g. include the cursor), use the long form for the row-level fields:

```bash
airbyte-agents organizations list --fields data.id,next --json '{}'
```

## When to use

Most workflows do not need the organization ID directly — `workspace` is the primary identifier passed to other commands. Use this command when:

- You need to confirm which organization the credentials belong to.
- You are setting `AIRBYTE_ORGANIZATION_ID` and want to verify the value.
- You are debugging multi-org credential setups.

## Hints

- Output is paginated automatically by the CLI; you do not need to handle cursors.
- The organization ID is a UUID; it is rarely needed at the command line.
