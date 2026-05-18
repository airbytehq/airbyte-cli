---
description: Research the codebase to understand how something works
---

# Research Codebase

<!-- ultrathink -->

You are tasked with conducting comprehensive research across the `airbyte` CLI codebase to answer the user's question by spawning parallel sub-agents and synthesizing their findings.

## Research Question

<research-question>
$ARGUMENTS
</research-question>

## Critical Constraint

YOUR ONLY JOB IS TO DOCUMENT AND EXPLAIN THE CODEBASE AS IT EXISTS TODAY:
- DO NOT suggest improvements or changes
- DO NOT perform root cause analysis unless explicitly asked
- DO NOT propose future enhancements
- DO NOT critique the implementation
- DO NOT recommend refactoring
- ONLY describe what exists, where it exists, how it works, and how components interact

## Research Process

### Step 0: Check Existing Research Index

Before spawning subagents, do a quick reuse check:

1. List existing research files: `ls -t thoughts/shared/research/*.md 2>/dev/null | head -5`
2. Match the question against the filename/title of each entry
3. If 1-2 entries are clearly relevant, read those docs first and treat them as prior context

When prior docs are used:
- Include their key findings in subagent prompts so agents can verify and extend instead of repeating work
- Treat older docs as potentially stale and explicitly re-validate file paths/behavior during this run

### Step 0.5: Branch Freshness Check (Non-Mutating)

Before spawning research agents, ensure you're aware of upstream changes:

```bash
git fetch origin main 2>/dev/null
BRANCH_HEAD=$(git rev-parse HEAD)
ORIGIN_MAIN=$(git rev-parse origin/main 2>/dev/null || echo "unknown")
BEHIND_COUNT=$(git rev-list --count HEAD..origin/main 2>/dev/null || echo "0")
echo "Branch HEAD: $BRANCH_HEAD"
echo "origin/main: $ORIGIN_MAIN"
echo "Commits behind main: $BEHIND_COUNT"
```

If `BEHIND_COUNT` is greater than 0, warn: "This branch is **$BEHIND_COUNT commits behind main**. Research findings may not reflect the latest code on main. Consider rebasing before acting on these results."

This is a warning only — do not rebase or modify the branch. Record both SHAs under a `## Provenance` section of the research doc.

### Step 1: Analyze the Question and Detect Domain

Break down the research question into:
- Specific components to investigate
- Patterns or concepts to find
- Directories likely to contain relevant code

**Detect the domain** of the research question to shape the deep-dive agent:

| Signal | Domain |
|--------|--------|
| References `Resource`, `Operation`, `Register`, command surface, adding commands | **Resources** (`internal/resources/`) |
| References `registry.Build`, Cobra flag generation, `--describe`, `--json`, `OperationHooks`, `ParamSchema` | **Registry** (`internal/registry/`) |
| References HTTP, retries, `APIError`, status codes, `Get/Post/Patch/Delete` | **Client** (`internal/client/`) |
| References credentials, OAuth, token caching, settings file, `~/.airbyte-cli/` | **Auth** (`internal/auth/`) |
| References `SKILL.md`, `skills/<command>/`, frontmatter | **Skills** (`skills/`) |
| References `--format`, JSON/table output, `--fields`, `writeResult` | **Output** (`internal/output/`) |
| References telemetry, Segment, `CLI Command Executed`, `is_internal_user` | **Telemetry** (`internal/telemetry/`) |
| References OpenAPI specs, `api/*.json`, `extracted_gen.go`, `--describe` payloads | **Schema** (`internal/spec/`, `cmd/extract-schemas/`) |
| References Cobra root, persistent flags, `version`, `login`, entrypoint | **Cmd** (`cmd/`) |
| Spans multiple areas or unclear | **Cross-cutting** (default) |

### Step 2: Spawn Parallel Research Tasks

First, create a unique temp directory for this research session:

```bash
RESEARCH_SESSION_ID=$(date +%Y%m%d-%H%M%S)
RESEARCH_TMP_DIR="thoughts/shared/research/.tmp/session-${RESEARCH_SESSION_ID}"
mkdir -p "$RESEARCH_TMP_DIR"
```

**Always spawn these three core agents in parallel:**

**Agent 1 - Locator (WHERE code lives):**
```
Agent(subagent_type="Explore",
  description="Locate files for [topic]",
  prompt="Find all files related to [topic] in the airbyte agents CLI.
Look in cmd/, internal/registry/, internal/resources/, internal/client/, internal/auth/,
internal/config/, internal/output/, internal/spec/, internal/telemetry/, skills/, api/.
Return a categorized list: implementation files, test files (*_test.go), generated files
(*_gen.go), skill docs (skills/<cmd>/SKILL.md), OpenAPI specs (api/*.json).
Be thorough — check both the resource Go file and its sibling test file.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/locator-findings.md
Use markdown format with file paths and brief descriptions.")
```

**Agent 2 - Analyzer (HOW code works):**
```
Agent(subagent_type="general-purpose",
  description="Analyze how [topic] works",
  prompt="Analyze how [topic] works in the airbyte agents CLI.
Read the relevant Go files and trace the code path from main.go through the
registry builder into the operation's Run function (and any PreRun hooks).
Document with file:line references.
Do NOT suggest improvements — only document what exists.

Read AGENTS.md first for the project architecture overview, then dive into the relevant
packages.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/analyzer-findings.md
Use markdown format with file:line references.")
```

**Agent 3 - Pattern Finder (similar implementations):**
```
Agent(subagent_type="general-purpose",
  description="Find patterns matching [topic]",
  prompt="Find existing examples of [pattern type] in the airbyte agents CLI.
Look at how other resources in internal/resources/ implement the same Resource/Operation
interface — workspaces.go, organizations.go, and connectors.go are good reference points.
Look at how their tests in *_test.go use newTestTokenServer() and newTestClient() helpers.
Return code snippets with file:line references.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/patterns-findings.md
Use markdown format with code snippets and file:line references.")
```

**Then spawn a domain-specific 4th agent based on the detected domain:**

#### If Resources or Registry domain:
```
Agent(subagent_type="general-purpose",
  description="Deep-dive registry architecture",
  prompt="Deep-dive into the resource/registry architecture for [topic].
Focus on:
- Resource interface in internal/registry/types.go (Name(), Description(), Operations())
- Operation struct: Params, Hooks (PreRun for ID resolution), Run function signature
- registry.Builder: how params become Cobra flags, --json vs per-flag mode, --describe behavior
- Param validation order: required check, type check, then hook execution, then Run
- How resources register in internal/resources/register.go
- PreRun resolution pattern (resolveConnectorID, resolveWorkspaceID) — server-side ID lookup
  to prevent path injection

Read AGENTS.md and the relevant files. Document with file:line references.
Do NOT suggest improvements — only document what exists.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/domain-findings.md
Use markdown format with file:line references.")
```

#### If Client or Auth domain:
```
Agent(subagent_type="general-purpose",
  description="Deep-dive HTTP and auth",
  prompt="Deep-dive into the HTTP client and auth layer for [topic].
Focus on:
- internal/client/client.go: Get/Post/Patch/Delete signatures, retry logic
  (3x exponential backoff on 429/502/503/504), 30s timeout, X-ADP-Agent-CLI header
- internal/client/errors.go: APIError struct, ExitCode() mapping (auth=2, not-found=3, validation=4)
- internal/auth/credentials.go: ResolveSettings — env vars first (all three required),
  then ~/.airbyte-cli/settings.json
- internal/auth/credentials_file.go: atomic writes, 0600 permission enforcement
- internal/auth/token.go: TokenManager OAuth caching and auto-refresh

Document with file:line references. Do NOT suggest improvements.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/domain-findings.md
Use markdown format with file:line references.")
```

#### If Skills or Schema domain:
```
Agent(subagent_type="general-purpose",
  description="Deep-dive skills/schema docs",
  prompt="Deep-dive into the [skills | OpenAPI schema] surface for [topic].
For skills: look at skills/<command>/SKILL.md files, their YAML frontmatter
(name/description/command), and the relationship between a skill and its underlying
command in internal/resources/.
For schema: look at how api/*.json is consumed by cmd/extract-schemas/ to generate
internal/spec/extracted_gen.go, and how registry's --describe surfaces the schema at runtime.

Document with file:line references. Do NOT suggest improvements.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/domain-findings.md
Use markdown format with file:line references.")
```

#### If Cross-cutting or other domain:
```
Agent(subagent_type="general-purpose",
  description="Cross-cutting analysis of [topic]",
  prompt="Analyze how [topic] interacts across the airbyte agents CLI's packages.
Trace the flow from main.go (config -> auth -> client -> registry -> Cobra) through
the relevant packages. Identify which packages collaborate and via what interfaces.

Read AGENTS.md for architecture context. Document with file:line references.
Do NOT suggest improvements — only document what exists.

IMPORTANT: Write your complete findings to: [RESEARCH_TMP_DIR]/domain-findings.md
Use markdown format with file:line references.")
```

### Step 3: Synthesize Findings (Incremental)

After ALL sub-tasks complete, synthesize incrementally to avoid context overload:

1. **Read each findings file one at a time:**
   - `[RESEARCH_TMP_DIR]/locator-findings.md` → Extract top 10 most relevant files
   - `[RESEARCH_TMP_DIR]/analyzer-findings.md` → Extract key architectural insights (5-7 bullets)
   - `[RESEARCH_TMP_DIR]/patterns-findings.md` → Extract reusable patterns (3-5 examples)
   - `[RESEARCH_TMP_DIR]/domain-findings.md` → Extract domain-specific insights (5-7 bullets)

2. **Connect findings across components** — note relationships between files/patterns. Every synthesized claim must be grounded in concrete evidence from the findings files (file path and line reference where available).

3. **If synthesis feels overwhelming**, output partial findings and note what needs more review.

### Step 4: Generate Research Document

```bash
mkdir -p thoughts/shared/research
```

Write `thoughts/shared/research/YYYY-MM-DD-description.md` using this format:

```markdown
# Research: [Topic]

**Date**: [Current date]
**Question**: [Original question]
**Domain**: [Resources | Registry | Client | Auth | Skills | Output | Telemetry | Schema | Cmd | Cross-cutting]

## Provenance

- **Branch HEAD**: `<sha>`
- **origin/main**: `<sha>`
- **Commits behind main**: N

## Summary

[High-level answer to the research question — 2-3 paragraphs]

## Detailed Findings

### [Component/Area 1]
- Description of what exists
- Location: `path/to/file.go:123`
- How it connects to other components

### [Component/Area 2]
...

## Code References

| File | Line | Description |
|------|------|-------------|
| `internal/resources/connectors.go` | 42 | resolveConnectorID PreRun hook |
| ... | ... | ... |

## Architecture Notes

[Patterns, conventions, and design decisions found — e.g., registry-based command building, PreRun ID resolution, file-input `@filename` syntax]

## Open Questions

[Any areas that need further investigation]
```

### Step 4.5: Cleanup Temp Files (Safe Deletion)

After the research document is successfully written, clean up the temp files:

```bash
if [ -d "$RESEARCH_TMP_DIR" ] && [[ "$RESEARCH_TMP_DIR" == thoughts/shared/research/.tmp/session-* ]]; then
    rm -f "$RESEARCH_TMP_DIR/locator-findings.md"
    rm -f "$RESEARCH_TMP_DIR/analyzer-findings.md"
    rm -f "$RESEARCH_TMP_DIR/patterns-findings.md"
    rm -f "$RESEARCH_TMP_DIR/domain-findings.md"
    rmdir "$RESEARCH_TMP_DIR"
    echo "Temp files cleaned up."
else
    echo "WARNING: Skipping cleanup - unexpected temp directory path: $RESEARCH_TMP_DIR"
fi
```

**Safety guarantees**: only deletes files with names we created, uses `rmdir` (fails if not empty), no wildcards, path validation.

### Step 5: Present Summary

After creating the document, present a concise summary to the user:
- Key findings (3-5 bullet points)
- Most important file references
- Document location for full details
- Ask if they have follow-up questions

## Important Notes

- Always spawn parallel tasks to maximize efficiency
- Focus on finding concrete file paths and line numbers
- Research documents should be self-contained
- Keep the main agent focused on synthesis, not deep file reading
- Document cross-component connections (e.g., a Resource's PreRun → client.Get → output formatter)
- You are a documentarian, not an evaluator
