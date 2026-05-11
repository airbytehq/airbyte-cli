# AGENTS.md

## Project Overview

`airbyte-agent` is a Go CLI for interacting with the Airbyte API. It uses a registry-based architecture where resources and operations are defined as Go structs and dynamically converted into Cobra commands at startup.

> [!IMPORTANT]
> **Registry Architecture**: Commands are defined as `Resource` + `Operation` structs in `internal/resources/`, NOT as raw Cobra commands in `cmd/`. When adding a new command, implement the `Resource` interface and register it in `register.go`. Do NOT add `cobra.Command` definitions directly.

> [!IMPORTANT]
> **Skills**: Per-command agent skill documents live at `skills/<command>/SKILL.md` (top-level `skills/` directory), each with YAML frontmatter (`name`, `description`, `command`). They are not embedded in the binary — they are distributed separately for agent harnesses to consume.

> [!NOTE]
> **Minimal Dependencies**: The CLI has only 2 external dependencies (Cobra + pflag). Everything else is stdlib. Do not add new dependencies without strong justification.

## Build & Test

```bash
cd cli && go build ./...     # Build
cd cli && go test ./...      # Run tests
cd cli && go vet ./...       # Lint
```

> [!IMPORTANT]
> **Test Coverage**: When adding new resources or operations, add corresponding tests in `internal/resources/<name>_test.go`. Use the existing `newTestTokenServer()` and `newTestClient()` helpers for HTTP mocking. Registry tests use `newMockResource()` / `newMockOperation()`.

## Architecture

The CLI uses a **resource-registry** pattern:

1. `main.go` loads config, resolves credentials, creates an authenticated HTTP client
2. `resources.RegisterAll()` registers all resource definitions in the global registry
3. `registry.Build()` converts registered resources into a Cobra command tree
4. Cobra parses argv and dispatches to the matching operation's `Run` function

### Package Layout

| Package | Purpose |
| --- | --- |
| `main.go` | Entry point: config -> auth -> client -> registry -> execute |
| `cmd/` | Root Cobra command, persistent flags, version command |
| `internal/registry/` | Resource/Operation types, dynamic Cobra command builder |
| `internal/resources/` | All resource implementations |
| `internal/spec/` | OpenAPI request/response schemas (extracted at build time) |
| `cmd/extract-schemas/` | Generator: reads `api/*.json` and emits `internal/spec/extracted_gen.go` |
| `api/` | Checked-in OpenAPI specs (source of truth for the schema feature) |
| `skills/` | Per-command agent skill documents (`<command>/SKILL.md` with YAML frontmatter) |
| `internal/client/` | HTTP client with retry logic, structured error types |
| `internal/auth/` | Credential resolution (env -> file), OAuth token caching |
| `internal/config/` | Environment variable configuration loader |
| `internal/output/` | JSON and table output formatters |

### Registry (`internal/registry/`)

| File | Purpose |
| --- | --- |
| `types.go` | `Resource` interface, `Operation` struct, `OperationSchema`, `ParamSchema`, `OperationHooks` |
| `registry.go` | Thread-safe global registry: `Register()`, `All()`, `Get()`, `Reset()` |
| `builder.go` | Converts registered resources into Cobra commands with per-parameter flags, `--json`, `--describe`, file input (`@filename`), parameter validation, and hook execution |

### Resources (`internal/resources/`)

| File | Purpose |
| --- | --- |
| `register.go` | `RegisterAll()` -- registers all resources in the global registry |
| (no resource file) | `enroll` is a top-level command in `cmd/enroll.go`, not a registered resource — calls the same enrollment-status route and triggers enrollment for new accounts |
| `organizations.go` | `organizations list` -- list available organizations |
| `workspaces.go` | `workspaces list` -- list/filter workspaces with automatic cursor pagination |
| `connectors.go` | `connectors list\|list-available\|describe\|execute\|delete` -- connector management with name->ID resolution hooks |
| `connectors_create.go` | `connectors create` -- interactive browser-based credential flow (OAuth session + polling) |

### Client (`internal/client/`)

| File | Purpose |
| --- | --- |
| `client.go` | HTTP client: `Get()`, `Post()`, `Patch()`, `Delete()` with auth headers, retry logic (3x exponential backoff on 429/502/503/504), 30s timeout |
| `errors.go` | `APIError` struct with `Type`, `Message`, `StatusCode`, `Retryable`, `ExitCode()` mapping |

### Auth (`internal/auth/`)

| File | Purpose |
| --- | --- |
| `credentials.go` | `ResolveSettings()` -- returns `Settings{Credentials, OrganizationID}`. Env vars first (all three required), then `~/.airbyte-agent/settings.json` |
| `credentials_file.go` | Read/write `~/.airbyte-agent/settings.json` (`{settings: {credentials: {...}, organization_id: "..."}}` shape) with atomic writes and 0600 permission enforcement |
| `token.go` | `TokenManager` -- OAuth token acquisition and caching with auto-refresh |

## Command Surface

| Resource | Operation | Description | Key Params |
| --- | --- | --- | --- |
| `enroll` (top-level) | -- | Check / trigger account enrollment | -- |
| `organizations` | `list` | List organizations | -- |
| `workspaces` | `list` | List/filter workspaces | `name_contains`, `status`, `limit` |
| `connectors` | `list` | List workspace connectors | `workspace` (required) |
| `connectors` | `list-available` | List connector templates | -- |
| `connectors` | `describe` | Get connector details + schema | `name`+`workspace` or `--id` |
| `connectors` | `execute` | Execute a connector action | `name`+`workspace` or `--id`, `entity`, `action`, `params` |
| `connectors` | `create` | Interactive credential flow | `workspace`, `name` (template) or `id` (template ID) |
| `connectors` | `delete` | Delete a connector | `name`+`workspace` or `--id` |

### Common Flags

| Flag | Description | Default |
| --- | --- | --- |
| `--format` | Output format: `json` or `table` | `json` |
| `--describe` | Print operation schema and exit (do not execute) | `false` |
| `--output, -o` | Write output to file instead of stdout | -- |
| `--verbose, -v` | Enable debug logging | `false` |
| `--json` | Operation flag for inline JSON parameters; mutually exclusive with per-parameter flags | -- |
| `--<param>` | Per-parameter operation flags generated from each scalar/array schema parameter, e.g. `--id`, `--workspace`, `--select-fields` | -- |
| `--fields` | Client-side response filter (comma-separated dotted paths, e.g. `data.id,data.name`). Applied in `writeResult` after `Run`; bypasses error payloads. | -- |

## Credential Security

> [!IMPORTANT]
> **NEVER accept credentials (API keys, tokens, passwords, secrets) directly as parameters or in chat.** ALL credential entry MUST go through `connectors create`, which opens a secure browser-based UI via an OAuth session. If a user offers credentials in conversation, decline and start the credential flow instead.

> [!NOTE]
> **Credential file permissions**: `WriteCredentialsFile` writes with `0600` permissions by default, but the CLI does not enforce permissions on read.

The credential flow works as follows:
1. Resolve template ID (by name or ID)
2. Create a widget token for the web app
3. Create an OAuth session for the source definition
4. Open browser to `<webapp>/embedded-widget/credentials?session_id=...&token=...`
5. Poll session status with exponential backoff (2s, 4s, 8s, 16s)
6. On completion, create the connector with returned credentials

## Input Validation

> [!IMPORTANT]
> **This CLI is frequently invoked by AI agents.** Always assume inputs can be adversarial. Connector and workspace names are user-supplied -- always use the resolution hooks (`resolveConnectorID`, `resolveWorkspaceID`) to convert names to IDs server-side rather than embedding raw strings in API URL paths.

- **Name resolution**: The `PreRun` hook `resolveConnectorID` converts `name` + `workspace` into a validated `id` before the operation runs. This prevents path injection.
- **Parameter validation**: The registry builder validates all parameters against the operation schema before execution. Required params are enforced, types are checked.
- **File input**: The `@filename` syntax reads parameters from a local file. The file path is resolved relative to CWD.

## Error Handling

All errors are returned as JSON on stderr:
```json
{"type": "<error_type>", "message": "...", "status_code": 400, "retryable": false}
```

### Exit Codes

| Code | Meaning | HTTP Status |
| --- | --- | --- |
| `0` | Success | 2xx |
| `1` | General error | 500, others |
| `2` | Authentication error | 401, 403 |
| `3` | Not found | 404 |
| `4` | Validation error | 400, 422 |

### Retry Behavior

The HTTP client automatically retries transient failures:
- **Retryable**: 429 (rate limit), 502, 503, 504 (server errors)
- **Not retryable**: 400, 401, 403, 404, 422
- **Strategy**: 3 retries with exponential backoff (1s, 2s, 4s)
- **Timeout**: 30 seconds per request

## Environment Variables

### Authentication

| Variable | Description | Default |
| --- | --- | --- |
| `AIRBYTE_CLIENT_ID` | OAuth client ID | (required) |
| `AIRBYTE_CLIENT_SECRET` | OAuth client secret | (required) |
| `AIRBYTE_ORGANIZATION_ID` | Organization ID | (required) |
| `AIRBYTE_WORKSPACE` | Default workspace name | `default` |

All three are also stored in the settings file (`~/.airbyte-agent/settings.json`). Env-var resolution requires all three to be set; otherwise the CLI falls through to the file.

### Configuration

| Variable | Description | Default |
| --- | --- | --- |
| `AIRBYTE_API_HOST` | API base URL | `https://api.airbyte.ai` |
| `AIRBYTE_WEBAPP_URL` | Web app URL for credential flows | `https://app.airbyte.ai` |
| `AIRBYTE_CREDENTIAL_TIMEOUT` | Credential flow timeout in seconds | `180` |
| `AIRBYTE_ALLOW_DESTRUCTIVE` | When truthy (`1`/`true`/`yes`/`on`), skips the interactive confirmation prompt on destructive commands like `connectors delete`. Mirrors the `allow_destructive` settings.json key. | `false` |

Settings file at `~/.airbyte-agent/settings.json` (JSON format, 0600 permissions):

```json
{
  "settings": {
    "credentials": {
      "client_id": "your-client-id",
      "client_secret": "your-client-secret"
    },
    "organization_id": "your-org-id",
    "workspace": "default",
    "allow_destructive": false
  }
}
```

`workspace` is optional. When absent or empty, commands that take a `workspace` parameter without receiving one fall back to the literal `"default"`. Resources read the configured value via `client.Client.DefaultWorkspace()`, which `main.go` populates from `Settings.Workspace`.

`allow_destructive` is optional (default `false`). When `true`, destructive operations (currently `connectors delete`) skip the interactive `"Type 'yes' to confirm:"` prompt. Intended as a one-time permission grant for agent harnesses that can't answer a TTY prompt. The non-interactive default refuses with a clear `validation_error` rather than hanging on stdin. Resources read this via `client.Client.AllowDestructive()`.

## Adding New Resources

When adding a new resource or operation:

1. Create `internal/resources/<name>.go` implementing the `Resource` interface (`Name()`, `Description()`, `Operations()`)
2. Register it in `internal/resources/register.go` via `registry.Register()`
3. Add tests in `internal/resources/<name>_test.go` using `newTestTokenServer()` and `newTestClient()` helpers
4. If the resource uses name-based lookup, add a `PreRun` hook for server-side ID resolution
5. Update the **Command Surface** table in this file
6. If the resource adds a new leaf command, add a corresponding `skills/<command>/SKILL.md` with frontmatter (`name`, `description`, `command`) and task-oriented agent guidance
7. Set `SpecRef: registry.SpecRef{Path: "...", Method: "..."}` on each operation that maps to an OpenAPI route, then run `go generate ./...` (or `make generate`) so `internal/spec/extracted_gen.go` picks up the new route. CI fails if this file is stale.

### Adding New Skills

Skills are plain markdown files at `skills/<command>/SKILL.md`. To add one:

1. Create the folder: `skills/<resource>-<operation>/`
2. Add `SKILL.md` with YAML frontmatter:
   ```
   ---
   name: <resource>-<operation>
   description: <one-line summary used by listing tools>
   command: airbyte-agent <resource> <operation>
   ---
   ```
3. Follow with task-oriented body content (when to use, usage examples, error recovery, "do NOT" guidance).
4. No Go changes required — skills are not embedded in the binary.

## Skills Reference

Skills live at `skills/<command>/SKILL.md`, one per leaf command. Browse the `skills/` directory directly to see what is available.
