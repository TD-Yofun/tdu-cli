# Project: tdu-cli

## Language Policy

All generated artifacts (code, comments, commit messages, documentation, etc.) **MUST default to English** unless the user explicitly specifies another language.

## Overview

**tdu** (talkdesk utils) is a CLI tool written in Go that provides a collection of utilities for daily Talkdesk work. The primary feature is an **interactive upgrade** system for related tools.

- Repository: `github.com/TD-Yofun/tdu-cli`
- Language: Go 1.26+
- Framework: Cobra (CLI) + promptui (interactive selection)
- Build / Release: GoReleaser v2 + Homebrew cask (`TD-Yofun/homebrew-tap`)
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
    forticlient_vpn.go   # FortiClient VPN upgrade logic (GitHub release → download .mpkg → installer)
  fix/
    fix.go               # fix subcommand entry, promptui interactive selection of fix targets
    forticlient_vpn.go   # FortiClient VPN fix logic (daemon loading, process restart, FortiTray)
  report/
    report.go            # report subcommand entry, promptui interactive selection of report targets
    forticlient_vpn.go   # FortiClient VPN report logic (collect logs, system info, create GitHub issue)
  utils/
    utils.go             # Shared utilities (NewCommand, PrintSection, PrintStep, PrintDetail, RunSudoCommand, etc.)
.goreleaser.yml          # GoReleaser v2 config (builds, archives, homebrew cask, changelog)
```

## CLI Command Tree

```
tdu
├── --version            # Show version (injected at build time)
├── upgrade              # Interactively select and upgrade a tool
│   ├── tdu              # Self-upgrade via Homebrew
│   ├── td-cli           # Download and install from GitHub Release (Talkdesk/td-cli)
│   └── forticlient-vpn  # Download and install FortiClient VPN .mpkg from GitHub Release
└── fix                  # Interactively select and fix known issues
    └── forticlient-vpn  # Fix blank screen and SAML login issues
└── report               # Collect diagnostics and report issues to GitHub
    └── forticlient-vpn  # Collect logs/info and create a GitHub issue
```

## Key Modules

### cmd/root.go
- Defines the root Cobra command `tdu`
- `version` variable is set at build time via `-ldflags "-X github.com/TD-Yofun/tdu-cli/cmd.version=..."`
- Registers `upgrade.Cmd`, `fix.Cmd`, and `report.Cmd` as subcommands in `init()`

### cmd/upgrade/upgrade.go
- Defines the `upgrade` subcommand
- Uses promptui for interactive selection (with emoji template)
- Maintains `upgradeItems` slice — the list of all selectable upgrade targets
- Routes user selection to `upgradeTdu()`, `upgradeTdCli()`, or `upgradeFortiClientVPN()`

### cmd/upgrade/tdu.go
- Self-upgrade via `brew update` + `brew upgrade TD-Yofun/tap/tdu`

### cmd/upgrade/tdcli.go (~450 lines)
- 6-step process: get local version → query latest version via GitHub API → compare versions → download (with progress bar & cache) → extract tar.gz and install binary (sudo) → verify
- Requires `HOMEBREW_GITHUB_API_TOKEN` env var for private repo access
- Auto-detects architecture (arm64/amd64), downloads the matching darwin binary
- Cache directory: `~/.cache/tdu/td-cli/{version}/`
- Install path: `/usr/local/bin/td`
- Shared helper functions: `printSection()`, `printStep()`, `printDetail()`, `formatBytes()`, `runSudoCommand()`, `downloadReleaseAsset()` (with progress bar)

### cmd/upgrade/forticlient_vpn.go (~290 lines)
- 6-step process: get local version (via `defaults read` plist) → fetch release by tag from GitHub API → find latest `.mpkg` asset by version sorting → download (with cache) → install via `sudo installer -pkg` → verify
- Uses the fixed release tag `forticlient-vpn` on the `TD-Yofun/tdu-cli` repo to host `.mpkg` assets
- Reuses shared types (`ghRelease`, `ghAsset`) and helpers (`downloadReleaseAsset`, `runSudoCommand`, `formatBytes`) from tdcli.go
- Version comparison supports multi-segment versions (e.g. `7.4.3.1889`)
- Cache directory: `~/.cache/tdu/forticlient-vpn/{version}/`

### cmd/upgrade/exec.go
- Wraps `exec.Command` to allow easy mocking in tests via `newCommand()`

### cmd/fix/fix.go
- Defines the `fix` subcommand
- Uses promptui for interactive selection (with emoji template, same pattern as upgrade)
- Maintains `fixItems` slice — the list of all selectable fix targets
- Routes user selection to `fixFortiClientVPN()`

### cmd/fix/forticlient_vpn.go
- 6-step process: diagnose (check process status) → confirm with user → load system daemons (sudo) → restart FortiClient → start FortiTray → verify
- Fixes two issues: blank screen (caused by stopped `fctservctl2` / `PrivilegedHelper` daemons) and SAML login failure (caused by stopped `FortiTray`)
- Checks process status via `pgrep` before applying fixes (skips already-running services)
- Asks user for confirmation before executing sudo commands
- Uses `launchctl load` / `launchctl start` to restore system daemons
- Has its own `newCommand()`, `printSection()`, `printStep()`, `printDetail()`, `runSudoCommand()` helpers (scoped to the `fix` package)

### cmd/report/report.go
- Defines the `report` subcommand
- Uses promptui for interactive selection (with emoji template, same pattern as upgrade/fix)
- Maintains `reportItems` slice — the list of all selectable report targets
- Routes user selection to `reportFortiClientVPN()`

### cmd/report/forticlient_vpn.go
- 5-step process: collect app info → check service status → collect logs (with optional sudo) → compose & preview issue → submit to GitHub
- Collects: FortiClient version, macOS version, architecture, hostname, service status (processes + launchd daemons)
- Log sources: user-level (`~/Library/Logs/FortiClient/`) and system-level (`/Library/Application Support/Fortinet/FortiClient/Logs/`)
- System logs may require sudo — prompts user for authorization before reading
- Composes a GitHub issue with environment info, service status, and logs in collapsible `<details>` blocks
- Creates issue via GitHub API POST to `TD-Yofun/tdu-cli/issues` using `HOMEBREW_GITHUB_API_TOKEN`
- Shows issue preview and asks for confirmation before submitting
- Labels created issues with `bug` and `forticlient-vpn`

### cmd/utils/utils.go
- Shared utility functions used by `upgrade`, `fix`, and `report` packages
- `NewCommand()`: wraps `exec.Command` for testability
- `PrintSection()`, `PrintStep()`, `PrintDetail()`: emoji-style formatted output
- `RunSudoCommand()`: runs commands with sudo, with prominent permission denied error
- `FormatBytes()`: human-readable byte size formatting
- `RequireGitHubToken()`: checks `HOMEBREW_GITHUB_API_TOKEN` env var with prominent error box

## Build & Release

```bash
# Local build
go build -o tdu .

# Release via GoReleaser (triggered automatically by CI on tag push)
git tag v0.x.x && git push origin v0.x.x
```

GoReleaser v2 configuration (`.goreleaser.yml`):
- Target platforms: darwin/linux × amd64/arm64
- CGO disabled, fully static binaries
- Homebrew cask auto-published to `TD-Yofun/homebrew-tap` (requires `TAP_GITHUB_TOKEN` env)
- Custom Ruby module `GitHubPrivateRepo` in cask for private release asset downloads
- Post-install hook removes macOS quarantine attribute

## Release & Commit Workflows

Both workflows live as on-demand skills under `.agents/skills/`:

- `commit` — [.agents/skills/commit/SKILL.md](.agents/skills/commit/SKILL.md): stage and commit with a Conventional Commits message generated from the diff.
- `release` — [.agents/skills/release/SKILL.md](.agents/skills/release/SKILL.md): commit pending changes, bump semver tag, build-check, tag, and push to trigger GoReleaser CI.

Invoke them via `/commit` or `/release` in chat, or they will load automatically when the user asks to commit or release.

## Coding Conventions

- When adding a new upgradeable tool, create a dedicated file under `cmd/upgrade/` and register it in the `upgradeItems` slice and `switch` statement in `upgrade.go`.
- When adding a new fixable tool, create a dedicated file under `cmd/fix/` and register it in the `fixItems` slice and `switch` statement in `fix.go`.
- When adding a new reportable tool, create a dedicated file under `cmd/report/` and register it in the `reportItems` slice and `switch` statement in `report.go`.
- Wrap external command calls with `newCommand()` to keep them testable.
- Use emoji-style output for user interaction (`printSection`, `printStep`, `printDetail`).
- Follow the 6-step upgrade pattern: check local version → fetch remote version → compare → download (with cache) → install → verify.
- Follow the fix pattern: diagnose → confirm with user → apply fix (with sudo prompt) → restart → verify.
- Follow the report pattern: collect info → check services → collect logs (with optional sudo) → compose & preview → submit to GitHub.
- When sudo is required, clearly list the actions that need elevated privileges and ask the user for confirmation before proceeding.
- Error handling should be detailed, including debug info (missing token, insufficient permissions, missing resources, etc.).
- Provide progress bars and caching for large file downloads.
- Reuse shared helpers from existing files (`downloadReleaseAsset`, `runSudoCommand`, `formatBytes`, `ghRelease`/`ghAsset` types) instead of duplicating logic.
- Require `HOMEBREW_GITHUB_API_TOKEN` for any GitHub API access and provide clear instructions when it's missing.
