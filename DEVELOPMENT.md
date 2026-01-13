# Kernel CLI

A command-line tool for deploying and invoking Kernel applications.

## Installation

```bash
brew install onkernel/tap/kernel
```

## Development Prerequisites

Install the following tools:

- Go 1.22+ ( https://go.dev/doc/install )
- [Goreleaser Pro](https://goreleaser.com/install/#pro) - **IMPORTANT: You must install goreleaser-pro, not the standard version, as this is required for our release process**
- [chglog](https://github.com/goreleaser/chglog)

Compile the CLI:

```bash
make build   # compiles the binary to ./bin/kernel
```

Run the CLI:

```bash
./bin/kernel --help
```

## Development workflow

Useful make targets:

- `make build` – compile the project to `./bin/kernel`
- `make test` – execute unit tests
- `make lint` – run the linter (requires `golangci-lint`)
- `make changelog` – generate/update the `CHANGELOG.md` file using **chglog**
- `make release` – create a release using **goreleaser** (builds archives, homebrew formula, etc. See below)

### Developing Against API Changes

A typical workflow we encounter is updating the API and integrating those changes into our CLI. The high level workflow is (update API) -> (update SDK) -> (update CLI). Detailed instructions below

1. Get added to https://www.stainless.com/ organization
1. For the given SDK version switch to branch changes - see https://app.stainless.com/docs/guides/branches
1. Update `openapi.stainless.yml` with new endpoint paths, objects, etc
   1. Note: https://github.com/stainless-sdks/kernel-config/blob/main/openapi.stainless.yml is the source of truth. You can pull older versions as necessary
1. Update `openapi.yml` with your changes
1. Iterate in the diagnostics view until all errors are fixed
1. Hit `Save & build branch`
1. This will then create a branch in https://github.com/stainless-sdks/kernel-go
1. Using either your branch name or a specific commit hash you want to point to, run this script to modify the CLI's `go.mod`:

```
./scripts/go-mod-replace-kernel.sh <commit | branch name>
```

### Releasing a new version

Releases are automated via GitHub Actions. Simply push a version tag and the release workflow will handle the rest.

#### To release:

```bash
# Find the latest version
git describe --abbrev=0

# Create and push a new tag (bump version following https://semver.org/)
git tag -a v<VERSION> -m "Version <VERSION>"
git push origin v<VERSION>
```

The release workflow will automatically:
- Build binaries for darwin, linux, and windows (amd64 and arm64)
- Create a GitHub release with changelog
- Publish to npm as `@onkernel/cli`
- Update the Homebrew formula in `onkernel/homebrew-tap`

#### Required GitHub Secrets

The following secrets must be configured in the repository settings:

| Secret | Description |
|--------|-------------|
| `GH_PAT` | GitHub Personal Access Token with `repo` scope. Must have write access to both this repository (for creating releases) and `onkernel/homebrew-tap` (for updating the Homebrew formula). Create at https://github.com/settings/tokens/new?scopes=repo |
| `GORELEASER_KEY` | GoReleaser Pro license key (required for npm and homebrew publishing) |
| `NPM_TOKEN` | npm access token for publishing `@onkernel/cli` |

#### Local dry-run (optional)

To test the release process locally before pushing a tag:

Prerequisites:
- Install **goreleaser-pro** via `brew install --cask goreleaser/tap/goreleaser-pro`
- Export `GORELEASER_KEY=<license key from 1pw>`

```bash
make release-dry-run
```

This will check that everything is working without actually releasing anything.
