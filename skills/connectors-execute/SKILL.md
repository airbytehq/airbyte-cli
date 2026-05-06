---
name: connectors-execute
description: Run an action (list/get/search/create/update) against an entity on a connector. The workhorse command — read this skill before calling.
command: airbyte connectors execute
---

# connectors execute

Run an action against an entity on a connector — the workhorse command for actually moving data. This skill embeds the same knowledge that the Sonar SDK injects into agent context for the underlying API; you can invoke `execute` correctly without first running `connectors describe` for anything except entity discovery on an unfamiliar connector.

> [!IMPORTANT]
> **Always pass parameters as `--json '{...}'`.** The CLI also exposes per-parameter flags (`--workspace`, `--name`, etc.) for human use, but agents should always send a single JSON payload. The two modes are mutually exclusive and JSON keeps your input self-describing for review and replay.

> [!IMPORTANT]
> **Field selection is MANDATORY.** Every call MUST include `select_fields` (allowlist) or `exclude_fields` (blocklist). Unfiltered responses waste both bandwidth and context window. Both support dot-notation for nested fields (e.g. `billing.address.city`). If both are passed, `select_fields` wins.

> [!IMPORTANT]
> **Prefer `context_store_search` over `list`.** The default action for any read is `context_store_search` — it supports filtering, sorting, and pagination. Only use `list` when (a) you need today's data (the search index can lag hours), or (b) `context_store_search` returns no results and you suspect indexing delay.

## Usage

```bash
airbyte connectors execute --json '{
  "workspace": "default",
  "name": "hubspot",
  "entity": "contacts",
  "action": "context_store_search",
  "select_fields": ["id", "email", "firstName"],
  "params": {"limit": 20, "query": {"filter": {"fuzzy": {"firstName": "Teo"}}}}
}'
```

`name` (or `id`), `entity`, and `action` are required. `workspace` defaults to `default` when omitted. Pass complex payloads via `--json @path/to/file.json` to keep the shell command short.

## Available actions

Every connector exposes these actions. Entities vary per connector (run `connectors describe` to discover them on an unfamiliar source).

| Action | Purpose | Supports filtering? |
|---|---|---|
| `context_store_search` | **Default for reads.** Filter, sort, paginate over the indexed entity store. | yes (rich) |
| `list` | Live read from the source. Use when search index may lag or returns empty. | limited |
| `get` | Fetch a single entity by ID. | n/a |
| `api_search` | Provider-native search (e.g. Slack search syntax). Returns `{data, meta: {has_more}}`. | provider-specific |
| `create` | Write a new entity. | n/a |
| `update` | Modify an existing entity. | n/a |

## Response structure

```jsonc
// list / api_search / context_store_search
{ "data": [ ... ], "meta": { "has_more": true } }

// get — returns the entity directly, no envelope
{ "id": "...", ... }
```

To paginate, pass `cursor=<last_cursor_value>` in `params` while `has_more` is true.

## How to use `context_store_search`

`action=context_store_search` reads `params.query` with `filter`, `sort`, and `limit`:

```jsonc
// Basic filter
{"action": "context_store_search", "params": {"limit": 20, "query": {"filter": {"eq": {"status": "active"}}}}}

// Filter + sort
{"action": "context_store_search", "params": {"limit": 20, "query": {"filter": {...}, "sort": [{"created": "desc"}]}}}
```

**Always prefer `fuzzy` over `like` when searching for text.** `fuzzy` matches words in any order, ignores punctuation/casing, and handles partial names. `like` requires an exact substring match and fails on typos or word reordering. Only fall back to `like` when you need exact substring matching (e.g. prefix search on IDs).

```jsonc
// Find a user by name — use fuzzy
"params": {"query": {"filter": {"fuzzy": {"firstName": "Teo"}}}}

// Find an external ID with a known prefix — use like
"params": {"query": {"filter": {"like": {"externalId": "CUS-"}}}}
```

## Filter operators

The operator is the **outer key**; `field: value` is nested inside. All examples below go inside `params.query.filter`:

| Operator | Meaning | Example |
|---|---|---|
| `eq` | Exact match | `{"eq": {"status": "completed"}}` |
| `neq` | Not equal | `{"neq": {"status": "deleted"}}` |
| `gt` / `gte` | Greater / greater-or-equal | `{"gte": {"started": "2026-01-01T00:00:00Z"}}` |
| `lt` / `lte` | Less / less-or-equal | `{"lt": {"amount": 1000}}` |
| `in` | Set membership | `{"in": {"stage": ["discovery", "negotiation"]}}` |
| `like` | Substring (exact) | `{"like": {"externalId": "CUS-"}}` |
| `fuzzy` | Fuzzy text match | `{"fuzzy": {"name": "john smith"}}` |
| `keyword`, `contains`, `any` | Provider-specific | see connector docs |

**Combining filters (AND):** put multiple operator keys in the same filter object.

```jsonc
{"filter": {"gte": {"started": "2026-01-01T00:00:00Z"}, "eq": {"status": "completed"}}}
```

**Composing with logical operators:**

```jsonc
{"filter": {"and": [cond1, cond2]}}
{"filter": {"or":  [cond1, cond2]}}
{"filter": {"not": cond}}
```

## ID resolution (filtering by related entity)

When filtering by a related entity (a person, team, project, account…), foreign keys are **not always named `id`**. Look for fields whose name or description indicates a link to another entity: `ownerId`, `accountId`, `assignee_id`, `project_key`, etc. Workflow:

1. Run `connectors describe` once for the connector to see entity schemas.
2. Identify the foreign-key field that links the entities you care about.
3. Search the related entity by name to get its primary key.
4. Use that key in the filter.

Example — find deals owned by a user named "Teo":

```bash
# 1. Find Teo's id in the users entity
airbyte connectors execute --json '{
  "name": "hubspot",
  "entity": "users",
  "action": "context_store_search",
  "select_fields": ["id", "firstName"],
  "params": {"query": {"filter": {"fuzzy": {"firstName": "Teo"}}}}
}'

# 2. Use that id as the foreign key on deals
airbyte connectors execute --json '{
  "name": "hubspot",
  "entity": "deals",
  "action": "context_store_search",
  "select_fields": ["id", "name", "amount", "ownerId"],
  "params": {"query": {"filter": {"eq": {"ownerId": "<teo-id>"}}}}
}'
```

## Pagination

- **Default `limit`: 20–25.** Don't paginate unless the user explicitly asks for "all".
- For *"how many"*-style questions with `has_more=true`, answer **"at least N"** rather than counting through every page.
- **Hard stop at 3 pages.** If you'd need more, narrow the filter instead.
- Pagination is cursor-based: read `cursor` from the response (or `meta.next_cursor`, varies per connector) and pass it back as `params.cursor` on the next call while `has_more` remains true.

## Date ranges including today

Search indices can lag the source by hours. When a date range **includes today**, issue **both** a `context_store_search` and a `list` with date params — in the same agent turn — then merge results and deduplicate by `id`. If the date range ends *before* today, `context_store_search` alone is sufficient.

Always resolve relative date phrases ("today", "yesterday", "this week") to **explicit absolute timestamps** (ISO 8601, UTC) and tell the user which range you used.

## Field selection (mandatory)

Two complementary mechanisms — use **both** when you know the fields you need:

- **`select_fields` / `exclude_fields` (API-side, inside the JSON payload)** — passed to the source connector to reduce upstream work and bandwidth. Dot-notation for nested fields supported.
- **`--fields` (CLI-side, global flag)** — shapes the JSON the CLI prints to stdout, after the API responds.

```bash
airbyte connectors execute --fields data.id,data.email,meta.has_more --json '{
  "workspace": "default",
  "name": "hubspot",
  "entity": "contacts",
  "action": "context_store_search",
  "select_fields": ["id", "email", "firstName"],
  "params": {"limit": 20, "query": {"filter": {"eq": {"lifecyclestage": "customer"}}}}
}'
```

`--fields` (CLI) auto-broadcasts row-level paths through the `data` wrapper, so `--fields id,email` is equivalent to `--fields data.id,data.email` *unless* you also want top-level fields like `meta`/`next` — then use the explicit dotted form for the row paths.

## Write actions (`create`, `update`)

> [!IMPORTANT]
> **Write failure handling.** If a write call returns an error or indicates the target was unreachable, do NOT retry with a different target identifier (channel, recipient, conversation, repository, record, etc.). Surface the failure to the caller and let them decide. Silently substituting a destination is forbidden — return the failure instead of completing the work against a different target.

## Error recovery

| Error | Likely cause | Fix |
|---|---|---|
| `not_found` (exit 3) on connector | Name not found | Run `connectors list` to see exact names. The CLI matches against connector instance name, template display name, AND template slug, case-insensitively — so any of those works. |
| `validation_error` (exit 4) on entity/action | Guessed entity name | Run `connectors describe` to enumerate entities. Actions are universal (see table above). |
| Ambiguous name (exit 4) | Two connectors share a name | Pass `"id": "<uuid>"` in the JSON payload instead of `"name"`. |
| `auth_error` (exit 2) | Credentials invalid or expired | Re-check `enrollment status`; if needed, re-run `connectors create` to refresh. |
| Empty `data: []` from `context_store_search` | Index lag, or filter too narrow | Retry with `"action": "list"` (live source). If still empty, broaden the filter. |

## Do NOT

- Do NOT call `execute` without `select_fields` or `exclude_fields`. Field selection is mandatory.
- Do NOT use `like` when `fuzzy` would do — `like` fails on word reordering and typos.
- Do NOT guess entity names. Run `connectors describe` for unfamiliar connectors.
- Do NOT pass credentials in the `execute` payload — credentials live on the connector and are set via `connectors create`.
- Do NOT paginate beyond 3 pages — narrow the filter instead.
- Do NOT pass relative dates ("today", "last week") — resolve to absolute ISO 8601 timestamps and report the range to the user.
- Do NOT silently retry write failures against a different target.
- Do NOT use the per-flag form (`--workspace`, `--name`, `--params`) in agent invocations — pass a single `--json` payload instead.
