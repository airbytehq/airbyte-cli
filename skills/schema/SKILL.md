---
name: schema
description: Print the merged CLI + OpenAPI schema (request, response, parameters) for any operation.
command: airbyte schema
---

# schema

Return the full machine-readable schema for an operation: the CLI-level parameter shape **and** the underlying OpenAPI route's parameters, request body, and response. Equivalent to `<resource> <operation> --describe`, but discoverable as a top-level command.

> [!IMPORTANT]
> Run `airbyte schema <resource> <operation>` **before** writing code or scripts that consume an operation's output. The `api.response` schema tells you exactly what fields will come back so you can pass `--fields` correctly the first time.

## Usage

```
airbyte schema <resource> <operation>

# Examples
airbyte schema workspaces list
airbyte schema connectors execute
airbyte schema organizations list
```

## Output shape

```jsonc
{
  "description": "...",        // CLI-level operation description
  "params": { ... },           // CLI flag/JSON parameters (what you pass)
  "api": {                     // OpenAPI route info (omitted if no mapping)
    "path": "/api/v1/...",
    "method": "GET",
    "summary": "...",
    "description": "...",
    "parameters": [ ... ],     // query/path/header parameters
    "request_body": { ... },   // present on POST/PATCH/PUT routes
    "response": { ... }        // 200/2xx response schema, $refs inlined
  }
}
```

The two surfaces are intentionally separate:

- **`params`** — what you, as a CLI caller, pass inside the `--json` payload. Includes CLI conveniences (workspace fallback, name/id alternation, etc.).
- **`api`** — what bytes go on the wire to the Airbyte API. Use this to know what fields the response will contain and pick `--fields` accordingly.

## When to use

- **Before building any automation** that depends on an operation's response shape — read `api.response` so you can shape your filtering/parsing precisely.
- **When `--fields` returns something unexpected** — `api.response` shows the exact field names and structure.
- **When discovering the API surface** as an agent — `airbyte schema <r> <op>` is the canonical way to learn what an operation does without making a request.

## Equivalents

```bash
airbyte schema connectors execute
# is identical to
airbyte connectors execute --describe
```

Use whichever fits your flow. `airbyte schema` is the discoverable top-level form; `--describe` is the per-operation flag.

## Hints

- `--describe` and `airbyte schema` never make API calls — safe to run without auth, against unfamiliar accounts, etc.
- Errors from `airbyte schema` (unknown resource or operation) are JSON on stderr with exit code 3.
- Operations that don't map to an OpenAPI route (e.g. `auth login`, which is purely local) omit the `api` block.
