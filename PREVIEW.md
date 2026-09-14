# Preview binaries

The `Preview CLI binaries` workflow builds same-repository pull requests automatically.
It also supports `workflow_dispatch` after the workflow exists on the default branch.
It builds the PR head, not GitHub's synthetic merge commit. Archives cover Linux,
macOS, and Windows on amd64 and arm64, with `SHA256SUMS`. The embedded version is
`0.0.0-preview.g<commit>`; the full commit is embedded as well.

Download a run's artifact (GitHub authentication required):

```sh
gh run list --repo kernel/cli --workflow preview.yaml --branch <branch>
gh run download <run-id> --repo kernel/cli --name kernel-preview-<full-commit> --dir preview
cd preview
sha256sum -c SHA256SUMS
# macOS: shasum -a 256 -c SHA256SUMS
# Extract the archive matching your operating system and architecture.
tar -xzf kernel_0.0.0-preview.g<commit>_linux_amd64.tar.gz
./kernel --version
./kernel vaults credentials --help
./kernel vaults items invoke --help
```

Use the extracted binary explicitly rather than replacing the stable installation.
Set `KERNEL_BASE_URL` and `KERNEL_API_KEY` for your local/test environment before API
calls; do not assume the production API supports preview features.

Artifacts expire after 14 days. No GitHub release, stable tag, npm package, Homebrew
formula, or production deployment is created. macOS binaries are unsigned; Windows
archives contain `kernel.exe`. Download on trusted machines and verify checksums.

## Temporary SDK pin

`go.mod` replaces the normal Go SDK with the immutable STLC preview revision from
`kernel/kernel` PR #3898. The staging SDK repository requires GitHub authentication.
Tests and preview builds use the existing GitHub App credentials to obtain a
contents-read token scoped to `kernel-go-sdk-staging`, solely for `go mod download`.
No token is passed to compilation/tests, no credential config is persisted, and Go
caching is disabled so private module sources are not exported into Actions caches.
Fork PR jobs are skipped while this private dependency is required. No
`pull_request_target` workflow executes untrusted PR code.

Replace the preview pin with the released `github.com/kernel/kernel-go-sdk` version
before a stable release, then remove the temporary SDK-auth steps and restore fork
testing/cache behavior. The stable release workflow rejects the staging SDK pin.

For a local cross-platform build, authenticate Git for the SDK repository, then run:

```sh
GOPRIVATE=github.com/kernel/kernel-go-sdk-staging go mod download
bash scripts/build-preview.sh
```
