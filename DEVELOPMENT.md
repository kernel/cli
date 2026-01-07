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

Prerequisites:

- Make sure you have **goreleaser-pro** installed via `brew install --cask goreleaser/tap/goreleaser-pro`. You will need a license key (in 1pw), and then `export GORELEASER_KEY=<the key>`. **Note: goreleaser-pro is required, not the standard goreleaser version.**

- Grab the NPM token for our org (in 1pw) and run `npm config set '//registry.npmjs.org/:_authToken'=<the token>`

- export a `GITHUB_TOKEN` with repo and write:packages permissions: https://github.com/settings/tokens/new?scopes=repo,write:packages.

With a clean tree on the branch you want to release (can be main or a pr branch you're about to merge, doesn't matter), run:

```bash
make release-dry-run
```

This will check that everything is working, but not actually release anything.
You should see one error about there not being a git tag, and that's fine.

To actually release, run:

```bash
# use `git describe --abbrev=0` to find the latest version and then bump it following https://semver.org/
./scripts/release.sh <version> [description]
```
