---
name: commit
description: 'Stage and commit current workspace changes with a Conventional Commits message generated from the diff. Use when the user asks to commit, save changes, or create a commit. Inspects git status/diff, picks the right type (feat/fix/refactor/docs/chore/test/style), writes an English message, runs git add -A and git commit. Does not push.'
argument-hint: 'Optional: extra context or commit type override (e.g. fix, chore)'
---

# Commit

Commit the current workspace changes following Conventional Commits format. Execute end-to-end without asking for confirmation unless something is ambiguous.

## When to Use
- User says "commit", "save", "提交", or similar.
- After finishing a logical unit of work that should be persisted to git history.

## Do NOT Use For
- Pushing to remote (use `/release` or let the user push).
- Amending or rewriting existing commits (unless explicitly requested).

## Procedure

1. **Inspect changes**
   - Run `git status` to list modified, staged, and untracked files.
   - Run `git diff` (and `git diff --staged` if anything is already staged) to understand the actual code changes.
   - If there are no changes, stop and tell the user there is nothing to commit.

2. **Decide commit type**
   - `feat:` — new user-facing feature or command.
   - `fix:` — bug fix.
   - `refactor:` — code restructuring without behavior change.
   - `docs:` — documentation only (README, comments, copilot-instructions, prompts, skills, etc.).
   - `chore:` — build, tooling, config, dependencies, release plumbing.
   - `test:` — adding or updating tests.
   - `style:` — formatting only.
   - If the user provided an override in the invocation, honor it.

3. **Write the commit message**
   - Subject line: `<type>: <concise summary>` in English, imperative mood, ≤ 72 chars, no trailing period.
   - If the change spans multiple meaningful aspects, add a body: blank line, then `-` bullet points describing what changed and why.
   - Do **not** add `Co-authored-by`, AI attribution, or sign-off lines.

4. **Stage and commit**
   - Run `git add -A`.
   - Commit using a heredoc to preserve newlines (replace the `<...>` placeholders with the real text before running):
     ```bash
     git commit -m "$(cat <<'EOF'
     <type>: <subject>

     - <bullet 1>
     - <bullet 2>
     EOF
     )"
     ```

5. **Report**
   - Show the resulting commit hash and subject (`git log -1 --oneline`).
   - Do **not** push.

## Notes
- All commit content must be in English (per repo language policy).
- Never switch branches or modify `git config` on the user's behalf.
- Never use `--no-verify`. If a hook fails, surface the error and stop.
