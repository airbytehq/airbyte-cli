---
description: Execute an implementation plan phase-by-phase with verification
model: opus
---

# Implement Plan

<!-- ultrathink -->

You are tasked with implementing an approved plan from `thoughts/shared/plans/` (or the project root). Execute the plan phase-by-phase with verification checkpoints in the `airbyte-agent` CLI codebase.

## Plan Reference

<plan-reference>
$ARGUMENTS
</plan-reference>

## Initial Setup

### Step 1: Locate and Read the Plan (Lazy Loading)

If a specific plan file was provided, use it. Otherwise check for `PLAN.md` at project root, then list plans in `thoughts/shared/plans/`.

**Fresh-context start:** If this session has no prior conversation history (i.e., the user started a new session specifically for implementation), the plan file is the sole source of truth. Its Overview, Research References, and Current State sections provide all necessary context. Do not ask the user to re-explain the feature — read the plan.

#### Step 1a: Clean worktree gate

```bash
if [ -n "$(git status --porcelain)" ]; then
  echo "DIRTY"
else
  echo "CLEAN"
fi
```

If the worktree is dirty, warn: "There are uncommitted changes in the working tree. Commit or stash them before proceeding — the rebase step requires a clean worktree." This is a warning, not a hard block — the user may be resuming a previous `/implement` run. If the user confirms they want to proceed, skip the rebase step (Step 1c) but still run the drift check (Step 1b).

#### Step 1b: Fetch and deterministic drift check

```bash
git fetch origin main 2>/dev/null
ORIGIN_MAIN=$(git rev-parse origin/main 2>/dev/null || echo "unknown")

# Extract Go/JSON/Markdown file paths referenced in the plan
referenced_files=$(grep -oE '[a-zA-Z0-9_./-]+\.(go|json|md|yaml|yml)' <plan-path> | sort -u | head -20)

# Find the merge-base between current branch and origin/main
MERGE_BASE=$(git merge-base HEAD origin/main 2>/dev/null || echo "")

# Check what changed on main since the branch diverged, scoped to plan-referenced files
if [ -n "$MERGE_BASE" ] && [ -n "$referenced_files" ]; then
  echo "--- Files changed on main since branch diverged ---"
  git diff --name-status -M "$MERGE_BASE"..origin/main -- $referenced_files 2>/dev/null
fi
```

Interpret the `--name-status` output:
- **`D` (deleted) or `R` (renamed)**: **Block** — warn: "Files referenced in the plan have been deleted or renamed on main: [list]. The plan likely needs revision." Stop and ask if they want to proceed anyway.
- **`M` (modified)**: **Warn** — "Files referenced in the plan have been modified on main since this branch diverged: [list]. Verify before proceeding." Warning only.
- **No changes**: Proceed normally.

#### Step 1c: Rebase onto main

If the worktree is clean:

```bash
BEHIND_COUNT=$(git rev-list --count HEAD..origin/main 2>/dev/null || echo "0")
if [ "$BEHIND_COUNT" -gt 0 ]; then
  git rebase origin/main
fi
```

If the rebase fails with conflicts, stop and report: "Rebase onto origin/main failed with merge conflicts. Resolve them manually and re-run /implement." List conflicting files from `git diff --name-only --diff-filter=U`.

If the worktree was dirty and the user chose to proceed, skip this step.

**Read the plan using lazy phase loading** — do NOT read the entire plan upfront:

1. **Read the Overview section** fully — title, summary, constraints (MUST/SHOULD/DO NOT rules), recommended implementation mode, complexity
2. **Scan all phase headers** — find `## Phase` or `### Phase` headers and their checkbox status (`- [x]` vs `- [ ]`). Record each phase name, line range, and completion status
3. **Build a cross-phase file list** — lightweight scan of ALL phases for Go file paths (look for `*.go`, `*.json`, `*.md` paths in `### Changes` tables). Collect just the paths, not implementation details. Pass this list to subagents so they make forward-compatible choices
4. **Identify the first incomplete phase** — the first phase with unchecked items
5. **Read ONLY that phase's detailed section** using line offsets

### Step 2: Review Context (Current Phase Only)

Read referenced files for the **current phase only**:
- Files that will be modified in the current phase
- Directly related code needed for context (imports, interfaces the phase extends)

Run an unresolved-decision preflight before any code changes:
- Scan **all phases** of the plan (not just the current one) for unresolved markers: `TBD`, `TODO`, `Open Question`, `decide later`, `?` placeholders
- If unresolved decisions remain, STOP and ask the user to resolve them before proceeding

### Step 3: Plan Validation Gate

Before starting the phase loop, validate the plan structure:

1. **Phase completeness** — every phase must list at least one file in its `### Changes` table
2. **No forward dependencies** — no phase should depend on files that are only created by a later phase. Cross-reference the cross-phase file list from Step 1.3 and flag forward dependencies explicitly.
3. **Verification commands** — every phase should include a `#### Verification` section with runnable commands

If validation fails, show the issues as warnings and let the user decide whether to proceed.

Use actionable validation messages, for example: `Phase 3 references internal/resources/datasets.go, but that file is only created in Phase 4 — forward dependency detected.`

### Step 4: Verify Understanding

Before implementing, confirm with the user:
- Plan title
- Current phase status for all phases
- Which phase you'll begin with
- What changes it involves
- Which files will be modified

Ask "Proceed?"

---

## Phase Execution

Each phase is executed by a **subagent** to keep context fresh. The main agent acts as coordinator.

### For Each Phase

1. **Record a phase baseline snapshot** so verification stays scoped to this phase:

```bash
phase_baseline=$(git stash create "phase-baseline" || true)
phase_baseline=${phase_baseline:-HEAD}
pre_phase_untracked=$(git ls-files --others --exclude-standard)
```

Use this baseline plus the untracked-file snapshot for every later `git diff`, `git diff --stat`, and `git diff --name-only` calculation in this phase. The `HEAD` fallback keeps Phase 1 working when the worktree was clean before the phase began.

2. **Spawn a subagent** for the phase (copy-paste content, do NOT just reference the plan file):

```
Agent(subagent_type="general-purpose",
  description="Implement Phase [N]: [Name]",
  prompt="Implement Phase [N]: [Name] of the airbyte-agent CLI plan.

## Plan Section
[Paste the phase's plan content here]

## Files to Modify This Phase
[List of files for this phase from the Changes table]

## Cross-Phase File List (make forward-compatible choices)
[All files modified across all phases]

## Previous Phase Handoff (context from Phase N-1)
[If Phase 1: This is the first phase — no previous handoff.]
[If Phase N>1: Paste the JSON handoff artifact from Phase N-1.
 Pay attention to:
 - `public_interfaces_changed`: do not break these interfaces
 - `invariants_added`: do not violate these rules
 - `required_followups`: address these items if they apply to your phase]

## Constraints
[MUST/SHOULD/DO NOT rules from plan overview]

## Project Conventions
- Read AGENTS.md first if you have not already — it documents the registry architecture,
  Resource/Operation interfaces, and adding-new-resources checklist.
- New resources: implement Resource (Name(), Description(), Operations()) and register
  in internal/resources/register.go. Add *_test.go using newTestTokenServer() / newTestClient().
- Name-based lookups: use a PreRun hook (resolveConnectorID-style) to resolve to a
  validated id server-side. Never embed raw user-supplied names in URL paths.
- New leaf command: add skills/<resource>-<operation>/SKILL.md with YAML frontmatter
  (name, description, command).
- If an operation maps to an OpenAPI route, set SpecRef{Path, Method} and run
  go generate ./... so internal/spec/extracted_gen.go is current.
- Do NOT accept credentials directly — route credential entry through connectors create.
- Do NOT add new third-party dependencies without strong justification.

## Verification (run after implementing)
[Phase-level verification commands — see below]

Report back: what changed, what checks passed, any issues.")
```

3. **Phase-level verification** (run by the subagent — NOT the full test suite):

```bash
gofmt -l . | tee /dev/stderr | wc -l   # expect 0
go vet ./<changed-package>/...
go build ./...
go test ./internal/<relevant-package>/... -run <TestName> -v
```

**Identifying related tests:** Match modified source files to test files. `connectors.go` → `connectors_test.go` in the same directory. If new tests were added, run the specific test name with `-run`. If no specific tests found, run the whole package directory. Do NOT run `go test ./...` at phase level; reserve the full suite for the Final Verification.

4. **Phase verification gate** — after the subagent completes, run a structured verification gate:

**Deterministic checks (always run):**
- Extract the current phase's `### Changes` table. Parse the `| File |` column values (second column). If no change table found, skip file delta verification and note "No change table found for this phase — skipping file delta verification."
- Normalize each expected path by trimming whitespace, stripping backticks, and removing any leading `./`.
- Run `git diff --name-only <phase_baseline>` to capture tracked file paths that changed since the phase baseline, then add any newly created untracked files by diffing `git ls-files --others --exclude-standard` against `pre_phase_untracked`. Normalize those paths the same way.
- Compare expected and actual file lists to identify (a) matched files, (b) expected files NOT modified, (c) unexpected files modified.

Include the file delta results in the phase summary:

```
Phase N modified X/Y expected files.
[If unexpected]: Z unexpected file(s) changed: `path/to/file1`, `path/to/file2`
[If missing]: W expected file(s) NOT modified: `path/to/file3`
```

If anything looks off, surface it to the user before proceeding.

5. **Receive subagent results** — the main agent reviews what changed and what passed.

6. **Update the plan file** — check off completed items: `- [x]`, note deviations inline.

7. **Generate phase handoff** — after updating the plan, build a concise JSON object summarizing the completed phase. Keep it in memory as context for the next phase (do not write a file):

```json
{
  "phase": N,
  "phase_name": "Name from the phase header",
  "changed_files": ["repo-relative paths from the phase-scoped tracked diff plus newly created untracked files"],
  "public_interfaces_changed": ["exported Go symbols, Resource interface adds, new flags, new command surface"],
  "invariants_added": ["rules the next phase must not violate, e.g., 'PreRun returns id, do not bypass'"],
  "decisions_and_why": [
    { "decision": "chose to extend connectors over new resource", "rationale": "verb fits existing surface" }
  ],
  "open_risks": ["potential issues for later phases"],
  "verification_evidence": {
    "gofmt": "clean | N files",
    "vet": "pass | fail",
    "build": "pass | fail",
    "tests": "N passed, M failed",
    "file_delta": {
      "matched_count": 0,
      "expected_count": 0,
      "missing_files": [],
      "unexpected_files": []
    }
  },
  "required_followups": ["things the next phase MUST address"]
}
```

Keep it concise: 1-5 items per array field. Focus `public_interfaces_changed` on cross-file contracts. Include `decisions_and_why` only for non-obvious choices.

8. **Report and gate** — summarize what changed, which checks passed (gofmt/vet/build/relevant tests), the file delta results, the handoff summary, and any manual verification items from the plan. Ask the user to test and confirm before proceeding.

9. **Phase transition** — wait for confirmation, then read the next phase's section and loop back to step 1.

---

## Handling Issues

**If plan doesn't match reality:** Report the mismatch with what the plan expected, what actually exists in code, and evidence (file:line references and/or command output). Offer options:
1. Adapt implementation to reality
2. Stop and revise the plan
3. Ask for guidance

**If automated checks fail:**
1. Analyze the failure
2. Fix if it's a simple issue from your changes
3. Report if it's unclear or pre-existing
4. Do NOT proceed until checks pass

**If stuck:**
1. Re-read relevant code in `internal/` and `cmd/`
2. Re-read `AGENTS.md` for architecture conventions
3. Consider if the codebase evolved since the plan was written
4. Ask for clarification with specific questions

## Completion

After all phases complete, run the **Final Verification** (this is the only time the full test suite runs):

```bash
gofmt -l .                                  # expect empty
go vet ./...
go build ./...
go test ./...
```

If any phase added or changed a `SpecRef`, also run:

```bash
go generate ./...                           # or: make generate
git status -- internal/spec/extracted_gen.go   # expect: tracked, no diff after regen
```

Then report:
- List all completed phases with checkmarks
- Confirm Final Verification passed (gofmt clean, vet clean, build green, full test suite green)
- Confirm `internal/spec/extracted_gen.go` is up to date if `SpecRef` changed
- List any remaining manual verification items from the plan
- Offer to create a commit, run additional probe commands (e.g., `./airbyte-agent <new-cmd> --describe`), or other next steps

## Important Guidelines

### Honor the Plan
- The plan was carefully designed and approved
- Follow its intent even if small details need adjustment
- Don't add scope or "improvements" not in the plan

### Reality Can Be Messy
- Code may have changed since planning
- Some assumptions may be wrong
- Communicate mismatches clearly before proceeding

### Verification is Critical
- Phase-level: `gofmt` + `vet` + `build` + relevant tests (fast feedback)
- Final: full `go test ./...` and (if applicable) `go generate ./...` (comprehensive, runs once after all phases)
- Never proceed past a manual checkpoint without user confirmation

### Maintain Momentum
- Complete one phase fully before starting the next
- Keep the end goal in mind
- Ask for help rather than getting stuck

### Project-Specific Reminders
- **Registry, not raw Cobra** — new commands must be a `Resource`+`Operation` registered in `internal/resources/register.go`, NOT a `cobra.Command` added to `cmd/`.
- **Skills are not embedded** — `skills/<command>/SKILL.md` is a plain markdown file; no Go changes are required to add or edit one.
- **Schema generation is gate-checked** — CI fails when `internal/spec/extracted_gen.go` is stale after a `SpecRef` change. Don't skip the regen step.
- **Credentials route through `connectors create`** — never accept secrets as CLI flags or chat input.
- **Adversarial inputs assumed** — agents drive this CLI. Use `PreRun` hooks for name → id resolution to keep raw user strings out of URL paths.
