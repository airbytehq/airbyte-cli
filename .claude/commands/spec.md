---
description: Create a detailed implementation spec with research phase
model: opus
---

# Implementation Spec

<!-- ultrathink -->

You are tasked with creating a detailed implementation specification for the `airbyte-agent` CLI through research and iterative collaboration. Be skeptical, thorough, and work with the user to produce a high-quality spec.

## Feature Request

<feature-request>
$ARGUMENTS
</feature-request>

## Read-Only Research Constraint

During Steps 0-4 (research and design), you and all spawned agents operate in **READ-ONLY mode**:
- **DO NOT** create, modify, or delete any source files
- **DO NOT** run commands that mutate state (`git commit`, file writes, etc.)
- **You MAY** read files, search code, run `git log`/`git diff`/`git status`, and execute read-only commands
- The **ONLY files** you may write during the entire spec process are the plan file in Step 5 and the in-progress checkpoint in `thoughts/shared/plans/.in-progress/`
- Research findings should be communicated via agent responses, NOT written to source files

## Process

### Step -1: Check for In-Progress Spec (Recovery)

```bash
ls -t thoughts/shared/plans/.in-progress/*.md 2>/dev/null | head -3
```

If in-progress files exist, read the most recent one and present it via `AskUserQuestion`:

```
question: "I found an in-progress spec checkpoint. Resume or start fresh?"
header: "Recovery"
options:
  - "Resume from checkpoint" — "[filename] — last updated at [step], covers [summary]"
  - "Start fresh" — "Ignore the checkpoint and begin a new spec"
```

- **"Resume"** → Read the checkpoint to recover feature request, completed research, current step, design decisions. Skip already-completed steps.
- **"Start fresh"** → Delete the in-progress file and continue to Step 0.

### Step 0: Check for Existing Research (MANDATORY)

You MUST complete this step before any research.

1. List recent research files:
```bash
ls -t thoughts/shared/research/*.md 2>/dev/null | head -8
```

2. If NO research files exist → Skip to Step 0.5

3. If research files exist, you MUST use `AskUserQuestion`:

```
question: "I found existing research files. Which would you like to use for this spec?"
header: "Research"
multiSelect: true
options:
  - "[filename1.md]" — "[first line/title from file]"
  - "[filename2.md]" — "[first line/title from file]"
  - "None - run fresh research" — "Spawn new research agents"
```

4. After selection, ask about gaps:

```
question: "Do you need additional research beyond the selected files?"
header: "Gaps"
options:
  - "No, sufficient" — "Proceed with selected research (Recommended)"
  - "Yes, need more" — "I'll ask what specific areas need research"
```

5. Route based on selections:
   - No research files exist → Step 0.5
   - "None - run fresh research" → Step 0.5
   - Files selected + no gaps → Read selected files, skip to Step 0.5
   - Files selected + gaps → Read selected files, then Step 1b

### Step 0.5: Detect Domain and Complexity

**Analyze the feature request** to determine:

1. **Domain detection** — what packages will this touch?

| Signal | Domain |
|--------|--------|
| New `airbyte-agent <noun> <verb>` command, new resource, new operation | **Resources** (`internal/resources/`) |
| Changes to registry building, flag generation, `--describe`, `--json`, validation | **Registry** (`internal/registry/`) |
| HTTP retries, status codes, error mapping, new transport headers | **Client** (`internal/client/`) |
| Credentials, OAuth, settings file, token caching | **Auth** (`internal/auth/`) |
| Adding/editing skill docs, frontmatter changes | **Skills** (`skills/`) |
| Output formatters, `--fields`, table layout, JSON shaping | **Output** (`internal/output/`) |
| Telemetry events, properties, gating | **Telemetry** (`internal/telemetry/`) |
| OpenAPI extraction, generated schemas | **Schema** (`internal/spec/`, `cmd/extract-schemas/`) |
| Touches multiple packages or unclear | **Cross-cutting** |

2. **Complexity assessment**:

| Complexity | Signals |
|------------|---------|
| **Simple** (1-2 files, 1 phase) | Bug fix, single-flag change, single hook update |
| **Medium** (3-6 files, 2-3 phases) | New operation on existing resource, new flag with validation |
| **Complex** (7+ files, 4+ phases) | New resource with create/list/describe/execute/delete + schema + skill + tests |

### Step 1a: Full Research (if no existing research selected)

1. **Read any mentioned files FULLY** — no pagination
2. **Spawn parallel research tasks**:

```
Agent(subagent_type="Explore",
  description="Locate files for [feature area]",
  prompt="CONSTRAINT: READ-ONLY mode — do NOT create, modify, or delete any files.
Find WHERE all files related to [feature area] live in the airbyte-agent CLI.
Look in cmd/, internal/, skills/, api/. Return categorized list with file purposes.")

Agent(subagent_type="general-purpose",
  description="Analyze [related feature]",
  prompt="CONSTRAINT: READ-ONLY mode — do NOT create, modify, or delete any files.
Understand HOW [related feature] currently works in the airbyte-agent CLI.
Read the implementation and document the patterns used. Include file:line references.
Do NOT suggest improvements.")

Agent(subagent_type="general-purpose",
  description="Find patterns for [feature]",
  prompt="CONSTRAINT: READ-ONLY mode — do NOT create, modify, or delete any files.
Find existing patterns in internal/resources/ that match [similar functionality].
Look at workspaces.go, organizations.go, connectors.go for Resource/Operation
interface examples, and their *_test.go files for the test patterns
(newTestTokenServer, newTestClient, newMockResource).
Return code snippets with references.")
```

3. Wait for ALL tasks to complete before proceeding
4. Read all identified files into main context

### Step 1b: Supplemental Research (if existing research has gaps)

1. Read all selected research files into context first
2. Ask user to specify the gap: "What specific area needs additional research?"
3. Spawn targeted research based on gap type:
   - **WHERE questions** → `Explore`
   - **HOW questions** → `general-purpose` analyzer agent
   - **PATTERN questions** → `general-purpose` pattern-finder agent
4. Synthesize new findings with existing research

#### Checkpoint: Save Research Progress

```bash
mkdir -p thoughts/shared/plans/.in-progress
```

Write to `thoughts/shared/plans/.in-progress/YYYY-MM-DD-description.md`:

```markdown
# In-Progress Spec: [Feature Name]
## Feature Request
[Original feature request text]
## Current Step: 2 (Present Understanding)
## Research Summary
- [Key files discovered with paths]
- [Patterns identified]
- [Existing research files used]
## Selected Research Files
- [Files selected in Step 0]
## Domain: [detected domain]
## Complexity: [simple|medium|complex]
```

Keep this under 30 lines.

### Step 2: Present Understanding

After gathering context, present what you learned:

```
Based on [source], I understand we need to [summary].

Key findings:
- [Current implementation detail with file:line]
- [Relevant pattern discovered]
- [Potential complexity identified]

Questions needing clarification:
- [Only genuine unknowns requiring human judgment]
```

### Step 3: Design Options (if applicable)

If multiple valid approaches exist, present a structured comparison and ask user preference. Common decision points for this CLI:

- **New operation vs. extend existing operation** — does this warrant a new verb, or is it a flag/parameter on an existing one?
- **`PreRun` hook vs. inline validation in `Run`** — name resolution and adversarial-input validation belong in `PreRun`; pure execution logic belongs in `Run`.
- **`--json` vs. per-parameter flags** — for nested/structured input prefer `--json`; for ergonomic CLI prefer per-parameter flags. The registry supports both.
- **Schema in code vs. extracted from OpenAPI** — does the new operation need a `SpecRef`? If yes, `api/*.json` must include the route.
- **New skill needed?** — every new leaf command should have a `skills/<command>/SKILL.md`.

### Step 4: Plan Structure

Get buy-in on phases before writing:

```
Proposed phases:
1. [Phase name] — [what it accomplishes]
2. [Phase name] — [what it accomplishes]

Does this phasing make sense?
```

#### Checkpoint: Save Design Progress

Update the in-progress checkpoint:

```markdown
# In-Progress Spec: [Feature Name]
## Feature Request
[Original feature request text]
## Current Step: 5 (Write Plan)
## Research Summary
[Carried forward]
## Design Decisions
- [Approach chosen and why]
- [Key patterns to follow]
## Approved Phase Structure
1. [Phase name] — [what it accomplishes]
2. [Phase name] — [what it accomplishes]
## Domain: [detected domain]
## Complexity: [simple|medium|complex]
```

### Step 4.5: Quality Validation

Before writing the plan, do a self-check pass against this checklist. Spawn at most one `general-purpose` agent if any item is in doubt; otherwise validate inline.

- [ ] **Resource/Operation conformance** — if adding a new resource, it implements `Resource` (`Name()`, `Description()`, `Operations()`) and is registered in `internal/resources/register.go`.
- [ ] **Hook placement** — name-based lookups use a `PreRun` hook (`resolveConnectorID`-style) that returns a validated `id` before the operation's `Run` executes. Never embed raw user-supplied names in URL paths.
- [ ] **Credential safety** — no plan step accepts credentials directly from chat or as a parameter. ALL credential entry routes through the `connectors create` browser flow.
- [ ] **Schema and `SpecRef`** — if the new operation maps to an OpenAPI route, the plan includes setting `SpecRef{Path, Method}` and re-running `go generate ./...` (or `make generate`) so `internal/spec/extracted_gen.go` is current. CI fails on staleness.
- [ ] **Skill doc** — new leaf commands have a corresponding `skills/<command>/SKILL.md` with YAML frontmatter (`name`, `description`, `command`).
- [ ] **Tests** — every new resource/operation has a sibling `*_test.go` using `newTestTokenServer()` and `newTestClient()` helpers; registry tests use `newMockResource()` / `newMockOperation()`.
- [ ] **Dependencies** — no new third-party dependencies are introduced unless strongly justified. The CLI keeps 3 external deps total (Cobra + pflag + analytics-go).
- [ ] **Destructive operations** — if any phase changes a destructive command, the plan honors the `allow_destructive` setting (env + settings.json) and the non-interactive TTY refusal path.
- [ ] **Documentation updates** — `AGENTS.md` Command Surface table and `CONTEXT.md` usage examples are updated if the user-facing surface changes.

Surface any failures to the user and resolve before proceeding.

---

## Step 5: Write Checklist Plan

```bash
mkdir -p thoughts/shared/plans
```

Lift the template below verbatim into `thoughts/shared/plans/YYYY-MM-DD-description.md` and fill in the bracketed slots:

```markdown
---
title: [Plan Title]
date: [YYYY-MM-DD]
status: ready
complexity: [simple | medium | complex]
domain: [resources | registry | client | auth | skills | output | telemetry | schema | cmd | cross-cutting]
---

# [Plan Title]

## Overview

[2-3 sentence summary of what we're building and why.]

**Recommended /implement mode**: standard (the airbyte-agent CLI uses sequential phases — no team mode).

### Constraints

- **MUST** [non-negotiable requirement, e.g., "PreRun hook must resolve names server-side"]
- **MUST** [requirement]
- **SHOULD** [strong preference, e.g., "prefer extending existing resource over creating a new one if the verb fits"]
- **DO NOT** [explicit exclusion, e.g., "DO NOT accept credentials as CLI parameters"]

## Research References

- `thoughts/shared/research/YYYY-MM-DD-topic.md` — [what we used from it]
- `AGENTS.md` §[section] — [what we used from it]

## Current State

[What exists today, with file:line references. This is the starting state the implementation will modify.]

- `internal/resources/<resource>.go:<line>` — [current behavior]
- `internal/registry/builder.go:<line>` — [current behavior]

## Phases

### Phase 1: [Phase Name]

**Goal**: [One sentence describing the outcome of this phase.]

#### Changes

| File | Purpose |
|------|---------|
| `internal/resources/<file>.go` | [what changes — new function, modified operation, etc.] |
| `internal/resources/<file>_test.go` | [new test cases for the changes above] |

#### Tasks

- [ ] [Concrete, verifiable step]
- [ ] [Concrete, verifiable step]
- [ ] Run `go generate ./...` if any `SpecRef` was added or changed
- [ ] Update `AGENTS.md` Command Surface table (if user-facing surface changed)

#### Verification

```bash
gofmt -l . | tee /dev/stderr | wc -l   # expect 0
go vet ./...
go build ./...
go test ./internal/<package>/... -run <TestName> -v
```

#### Manual Verification

- [ ] [User-facing behavior to test by hand, e.g., "run `airbyte-agent connectors describe --json '{...}'` against a real workspace"]

### Phase 2: [Phase Name]

...

## Final Verification

After all phases are complete:

```bash
gofmt -l .                   # expect empty
go vet ./...
go build ./...
go test ./...
```

If `SpecRef` was added or changed in any phase: `go generate ./...` (or `make generate`) and confirm `internal/spec/extracted_gen.go` is committed.
```

### Checklist Format Rules

Walk through every rule below and confirm it is satisfied before declaring the plan ready for `/implement`.

1. **Every task is a checkbox** — `- [ ] ...`. No prose-only tasks.
2. **Every task is binary** — done or not done. No "mostly" states. "Implement function X" → done when the function exists and tests pass; not "make function X better".
3. **Every changed file appears in the `### Changes` table for its phase** — no silent edits.
4. **Each file row has a Purpose column entry** — "what changes", not "the file".
5. **Phases are independently verifiable** — every phase ends with a `#### Verification` block of runnable commands.
6. **Phases are ordered by dependency** — Phase N cannot reference a file that is only created in Phase N+1.
7. **Tests live in the same phase as the code they test** — never push tests to a follow-up phase.
8. **No `TBD`, `TODO`, `?`, or "decide later"** in the final plan. All design questions are resolved in Step 3.
9. **No hand-wavy "etc."** — if there are more files, list them.
10. **MUST/SHOULD/DO NOT constraints are stated once, in Overview** — don't repeat them per phase.
11. **Manual verification items are explicit** — every phase that affects user-facing behavior has at least one `- [ ]` under Manual Verification, or the section is omitted.
12. **`SpecRef` changes always trigger a `go generate ./...` task in the same phase.**
13. **New leaf command? New `skills/<command>/SKILL.md` is in the same phase.**
14. **Resource interface changes are isolated to a single phase** — don't split `Name()`/`Operations()` edits across phases.
15. **Credential paths route through `connectors create`** — no plan step accepts credentials inline.
16. **The final phase contains the Final Verification block** — running the full `go test ./...` belongs at the end, not after every phase.

#### Step 5 Cleanup: Remove In-Progress Checkpoint

```bash
rm -f thoughts/shared/plans/.in-progress/YYYY-MM-DD-description.md
rmdir thoughts/shared/plans/.in-progress 2>/dev/null || true
```

### Step 6: Review, Iterate, and Handoff

Present the plan location and ask for review options via `AskUserQuestion`:

```
question: "I've created the plan at thoughts/shared/plans/YYYY-MM-DD-description.md. How would you like to review it?"
header: "Review"
options:
  - "Looks good" — "Approve and proceed to handoff"
  - "Edit plan directly" — "I'll edit the file myself"
  - "Revise with feedback" — "I'll describe what to change and you update it"
  - "Start over" — "Go back to Step 2"
```

**Route based on selection:**

- **"Looks good"** → Proceed to Step 6.5.
- **"Edit plan directly"** → Tell the user the plan path and wait. After they confirm, re-read the file (your in-context copy is now stale), diff against what you wrote, summarize changes, then loop back to the review prompt.
- **"Revise with feedback"** → Ask what to change, update the file, then loop back to the review prompt.
- **"Start over"** → Go back to Step 2.

#### Step 6.5: Implementation Handoff

After approval:

```
question: "Plan approved. How would you like to proceed?"
header: "Handoff"
options:
  - "Start /implement now (keep context)" — "Continue in this session — good for simple specs"
  - "Clear context first (recommended)" — "Open a new session with /implement — avoids research context pollution"
  - "Done for now" — "I'll implement later"
```

- **"Start /implement now"** → Tell the user to run `/implement thoughts/shared/plans/<plan-file>` in this session.
- **"Clear context first"** → Tell the user:
  ```
  Open a new session and run:
  /implement thoughts/shared/plans/<plan-file>

  The plan file is self-contained — its Overview, Research References, and Current State
  sections carry forward everything /implement needs without the research conversation.
  ```
- **"Done for now"** → Confirm the plan path and exit.

## Guidelines

- **Be skeptical** — question vague requirements, verify with code rather than assuming.
- **Be interactive** — don't write the full plan in one shot; get buy-in at each step; ALWAYS use `AskUserQuestion` for research selection.
- **Be concise** — tables over prose, pattern references over code snippets, bullets over paragraphs, link to research for depth.
- **Be actionable** — every item is a checkbox, every file has a purpose, every change has a pattern reference, binary completion only.
- **Read-only until Step 5** — only the plan file and in-progress checkpoint are writable during the spec phase.
- **No open questions in the final plan** — if unknowns appear, STOP and research or ask immediately.
