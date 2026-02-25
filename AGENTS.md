# Kernel CLI

## Cursor Cloud specific instructions

This is a Go CLI application (no long-running services). Development commands are in the `Makefile`:

- **Build:** `make build` → produces `./bin/kernel`
- **Test:** `make test` → runs `go vet` + `go test ./...`
- **Lint:** `make lint` → runs `golangci-lint run` (requires `golangci-lint` on `PATH`)

### Gotchas

- `golangci-lint` is installed via `go install` to `$(go env GOPATH)/bin`. This directory must be on `PATH` (the update script handles this via `.bashrc`).
- The Makefile's `lint` target uses `|| true`, so it always exits 0 even when lint issues exist. Pre-existing lint warnings (errcheck, staticcheck) are present in the codebase and expected.
- The `go-keyring` dependency requires D-Bus and `libsecret` on Linux. These are pre-installed in the Cloud VM.
- `kernel create` works locally without authentication. Most other commands (`deploy`, `invoke`, `browsers`, etc.) require a `KERNEL_API_KEY` env var or `kernel login` OAuth flow.
- Go module path is `github.com/kernel/cli`. The project requires Go 1.25.0 (specified in `go.mod`).
