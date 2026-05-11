---
name: enroll
description: Verify (and trigger) account enrollment.
command: airbyte-agents enroll
---

# enroll

> [!NOTE]
> Requires the `airbyte-agents` CLI on `PATH`. Install via `brew install airbytehq/tap/airbyte-agents` or see the [project README](https://github.com/airbytehq/airbyte-agents-cli#install).

Check whether the account is enrolled and what its provisioning state is. **Always the first command in any session** — every other command will fail if the account is not provisioned.

The first call for a new account *also triggers* enrollment automatically — this is why the command is named `enroll` rather than `enrollment status`. Polling the same command moves the account through provisioning to completion.

> [!IMPORTANT]
> If `provisioning_state` is `IN_PROGRESS`, poll with exponential backoff until it transitions. Do NOT proceed with other commands until `is_enrolled: true`.

> [!IMPORTANT]
> If `provisioning_state` is `FAILED`, stop. The account needs manual intervention — do not retry automatically.

## Usage

```bash
airbyte-agents enroll
```

The command takes no parameters. Returns a JSON document with `is_enrolled` (bool) and `provisioning_state` (one of `null`, `IN_PROGRESS`, `COMPLETED`, `FAILED`), plus organization metadata when enrollment is complete.

## Provisioning state machine

```
null → IN_PROGRESS → COMPLETED
                  → FAILED
```

Provisioning typically completes within 10–30 seconds. The first call to `enroll` for a new account triggers provisioning automatically.

## Polling strategy

When `provisioning_state` is `IN_PROGRESS`, use exponential backoff:

| Attempt | Delay |
| --- | --- |
| 1 | 2s |
| 2 | 4s |
| 3 | 8s |
| 4+ | 16s |

If still `IN_PROGRESS` after ~2 minutes, inform the user — provisioning is taking longer than expected.

## Do NOT

- Do NOT poll faster than every 2 seconds.
- Do NOT proceed if provisioning failed.
- Do NOT cache enrollment status across sessions — verify at the start of each session.
