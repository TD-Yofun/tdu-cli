# tdu - talkdesk utils

A CLI tool containing various small utilities for daily Talkdesk work.

## Install

```bash
brew install TD-Yofun/tap/tdu
```

## Prerequisites

Some commands require a GitHub personal access token. Add the following to your shell profile (`~/.zshrc` or `~/.bashrc`):

```bash
export HOMEBREW_GITHUB_API_TOKEN=your_github_personal_access_token
```

## Usage

```bash
# Show version
tdu --version

# Interactive upgrade — select a tool to upgrade
tdu upgrade

# Interactive fix — select a known issue to fix
tdu fix
```

### Upgrade Targets

| Target | Description |
|---|---|
| `tdu` | Self-upgrade via Homebrew |
| `td-cli` | Download and install from GitHub Release (Talkdesk/td-cli) |
| `forticlient-vpn` | Download and install FortiClient VPN .mpkg from GitHub Release |

### Fix Targets

| Target | Description |
|---|---|
| `forticlient-vpn` | Fix blank screen and SAML login issues |

## Development

```bash
# Build
go build -o tdu .

# Run
./tdu --version
./tdu upgrade
./tdu fix
```

## Release

```bash
# Tag a new version
git tag v0.x.x
git push origin v0.x.x
# GitHub Actions will automatically build, release, and update Homebrew formula
```

## License

MIT
