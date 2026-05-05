---
name: connectors-list-available
description: List connector templates that can be installed via `connectors create`.
command: airbyte connectors list-available
---

# connectors list-available

List the connector templates available to install in this account. Each template has a `name` (e.g. `salesforce`, `hubspot`) that you pass to `connectors create --name <name>`.

## Usage

```
airbyte connectors list-available --format table
```

## When to use

Always run this **before** `connectors create` to discover the exact template `name` to use. Template names are stable identifiers — do not guess them.

## Workflow

```
airbyte connectors list-available --format table
airbyte connectors create --json '{"workspace": "my-workspace", "name": "salesforce"}'
```

## Hints

- The list is filtered to what your account has access to — it will not show every connector that exists in Airbyte's catalog.
