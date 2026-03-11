# Kernel CLI

## Cursor Cloud specific instructions

This is a Go CLI application (no long-running services). Development commands are in the `Makefile`:

- **Build:** `make build` → produces `./bin/kernel`
- **Test:** `make test` → runs `go vet` + `go test ./...`
- **Lint:** `make lint` → runs `golangci-lint run` (requires `golangci-lint` on `PATH`)

### Testing against production

When `KERNEL_API_KEY` is set (provided as a secret), the CLI hits the **production** Kernel API. After `make build`, test with `./bin/kernel`. Useful smoke-test sequence:

1. `./bin/kernel auth status` — verify auth
2. `./bin/kernel app list` — list deployed apps
3. `./bin/kernel browsers create` — create a browser session (remember to delete it after)
4. `./bin/kernel browsers delete <session_id>` — clean up

Be mindful that these operations affect production resources.

### Gotchas

- `golangci-lint` is installed via `go install` to `$(go env GOPATH)/bin`. This directory must be on `PATH` (the update script handles this via `.bashrc`).
- The Makefile's `lint` target uses `|| true`, so it always exits 0 even when lint issues exist. Pre-existing lint warnings (errcheck, staticcheck) are present in the codebase and expected.
- The `go-keyring` dependency requires D-Bus and `libsecret` on Linux. These are pre-installed in the Cloud VM.
- `kernel create` works locally without authentication. Most other commands (`deploy`, `invoke`, `browsers`, etc.) require a `KERNEL_API_KEY` env var or `kernel login` OAuth flow.
- Go module path is `github.com/kernel/cli`. The project requires Go 1.25.0 (specified in `go.mod`).
