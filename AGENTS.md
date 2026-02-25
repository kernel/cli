# Kernel CLI

Go CLI for managing Kernel browser sandboxes (deploy apps, manage browsers, etc.).

## Cursor Cloud specific instructions

### Project overview

Single Go CLI binary — no services, databases, or Docker needed. All commands hit the remote Kernel API (`api.onkernel.com`).

### Build, test, lint

Standard commands are in the `Makefile`:

- `make build` — compiles to `./bin/kernel`
- `make test` — runs `go vet` + `go test ./...`
- `make lint` — runs `golangci-lint run` (exits 0 via `|| true`; existing lint warnings are expected)

### Caveats

- **Go 1.25+** is required (`go.mod` specifies `go 1.25.0`).
- `golangci-lint` is installed via `go install` and lives in `~/go/bin`. The PATH is configured in `~/.bashrc` to include `$HOME/go/bin`.
- The Makefile `lint` target uses `|| true`, so `make lint` always exits 0 even when there are warnings. Run `golangci-lint run` directly to see the real exit code.
- E2E testing against the live Kernel platform requires a `KERNEL_API_KEY` env var or completing `kernel login` OAuth flow. Unit tests (`make test`) do not require authentication.
- `kernel create` is the best offline smoke test — it scaffolds a project from embedded templates without needing API credentials.
- Use `testify/require` and `testify/assert` for test assertions (per workspace coding standards).
