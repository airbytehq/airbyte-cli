# Discovery

First-time setup, enrollment verification, and organization listing.

> [!IMPORTANT]
> **Provisioning takes 10-30 seconds.** When `provisioning_state` is `IN_PROGRESS`, poll with exponential backoff (2s, 4s, 8s, 16s). Do NOT poll more frequently than every 2 seconds. Do NOT proceed with other commands until provisioning completes.

> [!IMPORTANT]
> **If provisioning fails, stop.** When `provisioning_state` is `FAILED`, do not retry automatically. The account needs manual intervention. Inform the user and do not attempt further API calls.

## When to Use

Use discovery commands during:
- **Initial setup**: First time using the CLI
- **Account verification**: Checking if credentials and enrollment are valid
- **Organization discovery**: Determining which organization and workspace to target

This is always the first step before any connector operations.

## Discovery Flow

Follow this exact sequence:

1. **Check enrollment**: `airbyte enrollment status`
   - If `is_enrolled: false` and `provisioning_state: null` or `IN_PROGRESS`, poll with backoff
   - If `is_enrolled: true` and `provisioning_state: COMPLETED`, proceed to step 2
   - If `provisioning_state: FAILED`, stop and inform the user

2. **List organizations**: `airbyte organizations list --format table`
   - The organization ID is used internally; you typically only need the workspace name

3. **List workspaces**: `airbyte workspaces list --format table`
   - If only one workspace exists, use it directly
   - If multiple exist, ask the user which to use

4. **Proceed with connector commands** using the discovered `workspace`

## Provisioning State Machine

```
null -> IN_PROGRESS -> COMPLETED
                    -> FAILED
```

### Polling Strategy

When `provisioning_state` is `IN_PROGRESS`, use exponential backoff:

| Attempt | Delay |
| --- | --- |
| 1 | 2 seconds |
| 2 | 4 seconds |
| 3 | 8 seconds |
| 4+ | 16 seconds |

Provisioning typically completes within 10-30 seconds. If it has not completed after 2 minutes of polling, inform the user that provisioning is taking longer than expected.

## Common Workflows

### First-time setup (complete flow)
```
airbyte enrollment status
airbyte organizations list --format table
airbyte workspaces list --format table
airbyte connectors list --json '{"workspace": "..."}'
```

### Re-check provisioning
```
airbyte enrollment status
```

## Do NOT

- **Do NOT skip the enrollment check.** Other commands will fail with auth or permission errors if the account is not provisioned.
- **Do NOT poll enrollment faster than every 2 seconds.** This wastes API calls and does not speed up provisioning.
- **Do NOT proceed if provisioning failed.** The account needs manual intervention.
- **Do NOT cache enrollment status across sessions.** Always verify at the start of a new session.

## Hints

- Enrollment status triggers provisioning for new users automatically on first check
- The organization ID from `organizations list` is used internally; you typically only need `workspace`
- If only one workspace exists, use it directly without prompting
- Poll enrollment status if `provisioning_state` is `IN_PROGRESS` -- it will transition to `COMPLETED` or `FAILED`
