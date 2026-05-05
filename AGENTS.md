# AGENTS.md

## Project Overview

`airbyte` is a Go CLI for interacting with the Airbyte API. It uses a registry-based architecture where resources and operations are defined as Go structs and dynamically converted into Cobra commands at startup.

> [!IMPORTANT]
> **Registry Architecture**: Commands are defined as `Resource` + `Operation` structs in `internal/resources/`, NOT as raw Cobra commands in `cmd/`. When adding a new command, implement the `Resource` interface and register it in `register.go`. Do NOT add `cobra.Command` definitions directly.

> [!IMPORTANT]
> **Embedded Skills**: Skill documents live in `internal/resources/skills/*.md` and are compiled into the binary via `//go:embed`. Do not create external skill files or reference paths outside this directory.

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
| `internal/resources/` | All resource implementations + embedded skill docs |
| `internal/client/` | HTTP client with retry logic, structured error types |
| `internal/auth/` | Credential resolution (env -> file), OAuth token caching |
| `internal/config/` | Environment variable configuration loader |
| `internal/output/` | JSON and table output formatters |

### Registry (`internal/registry/`)

| File | Purpose |
| --- | --- |
| `types.go` | `Resource` interface, `Operation` struct, `OperationSchema`, `ParamSchema`, `OperationHooks` |
| `registry.go` | Thread-safe global registry: `Register()`, `All()`, `Get()`, `Reset()` |
| `builder.go` | Converts registered resources into Cobra commands with `--json`, `--id`, `--describe`, file input (`@filename`), parameter validation, and hook execution |

### Resources (`internal/resources/`)

| File | Purpose |
| --- | --- |
| `register.go` | `RegisterAll()` -- registers all resources in the global registry |
| `enrollment.go` | `enrollment status` -- check account enrollment and provisioning state |
| `organizations.go` | `organizations list` -- list available organizations |
| `workspaces.go` | `workspaces list` -- list/filter workspaces with automatic cursor pagination |
| `connectors.go` | `connectors list\|list-available\|describe\|execute\|delete` -- connector management with name->ID resolution hooks |
| `connectors_create.go` | `connectors create` -- interactive browser-based credential flow (OAuth session + polling) |
| `skills.go` | `skills list\|show` -- embedded skill document access via `//go:embed` |

### Client (`internal/client/`)

| File | Purpose |
| --- | --- |
| `client.go` | HTTP client: `Get()`, `Post()`, `Patch()`, `Delete()` with auth headers, retry logic (3x exponential backoff on 429/502/503/504), 30s timeout |
| `errors.go` | `APIError` struct with `Type`, `Message`, `StatusCode`, `Retryable`, `ExitCode()` mapping |

### Auth (`internal/auth/`)

| File | Purpose |
| --- | --- |
| `credentials.go` | `ResolveCredentials()` -- env vars first, then `~/.airbyte/credentials` file |
| `credentials_file.go` | Read/write credentials file with atomic writes and 0600 permission enforcement |
| `token.go` | `TokenManager` -- OAuth token acquisition and caching with auto-refresh |

## Command Surface

| Resource | Operation | Description | Key Params |
| --- | --- | --- | --- |
| `enrollment` | `status` | Check account enrollment | -- |
| `organizations` | `list` | List organizations | -- |
| `workspaces` | `list` | List/filter workspaces | `name_contains`, `status`, `limit` |
| `connectors` | `list` | List workspace connectors | `workspace` (required) |
| `connectors` | `list-available` | List connector templates | -- |
| `connectors` | `describe` | Get connector details + schema | `name`+`workspace` or `--id` |
| `connectors` | `execute` | Execute a connector action | `name`+`workspace` or `--id`, `entity`, `action`, `params` |
| `connectors` | `create` | Interactive credential flow | `workspace`, `template_name` or `template_id` |
| `connectors` | `delete` | Delete a connector | `name`+`workspace` or `--id` |
| `skills` | `list` | List available skill docs | -- |
| `skills` | `show` | Show skill document content | `name` (required) |

### Global Flags

| Flag | Description | Default |
| --- | --- | --- |
| `--format` | Output format: `json` or `table` | `json` |
| `--describe` | Print operation schema and exit (do not execute) | `false` |
| `--output, -o` | Write output to file instead of stdout | -- |
| `--verbose, -v` | Enable debug logging | `false` |
| `--json` | Inline JSON parameters | -- |
| `--id` | Convenience flag for resource ID | -- |

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
| `AIRBYTE_ORGANIZATION_ID` | Organization ID | (optional) |

All three can also be stored in the credentials file (`~/.airbyte/credentials`).

### Configuration

| Variable | Description | Default |
| --- | --- | --- |
| `AIRBYTE_API_HOST` | API base URL | `https://api.airbyte.ai` |
| `AIRBYTE_WEBAPP_URL` | Web app URL for credential flows | `https://cloud.airbyte.com` |
| `AIRBYTE_CREDENTIAL_TIMEOUT` | Credential flow timeout in seconds | `300` |

Credentials can also be stored in `~/.airbyte/credentials` (JSON format, 0600 permissions):
```json
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret",
  "organization_id": "your-org-id"
}
```

## Adding New Resources

When adding a new resource or operation:

1. Create `internal/resources/<name>.go` implementing the `Resource` interface (`Name()`, `Description()`, `Operations()`)
2. Register it in `internal/resources/register.go` via `registry.Register()`
3. Add tests in `internal/resources/<name>_test.go` using `newTestTokenServer()` and `newTestClient()` helpers
4. If the resource uses name-based lookup, add a `PreRun` hook for server-side ID resolution
5. Update the **Command Surface** table in this file
6. If the resource needs usage guidance, add a skill document in `internal/resources/skills/<name>.md`

### Adding New Skills

Skill documents auto-register via Go's `//go:embed` directive. To add a new skill:

1. Create `internal/resources/skills/<name>.md` with a `# Title` heading followed by a one-line description
2. The skill will automatically appear in `airbyte skills list` and be readable via `airbyte skills show --json '{"name": "<name>"}'`
3. No code changes required -- the embed FS picks up all `.md` files in the directory

## Skills Reference

| Skill | Purpose |
| --- | --- |
| `getting-started` | Auth setup, first commands, schema discovery, error handling |
| `connectors` | Connector management workflows, credential flow, error recovery |
| `workspaces` | Workspace discovery and filtering |
| `discovery` | First-time enrollment, provisioning, organization listing |

Run `airbyte skills list` to see all available skills.
Run `airbyte skills show --json '{"name": "<skill>"}'` for detailed guidance.
