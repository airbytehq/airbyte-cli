# airbyte-cli

A Go CLI for the Airbyte API, designed to be driven by both humans and AI agents.

The CLI exposes Airbyte's resources (organizations, workspaces, connectors, etc.) as a uniform `airbyte <resource> <operation>` interface. Every command supports JSON input/output, schema introspection via `--describe`, and structured JSON errors with stable exit codes — making it safe to script and easy for agents to discover at runtime.

## What it is

- **Resource-driven**: commands aren't hand-written Cobra trees. Resources are declared as Go structs in `internal/resources/`, registered into a global registry, and dynamically materialized into Cobra commands at startup.
- **Self-describing**: any command will print its full parameter schema with `--describe`, so callers (especially LLMs) can discover required fields without guessing.
- **Minimal**: only two external dependencies (Cobra + pflag). Everything else is stdlib.
- **Distributable skills**: per-command agent skill documents live in `skills/<command>/SKILL.md` for downstream tooling (e.g. Claude Code) to consume directly.

See `AGENTS.md` for the full architecture reference and `CONTEXT.md` for the agent-facing usage guide.

## How it works

```
main.go
  ├── config.Load()              # read env vars (AIRBYTE_API_HOST, etc.)
  ├── auth.ResolveCredentials()  # env first, then ~/.airbyte/credentials
  ├── auth.NewTokenManager()     # OAuth token caching + auto-refresh
  ├── client.New()               # HTTP client w/ retry + structured errors
  ├── resources.RegisterAll()    # register every Resource into the registry
  ├── registry.Build()           # convert registry → Cobra command tree
  └── cmd.Execute()              # dispatch
```

| Package | Purpose |
| --- | --- |
| `cmd/` | Root Cobra command, persistent flags, version |
| `internal/registry/` | `Resource` interface, dynamic command builder |
| `internal/resources/` | Resource implementations |
| `skills/` | Per-command agent skill documents (`<command>/SKILL.md`) |
| `internal/client/` | HTTP client (3x exponential-backoff retry on 429/5xx, 30s timeout) |
| `internal/auth/` | Credential resolution, OAuth token caching |
| `internal/config/` | Environment variable loader |
| `internal/output/` | JSON and table formatters |

The HTTP client retries 429/502/503/504 with backoff and surfaces non-retryable errors (400/401/403/404/422) as `APIError` values that map to deterministic exit codes.

## Install

```bash
git clone https://github.com/airbytehq/airbyte-cli.git
cd airbyte-cli
make build         # builds ./airbyte
# or
make install       # installs to $GOBIN
```

Build directly without the Makefile:

```bash
go build -o airbyte .
```

## Configure

Credentials can be supplied via environment variables or a credentials file at `~/.airbyte/credentials` (JSON, `0600` permissions).

### Resolution order

1. **Environment variables** — used if both `AIRBYTE_CLIENT_ID` and `AIRBYTE_CLIENT_SECRET` are set. If either is missing, the CLI falls through to the file.
2. **Credentials file** at `~/.airbyte/credentials`.
3. If neither is configured, the CLI exits with an authentication error.

Env vars take precedence over the file when both are present, so they're useful for one-off overrides (e.g. `AIRBYTE_CLIENT_ID=... airbyte ...`).

### Environment variables

| Variable | Description | Default |
| --- | --- | --- |
| `AIRBYTE_CLIENT_ID` | OAuth client ID | (required) |
| `AIRBYTE_CLIENT_SECRET` | OAuth client secret | (required) |
| `AIRBYTE_ORGANIZATION_ID` | Organization ID | (optional) |
| `AIRBYTE_API_HOST` | API base URL | `https://api.airbyte.ai` |
| `AIRBYTE_WEBAPP_URL` | Web app URL for credential flows | `https://cloud.airbyte.com` |
| `AIRBYTE_CREDENTIAL_TIMEOUT` | Credential flow timeout (seconds) | `300` |

### Credentials file

```json
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret",
  "organization_id": "your-org-id"
}
```

## Usage

```bash
airbyte <resource> <operation> [flags]
```

Parameters can be supplied two ways: as a single JSON document via `--json`, or as individual flags (`--workspace foo --name bar`). The two modes are **mutually exclusive** — passing both is an error. Output is JSON by default; `--format table` produces a human-readable table.

### Two ways to pass parameters

**1. Individual flags (recommended for humans)** — scalar and array parameters in the operation's schema are exposed as `--<param>` flags, with snake_case keys converted to kebab-case (e.g. `select_fields` → `--select-fields`):

```bash
airbyte connectors describe --workspace default --name hubspot
airbyte connectors execute --workspace default --name hubspot \
  --entity contacts --action read \
  --select-fields id,email,name
```

Run `airbyte <resource> <operation> --help` to see the available flags for any command.

**2. JSON (recommended for agents and complex payloads)** — pass the whole parameter set as a JSON object:

```bash
airbyte connectors execute --json '{
  "workspace": "default",
  "name": "hubspot",
  "entity": "contacts",
  "action": "read",
  "select_fields": ["id", "email", "name"]
}'
```

Use `@filename` to load JSON from a file: `--json @params.json`. `--json` is the only way to pass nested objects (e.g. the `params` field on `connectors execute`).

### Common flags

| Flag | Description | Default |
| --- | --- | --- |
| `--json` | Operation flag for inline JSON parameters (or `@filename` to load from a file). Cannot be combined with per-parameter flags. | -- |
| `--format` | Output format: `json` or `table` | `json` |
| `--describe` | Print the operation's parameter schema and exit | `false` |
| `--output, -o` | Write output to a file instead of stdout | -- |
| `--verbose, -v` | Enable debug logging | `false` |
| `--fields` | Filter the response to only the listed fields. Comma-separated, dotted paths (e.g. `data.id,data.name`). Applied client-side, after the API responds. Errors are not filtered. | -- |

### Filtering output with `--fields`

`--fields` shapes the response payload after it returns from the API. Paths use dotted notation; when a path crosses an array, the remaining segments are applied to every element ("array broadcast"):

```bash
# Both of these work — list responses are wrapped in {"data": [...]} and the
# CLI auto-broadcasts when no path matches a top-level key.
airbyte organizations list --fields id,organization_name
airbyte organizations list --fields data.id,data.organization_name

# Mixed paths require explicit prefixes — the auto-broadcast only fires
# when *no* path matches a top-level key:
airbyte connectors list --fields data.id,data.name,next
```

**Path resolution rules:**

1. **Strict match first.** Paths are matched against top-level keys of the response.
2. **Smart wrapper fallback.** When *no* paths match top-level keys AND the response has *exactly one* top-level array (e.g. `{"data": [...]}`), each path is implicitly prefixed with that wrapper's key and re-applied. Lets you write `--fields id,name` instead of `--fields data.id,data.name` for list-style responses.
3. **Mixed cases stay strict.** If even one path matches top-level, no rewrite happens — pass explicit dotted paths if you also want row-level fields.
4. **Missing paths are dropped silently.** Errors are never filtered.

This is **client-side**: the full payload still travels from the API to the CLI. To reduce upstream work, `connectors execute` separately accepts `select_fields` / `exclude_fields` which are sent to the source connector. The two are complementary — combine them when you want both bandwidth savings and a clean output shape.

### Discovering commands

```bash
airbyte --help                              # list resources
airbyte connectors --help                   # list operations
airbyte connectors execute --describe       # show parameter schema
```

### Command surface

| Resource | Operation | Description |
| --- | --- | --- |
| `enrollment` | `status` | Check account enrollment & provisioning |
| `organizations` | `list` | List organizations |
| `workspaces` | `list` | List/filter workspaces |
| `connectors` | `list` | List connectors in a workspace |
| `connectors` | `list-available` | List connector templates |
| `connectors` | `describe` | Show a connector's entities and actions |
| `connectors` | `execute` | Run an action on a connector |
| `connectors` | `create` | Interactive browser-based credential flow |
| `connectors` | `delete` | Delete a connector |

### Examples

```bash
# Verify enrollment
airbyte enrollment status

# Find a workspace
airbyte workspaces list --format table

# Discover what a connector can do
airbyte connectors describe --json '{"workspace": "default", "name": "hubspot"}'

# Read data, limiting fields to keep the response small
airbyte connectors execute --json '{
  "workspace": "default",
  "name": "hubspot",
  "entity": "contacts",
  "action": "read",
  "select_fields": ["id", "email", "name"]
}'

# Create a new connector (opens a browser for secure credential entry)
airbyte connectors create --json '{
  "workspace": "default",
  "name": "hubspot"
}'

# Load a complex payload from a file
airbyte connectors execute --json @params.json
```

## Credentials are entered in the browser, not the CLI

The CLI never accepts API keys, tokens, or passwords as command-line parameters. `connectors create` opens a browser-based widget, runs an OAuth session, polls until the user submits, then creates the connector with the returned credentials. If you're an agent, do not ask the user to paste secrets — start the credential flow.

## Errors and exit codes

All errors are emitted as JSON on stderr.

```json
{"type": "not_found", "message": "...", "status_code": 404, "retryable": false}
```

| Exit code | Meaning | HTTP |
| --- | --- | --- |
| `0` | Success | 2xx |
| `1` | General error | 500, others |
| `2` | Authentication error | 401, 403 |
| `3` | Not found | 404 |
| `4` | Validation error | 400, 422 |

When you see a validation error, re-run the command with `--describe` to inspect the expected schema.

## Skills

Per-command agent skill documents live under `skills/<command>/SKILL.md`, each with YAML frontmatter (`name`, `description`, `command`) and task-oriented guidance. They are designed to be consumed by agent harnesses (e.g. Claude Code) — copy or symlink the directory into the harness's skill location, or distribute via your own tooling.

## Develop

```bash
go build ./...
go test ./...
go vet ./...
```

To add a new resource: implement the `Resource` interface in `internal/resources/<name>.go`, register it in `register.go`, and add tests using the existing `newTestTokenServer()` / `newTestClient()` helpers. See `AGENTS.md` for the full guide.
