# tdu - talkdesk utils

A CLI tool containing various small utilities for daily work.

## Install

```bash
brew install TD-Yofun/tap/tdu
```

## Usage

```bash
# Show version
tdu --version

# Interactive upgrade
tdu upgrade
```

## Development

```bash
# Build
go build -o tdu .

# Run
./tdu --version
./tdu upgrade
```

## Release

```bash
# Tag a new version
git tag v0.1.0
git push origin v0.1.0
# GitHub Actions will automatically build, release, and update Homebrew formula
```

## License

MIT
