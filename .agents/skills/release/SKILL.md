---
name: release
description: 'Cut a new tdu-cli release: commit pending changes, bump semver tag (patch/minor/major or explicit vX.Y.Z), build-check, tag, and push to trigger GoReleaser CI. Use when the user says release, ship, publish a version, 发版, 发布版本.'
argument-hint: 'Optional: patch | minor | major | explicit vX.Y.Z (if omitted, prompts for choice)'
---

# Release

Cut a new release of `tdu-cli`. Drive the full flow end-to-end and only stop for confirmation before irreversible remote actions.

## When to Use
- User says "release", "ship a version", "发版", "发布新版本", or similar.
- After merging features/fixes that should be published to users via Homebrew.

## Do NOT Use For
- Just committing changes without releasing (use `/commit`).
- Hotfixing a previously published tag (force-push / tag move are disallowed here).

## Procedure

1. **Check workspace status**
   - Run `git status`.
   - If there are uncommitted changes, run the [commit skill](../commit/SKILL.md) procedure first: inspect diff → choose Conventional Commit type → `git add -A` → `git commit` with an English message.
   - If the working tree is clean, continue.

2. **Sanity check**
   - Ensure the current branch is `main` (`git rev-parse --abbrev-ref HEAD`). If not, warn the user and ask before continuing.
   - Run `git fetch --tags origin` and check the local branch is not behind remote (`git status -sb`). If behind, ask the user to pull first.
   - Run `go build ./...` to make sure the project compiles. If it fails, stop and surface the error.

3. **Determine the new version**
   - Run `git tag --sort=-v:refname | head -1` to read the latest tag (e.g. `v0.2.1`). Version format is `vMAJOR.MINOR.PATCH`.
   - Compute the three candidates from the latest tag:
     - `patch`: `v0.2.1` → `v0.2.2`
     - `minor`: `v0.2.1` → `v0.3.0` (resets PATCH)
     - `major`: `v0.2.1` → `v1.0.0` (resets MINOR and PATCH)
   - Pick the new version using this priority:
     1. If the user passed an explicit `vX.Y.Z` in the skill invocation, use it.
     2. If the user passed `patch`/`minor`/`major`, use the matching candidate.
     3. **Otherwise, ask the user to choose** by presenting the three candidates with their resulting versions, e.g.:
        > Current: `v0.2.1`. Which bump?
        > - patch → `v0.2.2`
        > - minor → `v0.3.0`
        > - major → `v1.0.0`
        Use the `vscode_askQuestions` tool when available; otherwise ask in plain chat and wait for the answer. Do **not** assume a default — wait for the user.
   - Verify the chosen tag does not already exist locally or remotely (`git rev-parse -q --verify "refs/tags/<new-version>"` and `git ls-remote --tags origin "<new-version>"`). If it does, stop and report.
   - Echo the final chosen version back to the user before tagging.

4. **Tag**
   - Run `git tag <new-version>` (annotated tag is not required; GoReleaser keys off the tag name).

5. **Push**
   - Run `git push && git push --tags`.
   - This triggers GoReleaser CI (which publishes the GitHub Release and updates the Homebrew cask in `TD-Yofun/homebrew-tap`).

6. **Report**
   - Print the new version, the commit hash it points to, and a reminder that CI is now publishing the release.
   - Surface the URL `https://github.com/TD-Yofun/tdu-cli/actions` so the user can watch the release pipeline.

## Notes
- All commit messages and tag names must be in English.
- Never force-push, never delete or move existing tags without explicit user request.
- Never switch branches or modify `git config` on the user's behalf.
- Never use `--no-verify` to skip hooks.
- Do not edit `CHANGELOG.md` manually — GoReleaser generates release notes from commits.
