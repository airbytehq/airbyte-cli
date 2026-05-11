# Releases

How to cut a new release of `airbyte-agent`, what happens behind the scenes, and how to clean up if something goes wrong.

## TL;DR

```
./scripts/release.sh minor       # v0.1.0 → v0.2.0
./scripts/release.sh fix         # v0.2.0 → v0.2.1
./scripts/release.sh major       # v0.2.1 → v1.0.0
```

Then watch the workflow finish, eyeball the draft release + the formula commit in `airbytehq/homebrew-tap`, and click **Publish release** on the draft.

## The release script

`scripts/release.sh` is the single entry point for cutting a release. It:

1. Reads the latest `v*` git tag in this repo and treats it as the **current version** (defaults to `v0.0.0` if no tags exist yet).
2. Bumps it according to the argument:
   - `major` → `vX.0.0` (resets minor + patch)
   - `minor` → `v0.X.0` (resets patch)
   - `fix`   → `v0.0.X` (patch only)
3. Refuses to proceed unless:
   - You're on `main`
   - The working tree is clean
   - Local `main` matches `origin/main`
   - The target tag doesn't already exist
4. Prompts you to confirm `current → next`.
5. Runs `git tag -a` and `git push origin <tag>`.

The script does **not** publish anything itself — pushing the tag is what triggers the rest of the pipeline.

## What happens after the tag is pushed

`.github/workflows/release.yml` fires on any `v*` tag push. It:

1. Mints a cross-repo token via the Octavia bot GitHub App (scoped to `airbyte-agent-cli` + `homebrew-tap`).
2. Runs `goreleaser release --clean`, which:
   - Builds cross-platform binaries (linux/darwin/windows × amd64/arm64).
   - Creates a **draft** GitHub release in `airbyte-agent-cli` with the archives and `checksums.txt` attached.
   - Commits an updated `Formula/airbyte-agent.rb` directly to `main` on `airbytehq/homebrew-tap`.

Prerelease tags (e.g. `v0.1.0-rc1`) are detected automatically: the GitHub release is still created (as a prerelease draft) but the brew formula commit is **skipped** (`skip_upload: auto` in `.goreleaser.yaml`).

## Publishing the draft

The release stays as a draft until a human publishes it. This is intentional — it gives you a chance to verify the artifacts before users can pull them.

Two ways to publish:

**GitHub UI:** open the draft from `https://github.com/airbytehq/airbyte-agent-cli/releases`, click the pencil/edit icon, scroll to the bottom of the editor, click **Publish release**.

**CLI:**

```
gh release edit vX.Y.Z --draft=false --repo airbytehq/airbyte-agent-cli
```

Once published, the tarball URLs in the brew formula resolve and `brew install airbytehq/tap/airbyte-agent` works. Before publishing, those URLs return 404 to anonymous requests (which is what `brew` makes).

## Removing a tag

Tags are immutable by convention — once shipped, treat them as permanent. But if a release was never published (still a draft) and you need to redo it (wrong commit, broken build, etc.), here's the full cleanup.

**1. Delete the draft GitHub release** at `https://github.com/airbytehq/airbyte-agent-cli/releases`. From CLI:

```
gh release delete vX.Y.Z --repo airbytehq/airbyte-agent-cli --yes
```

**2. Delete the tag locally and on origin:**

```
git tag -d vX.Y.Z
git push origin :refs/tags/vX.Y.Z
```

**3. Revert the formula commit on `homebrew-tap`** if goreleaser already pushed one. Find the bump commit in `https://github.com/airbytehq/homebrew-tap/commits/main` and revert it (or push a follow-up commit restoring the previous formula). If this is the first release, just delete `Formula/airbyte-agent.rb` from the tap.

**4. Re-run the release script** when ready:

```
./scripts/release.sh minor    # or fix/major
```

Since the tag is gone, the script picks the previous tag (or `v0.0.0`) as "current" and bumps from there.

### Caveats

- If users have already run `brew install` against the published version, the binary is on their machine. You can't unship it remotely — the best you can do is publish a fixed version and tell them to `brew upgrade`.
- Moving a tag (force-pushing) is technically possible but breaks anyone who already fetched it. Always prefer cutting a new version (`fix` bump) over rewriting an existing tag.

## Troubleshooting

**`git tag vX.Y.Z was not made against commit <sha>`** — the workflow was triggered via "Run workflow" (workflow_dispatch) on `main` instead of the tag-push event. The runner checks out `main`'s tip, but goreleaser expects HEAD to match the tag. Let the tag-push trigger fire the workflow; don't trigger manually from the Actions tab.

**`Could not create file: Changes must be made through a pull request`** — `homebrew-tap`'s `main` is branch-protected. Either add the Octavia bot App as a bypass on the protection rule, or switch `.goreleaser.yaml`'s `brews[].repository` block to use `pull_request: enabled: true`.

**`brew install` returns 404** — the source repo is private. GitHub returns 404 (not 403) for anonymous requests to private-repo release assets. Make the repo public, or set up a public mirror repo for releases.

**`[@octokit/auth-app] appId option is required`** — `OCTAVIA_BOT_APP_ID` / `OCTAVIA_BOT_PRIVATE_KEY` secrets aren't set on this repo. Add them at the repo or org level.

**`Not Found` on `GET /repos/.../installation`** — the Octavia bot App isn't installed on the target repo. Org → GitHub Apps → Octavia bot → Configure → add the repo to its repository access list.
