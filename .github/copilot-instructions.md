# Project: tdu-cli

## Language Policy

All generated artifacts (code, comments, commit messages, documentation, etc.) **MUST default to English** unless the user explicitly specifies another language.

## Overview

**tdu** (talkdesk utils) is a CLI tool written in Go that provides a collection of utilities for daily Talkdesk work. The primary feature is an **interactive upgrade** system for related tools.

- Repository: `github.com/TD-Yofun/tdu-cli`
- Language: Go 1.26+
- Framework: Cobra (CLI) + promptui (interactive selection)
- Build / Release: GoReleaser + Homebrew tap (`TD-Yofun/homebrew-tap`)
- License: MIT

## Project Structure

```
main.go                  # Entry point, calls cmd.Execute()
cmd/
  root.go                # Root command definition (tdu), registers subcommands, version injected via ldflags
  upgrade/
    upgrade.go           # upgrade subcommand entry, promptui interactive selection of upgrade targets
    exec.go              # Thin wrapper around exec.Command for testability
    tdu.go               # tdu self-upgrade logic (brew update && brew upgrade)
    tdcli.go             # td-cli upgrade logic (GitHub API → download → install)
```

## CLI Command Tree

```
tdu
├── --version            # Show version (injected at build time)
└── upgrade              # Interactively select and upgrade a tool
    ├── tdu              # Self-upgrade via Homebrew
    └── td-cli           # Download and install from GitHub Release
```

## Key Modules

### cmd/root.go
- Defines the root Cobra command `tdu`
- `version` variable is set at build time via `-ldflags "-X ...cmd.version=..."`
- Registers `upgrade.Cmd` as a subcommand in `init()`

### cmd/upgrade/upgrade.go
- Defines the `upgrade` subcommand
- Uses promptui for interactive selection (with emoji template)
- Routes user selection to `upgradeTdu()` or `upgradeTdCli()`

### cmd/upgrade/tdu.go
- Self-upgrade via `brew update` + `brew upgrade TD-Yofun/tap/tdu`

### cmd/upgrade/tdcli.go (most complex module, ~350 lines)
- 6-step process: get local version → query latest version via GitHub API → compare versions → download (with progress bar & cache) → extract and install (sudo) → verify
- Requires `HOMEBREW_GITHUB_API_TOKEN` env var for private repo access
- Auto-detects architecture (arm64/amd64), downloads the matching darwin binary
- Cache directory: `~/.cache/tdu/td-cli/{version}/`
- Install path: `/usr/local/bin/td`

### cmd/upgrade/exec.go
- Wraps `exec.Command` to allow easy mocking in tests

## Build & Release

```bash
# Local build
go build -o tdu .

# Release via GoReleaser (triggered automatically by CI)
git tag v0.x.x && git push origin v0.x.x
```

GoReleaser configuration:
- Target platforms: darwin/linux × amd64/arm64
- CGO disabled, fully static binaries
- Homebrew formula auto-published to `TD-Yofun/homebrew-tap`

## Release Workflow

When the user says "release a version" (or similar), execute the following steps automatically:

1. **Check workspace status**: Run `git status` to see if there are uncommitted changes.
2. **Commit changes** (if any): Run `git add -A && git commit`. Auto-generate the commit message based on the changes, following Conventional Commits format (`feat: xxx` or `fix: xxx`).
3. **Determine the new version number**:
   - Run `git tag --sort=-v:refname | head -1` to get the latest tag.
   - Version format: `vMAJOR.MINOR.PATCH` (e.g. `v0.2.1`).
   - Default: increment PATCH (e.g. `v0.2.1` → `v0.2.2`).
   - If the user specifies a version, use that.
   - If the user says "minor", increment MINOR (e.g. `v0.2.1` → `v0.3.0`).
   - If the user says "major", increment MAJOR (e.g. `v0.2.1` → `v1.0.0`).
4. **Create tag**: Run `git tag <new-version>`.
5. **Push to remote**: Run `git push && git push --tags`.
6. **Confirm completion**: Display the final version number and push result.

## Coding Conventions

- When adding a new upgradeable tool, create a dedicated file under `cmd/upgrade/` and register it in the selection list in `upgrade.go`.
- Wrap external command calls with `newCommand()` (exec.go) to keep them testable.
- Use emoji-style output for user interaction (`printSection`, `printStep`, `printDetail`).
- Error handling should be detailed, including debug info (missing token, insufficient permissions, missing resources, etc.).
- Provide progress bars and caching for large file downloads.
