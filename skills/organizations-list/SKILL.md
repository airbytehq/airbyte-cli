---
name: organizations-list
description: List organizations the authenticated user belongs to.
command: airbyte organizations list
---

# organizations list

List the organizations that the authenticated principal has access to.

## Usage

```
airbyte organizations list
airbyte organizations list --format table
```

## When to use

Most workflows do not need the organization ID directly — `workspace` is the primary identifier passed to other commands. Use this command when:

- You need to confirm which organization the credentials belong to.
- You are setting `AIRBYTE_ORGANIZATION_ID` and want to verify the value.
- You are debugging multi-org credential setups.

## Hints

- Output is paginated automatically by the CLI; you do not need to handle cursors.
- The organization ID is a UUID; it is rarely needed at the command line.
