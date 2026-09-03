<p align="center">
  <img src="https://raw.githubusercontent.com/kernel/kernel-images/main/static/images/logo-kernel-light.svg" alt="Kernel Logo" width="55%">
</p>

<p align="center">
  <img alt="GitHub License" src="https://img.shields.io/github/license/kernel/cli">
  <a href="https://discord.gg/FBrveQRcud"><img src="https://img.shields.io/discord/1342243238748225556?logo=discord&logoColor=white&color=7289DA" alt="Discord"></a>
  <a href="https://x.com/juecd__"><img src="https://img.shields.io/twitter/follow/juecd__" alt="Follow @juecd__"></a>
  <a href="https://x.com/rfgarcia"><img src="https://img.shields.io/twitter/follow/rfgarcia" alt="Follow @rfgarcia"></a>
</p>

# Kernel CLI

The Kernel CLI is a fast, friendly command‑line interface for Kernel — the platform that provides sandboxed, ready‑to‑use Chrome browsers for browser automations and web agents.

Sign up at [kernel.sh](https://www.kernel.sh/) and read the [docs](https://www.kernel.sh/docs/introduction).

## What's Kernel?

Kernel provides sandboxed, ready-to-use Chrome browsers for browser automations and web agents. This CLI helps you deploy apps, run actions, manage browsers, and access live views.

### What you can do with the CLI

- Create new Kernel applications from templates
- Deploy and version apps to Kernel
- Invoke app actions (sync or async) and stream logs
- Create, list, view, and delete managed browser sessions
- Get a live view URL for visual monitoring and remote control

## Installation

Install the Kernel CLI using your favorite package manager:

```bash
# Using brew (recommended)
brew install onkernel/tap/kernel

# Using pnpm
pnpm install -g @onkernel/cli

# Using npm
npm install -g @onkernel/cli
```

Verify the installation:

```bash
which kernel
kernel --version
```

## Quick Start

1. **Create a new Kernel app:**

   ```bash
   kernel create
   ```

2. **Authenticate with Kernel:**

   ```bash
   kernel login
   ```

3. **Deploy your app:**

   ```bash
   kernel deploy index.ts
   ```

4. **Invoke your app:**
   ```bash
   kernel invoke my-app action-name --payload '{"key": "value"}'
   ```

## Authentication

### OAuth 2.0 (Recommended)

The easiest way to authenticate is using OAuth:

```bash
kernel login
```

This opens your browser to complete the authentication flow. Choose organization-wide access or restrict the login to one project. Your credentials are securely stored, automatically refreshed, and retain the selected scope until you log in again.

### API Key

You can also authenticate using an API key:

```bash
export KERNEL_API_KEY=<YOUR_API_KEY>
```

Create an API key from the [Kernel dashboard](https://dashboard.onkernel.com).

## Commands Reference

### Global Flags

- `--version`, `-v` - Print the CLI version
- `--no-color` - Disable color output
- `--log-level <level>` - Set log level (trace, debug, info, warn, error, fatal, print)
- `--project <id-or-name>` - Scope requests to a project by ID or exact name (or set `KERNEL_PROJECT`). Project-scoped OAuth tokens cannot switch projects.

## JSON Output

Many commands support JSON output for scripting and automation. Use `--output json` or `-o json` to get machine-readable output:

```bash
# Get browser session details as JSON
kernel browsers create -o json

# List apps as JSON
kernel app list -o json

# Deploy with JSONL streaming output (one JSON object per line)
kernel deploy index.ts -o json
```

Commands with JSON output support:
- **Browsers**: `create`, `list`, `get`, `view`
- **Browser Pools**: `create`, `list`, `get`, `update`, `acquire`
- **Profiles**: `create`, `list`, `get`, `update`
- **Extensions**: `upload`, `list`
- **Proxies**: `create`, `list`, `get`, `update`, `check`
- **API Keys**: `create`, `list`, `get`, `update`, `rotate`
- **Auth Connections**: `timeline`
- **Projects**: `update`
- **Org**: `limits get/set`
- **Apps**: `list`, `history`
- **Deploy**: `deploy` (JSONL streaming), `history`
- **Invoke**: `invoke` (JSONL streaming), `history`
- **Browser Sub-commands**: `replays list/start`, `process exec/spawn`, `fs file-info/list-files`
- **Browser NDJSON streaming**: `telemetry stream`

### Authentication

- `kernel login [--force]` - Login via OAuth 2.0
- `kernel logout` - Clear stored credentials
- `kernel auth` - Check authentication status

### App Creation

- `--name <name>`, `-n` - Name of the application
- `--language <language>`, `-l` - Sepecify app language: `typescript`, or `python`
- `--template <template>`, `-t` - Template to use:
  - `sample-app` - Basic template with Playwright integration
  - `captcha-solver` - Template demonstrating Kernel's auto-CAPTCHA solver
  - `stagehand` - Template with Stagehand SDK (TypeScript only)
  - `browser-use` - Template with Browser Use SDK (Python only)
  - `anthropic-computer-use` - Anthropic Computer Use prompt loop
  - `openai-computer-use` - OpenAI Computer Use Agent sample
  - `gemini-computer-use` - Implements a Gemini computer use agent (TypeScript only)
  - `openagi-computer-use` - OpenAGI Lux computer-use models (Python only)
  - `claude-agent-sdk` - Claude Agent SDK browser automation agent

### App Deployment

- `kernel deploy <file>` - Deploy an app to Kernel

  - `--version <version>` - Specify app version (default: latest)
  - `--force` - Allow overwriting existing version
  - `--env <KEY=VALUE>`, `-e` - Set environment variables (can be used multiple times)
  - `--env-file <file>` - Load environment variables from file (can be used multiple times)
  - `--output json`, `-o json` - Output JSONL (one JSON object per line for each event)

- `kernel deploy logs <deployment_id>` - Stream logs for a deployment

  - `--follow`, `-f` - Follow logs in real-time (stream continuously)
  - `--since`, `-s` - How far back to retrieve logs. Duration formats: ns, us, ms, s, m, h (e.g., 5m, 2h, 1h30m). Timestamps also supported: 2006-01-02, 2006-01-02T15:04, 2006-01-02T15:04:05, 2006-01-02T15:04:05.000
  - `--with-timestamps`, `-t` - Include timestamps in each log line

- `kernel deploy history [app_name]` - Show deployment history
  - `--limit <n>` - Max deployments to return (default: 100; 0 = all)
  - `--output json`, `-o json` - Output raw JSON array

### App Management

- `kernel invoke <app> <action>` - Run an app action

  - `--version <version>`, `-v` - Specify app version (default: latest)
  - `--payload <json>`, `-p` - JSON payload for the action
  - `--payload-file <path>`, `-f` - Read JSON payload from a file (use `-` for stdin)
  - `--sync`, `-s` - Invoke synchronously (timeout after 60s)
  - `--output json`, `-o json` - Output JSONL (one JSON object per line for each event)

- `kernel app list` - List deployed apps

  - `--name <app_name>` - Filter by app name
  - `--version <version>` - Filter by version
  - `--output json`, `-o json` - Output raw JSON array

- `kernel app history <app_name>` - Show deployment history for an app
  - `--limit <n>` - Max deployments to return (default: 100; 0 = all)
  - `--output json`, `-o json` - Output raw JSON array

### Logs

- `kernel logs <app_name>` - View app logs
  - `--version <version>` - Specify app version (default: latest)
  - `--follow`, `-f` - Follow logs in real-time
  - `--since <time>`, `-s` - How far back to retrieve logs (e.g., 5m, 1h)
  - `--with-timestamps` - Include timestamps in log output

### Browser Management

- `kernel browsers list` - List running browsers
  - `--query <q>` - Search by name, session ID, profile ID, proxy ID, or pool name
  - `--region us-east|eu-west|ap-southeast` - Filter by geographic region; omit to list sessions in all regions
  - `--tag <KEY=VALUE>` - Filter by tag, repeatable; a session must match every pair
  - `--output json`, `-o json` - Output raw JSON array
- `kernel browsers create` - Create a new browser session
  - `-s, --stealth` - Launch browser in stealth mode to avoid detection
  - `-H, --headless` - Launch browser without GUI access
  - `--kiosk` - Launch browser in kiosk mode
  - `--region us-east|eu-west|ap-southeast` - Geographic region for the session. Fixed once the session is created; requires a Start-Up or Enterprise plan and defaults to `us-east`.
  - `--private-host <host>` - Destination the browser reaches directly through the session's own network instead of Kernel-managed egress, for private hosts on a VPN or tunnel the session joins (repeatable or comma-separated, max 32). Accepts hostname patterns (`*.example.ts.net`), IPs (`10.1.30.63`, `[fd00::1]`), and private CIDRs (`100.64.0.0/10`). Replaces the default private ranges (RFC1918, `100.64.0.0/10`, `fc00::/7`); omit to keep them. Fixed once the session is created. Unrelated to a proxy's `--bypass-host`, which only chooses between upstream proxy and Kernel-managed direct egress.
  - `--start-url <url>` - Initial page to open on launch
  - `--proxy-id <id>` / `--proxy-name <name>` - Use that proxy for the session regardless of stealth (mutually exclusive with each other and with `--proxy-mode`)
  - `--proxy-mode direct|default` - Egress mode instead of a selected proxy: `direct` for no proxy regardless of stealth, `default` for the stealth-derived default (Kernel's stealth proxy with `--stealth`, direct egress otherwise). Omit all proxy flags to get the default.
  - `--name <name>` - Optional unique name for the session (used to find it later by name; can be changed with `browsers update --name`)
  - `--tag <KEY=VALUE>` - Set a tag on the session, repeatable; up to 50 pairs
  - `--pool-id <id>` - Acquire a browser from the specified pool (mutually exclusive with --pool-name; ignores other session flags). `--name`/`--tag` still apply to the acquired session.
  - `--pool-name <name>` - Acquire a browser from the pool name (mutually exclusive with --pool-id; ignores other session flags)
  - `--telemetry=all` - Enable telemetry for all categories
  - `--telemetry=off` - Disable telemetry
  - `--telemetry=<list>` - Per-category config, e.g. `--telemetry=network=on,page=off`
  - `--telemetry-export-otlp <id-or-name>` - Export captured telemetry over OTLP to one of the org's configured destinations. Implies `--telemetry=all` when `--telemetry` is not set, since export requires capture. Use `--telemetry-export-otlp=off` to disable export.
  - `--chrome-policy <json>` - Custom Chrome enterprise policy as a JSON object. Kernel-managed policies (extensions, proxy, automation) are rejected server-side.
  - `--chrome-policy-file <path>` - Read the Chrome enterprise policy from a file (use `-` for stdin). Mutually exclusive with `--chrome-policy`.
  - `--output json`, `-o json` - Output raw JSON object
  - _Note: When a pool is specified, omit other session configuration flags—pool settings determine profile, proxy, viewport, etc._
- `kernel browsers delete <id-or-name>` - Delete a browser by ID or name
- `kernel browsers view <id-or-name>` - Get live view URL for a browser by ID or name
  - `--output json`, `-o json` - Output JSON with liveViewUrl
- `kernel browsers get <id-or-name>` - Get detailed browser session info by ID or name
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers update <id-or-name>` - Update a running browser session by ID or name
  - `--name <name>` - Set a new unique name for the session (mutually exclusive with `--clear-name`)
  - `--clear-name` - Clear the session name
  - `--tag <KEY=VALUE>` - Set a tag, repeatable; up to 50 pairs. Replaces the entire tag set (not merged); mutually exclusive with `--clear-tags`
  - `--clear-tags` - Remove all tags from the session
  - `--telemetry=all` - Enable telemetry for all categories
  - `--telemetry=off` - Disable telemetry
  - `--telemetry=<list>` - Per-category config, e.g. `--telemetry=network=on,page=off`
  - `--proxy-id <id>` / `--proxy-name <name>` - Switch the session to that proxy regardless of stealth (mutually exclusive with each other and with `--proxy-mode`)
  - `--proxy-mode direct|default` - Change egress mode: `direct` for no proxy regardless of stealth, `default` to restore the browser default after using a selected proxy. Changing the proxy does not change stealth or CAPTCHA solver behavior.
  - `--clear-proxy` - Drop the selected proxy and restore the browser default (same as `--proxy-mode=default`)
  - `--disable-default-proxy` - Connect directly instead of through the default stealth proxy (same as `--proxy-mode=direct`); use `--disable-default-proxy=false` to restore the default
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers curl <id> <url>` - Make HTTP requests through a browser session's Chrome network stack
  - `-X, --request <method>` - HTTP method (default: GET; defaults to POST when `--data` is set)
  - `-H, --header <header>` - HTTP header, repeatable (`"Key: Value"` format)
  - `-d, --data <body>` - Request body
  - `--data-file <path>` - Read request body from file
  - `--max-time <seconds>` - Maximum time allowed for the request (default: 30)
  - `-o, --output <path>` - Write response body to file
  - `-I, --head` - Fetch headers only
  - `-i, --include` - Include response headers in output
  - `-D, --dump-header <path>` - Write received headers to file (use `-` for stdout)
  - `-w, --write-out <format>` - Output text after completion; supports `%{http_code}`, `%{response_code}`, `%{time_total}`, and `%{size_download}`
  - `-f, --fail` - Fail with no body output on HTTP errors
  - `-s, --silent` - Suppress progress output
  - _Note: redirects are followed automatically by Chromium._

### Browser Pools

- `kernel browser-pools list` - List browser pools
  - `--region us-east|eu-west|ap-southeast` - Filter by geographic region; omit to list pools in all regions
  - `--output json`, `-o json` - Output raw JSON array
- `kernel browser-pools create` - Create a browser pool
  - `--name <name>` - Optional unique name for the pool
  - `--size <n>` - Number of browsers in the pool (required)
  - `--fill-rate <n>` - Percentage of the pool to fill per minute
  - `--timeout <seconds>` - Idle timeout for browsers acquired from the pool
  - `--stealth`, `--headless`, `--kiosk` - Default pool configuration
  - `--refresh-on-profile-update` - Flush idle browsers when the pool's profile is updated (requires a profile)
  - `--profile-id`, `--profile-name`, `--proxy-id`, `--region`, `--start-url`, `--extension`, `--viewport`, `--private-host` - Same semantics as `kernel browsers create`
  - `--chrome-policy <json>` / `--chrome-policy-file <path>` - Custom Chrome enterprise policy applied to every browser in the pool, as a JSON object or from a file (`-` for stdin). Same semantics as `kernel browsers create`.
  - `--telemetry=all` / `--telemetry=off` / `--telemetry=<categories>` - Telemetry applied to browsers warmed into the pool. Same semantics as `kernel browsers create`.
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools get <id-or-name>` - Get pool details
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools update <id-or-name>` - Update pool configuration
  - Same flags as create (except `--region`, which is fixed at creation and cannot be updated) plus `--clear-profile`, `--clear-proxy`, `--clear-start-url`, `--clear-extensions`, `--clear-chrome-policy`, and `--clear-private-hosts` for removing durable configuration. `--clear-private-hosts` restores the default private IP ranges. `--fill-rate 0` pauses automatic filling. `--discard-all-idle` discards all idle browsers and refills the pool. `--telemetry` and private-host updates only apply to browsers warmed after the update.
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools delete <id-or-name>` - Delete a pool
  - `--force` - Force delete even if browsers are leased
- `kernel browser-pools acquire <id-or-name>` - Acquire a browser from the pool
  - `--timeout <seconds>` - Acquire timeout before returning 204
  - `--name <name>` - Optional name for the acquired session (applies to this lease; cleared on release)
  - `--tag <KEY=VALUE>` - Set a tag on the acquired session, repeatable; applies to this lease
  - `--telemetry=all` / `--telemetry=off` / `--telemetry=<categories>` - Telemetry override for this lease only, merged onto the pool's config
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools release <id-or-name>` - Release a browser back to the pool
  - `--session-id <id>` - Browser session ID to release (required)
  - `--reuse` - Reuse the browser instance (default: true)
- `kernel browser-pools flush <id-or-name>` - Destroy all idle browsers in the pool

### Browser Logs

- `kernel browsers logs stream <id>` - Stream browser logs
  - `--source <source>` - Log source: "path" or "supervisor" (required)
  - `--follow` - Follow the log stream (default: true)
  - `--path <path>` - File path when source=path
  - `--supervisor-process <name>` - Supervisor process name when source=supervisor. Most useful value is "chromium"

### Browser Replays

- `kernel browsers replays list <id>` - List replays for a browser
  - `--output json`, `-o json` - Output raw JSON array
- `kernel browsers replays start <id>` - Start a replay recording
  - `--framerate <fps>` - Recording framerate (fps)
  - `--max-duration <seconds>` - Maximum duration in seconds
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers replays stop <id> <replay-id>` - Stop a replay recording
- `kernel browsers replays download <id> <replay-id>` - Download a replay video
  - `-f, --output-file <path>` - Output file path for the replay video

### Browser Telemetry

Telemetry config is a sub-field of the browser session. Use `browsers create` or `browsers update` to enable, disable, or configure it, and `browsers get` to inspect the current state.

- Enable the default set: `kernel browsers update <id> --telemetry=all`
- Disable: `kernel browsers update <id> --telemetry=off`
- Capture specific categories: `kernel browsers update <id> --telemetry=console,network` (any of: `console`, `network`, `page`, `interaction`, `control`, `connection`, `system`, `screenshot`, `captcha`)

Per-category updates are partial — only categories you name are changed; others retain their current state. `--telemetry=all` and `--telemetry=off` reset the entire config.

#### Exporting telemetry

Captured telemetry can be exported over OTLP to one of the org's configured destinations with `--telemetry-export-otlp <id-or-name>`. A value that looks like an ID is sent as one; anything else is resolved as a destination name, which must match exactly one destination in the org.

- Capture and export: `kernel browsers create --telemetry-export-otlp my-collector`
- Capture without exporting: `kernel browsers create --telemetry=all`
- Stop exporting: `--telemetry-export-otlp=off`

Export is bound at session creation, so it is available on `browsers create` and on the managed-auth commands that create a browser (`auth connections create`, `update`, and `login`). A browser session keeps the destination it was created with — `browsers update` cannot change it — and browser pools do not support export.

#### Telemetry destinations

Destinations are the OTLP/HTTP endpoints sessions export to, managed per project.

- `kernel telemetry destinations list` - List OTLP destinations
  - `--page <n>` / `--per-page <n>` - Page number (1-based) and items per page (default 20)
  - `--name <name>` - Filter by exact destination name
  - `--query <text>` - Substring match against name or endpoint; IDs match by exact value
- `kernel telemetry destinations get <id-or-name>` - Get an OTLP destination
- `kernel telemetry destinations create --name <name> --endpoint <url>` - Create an OTLP destination
  - `--endpoint <url>` - Base OTLP/HTTP endpoint without a signal path: pass `https://api.honeycomb.io`, not `https://api.honeycomb.io/v1/logs` (required)
  - `--name <name>` - Destination name, unique within the project (required)
  - `--description <text>` - Optional description
  - `--header NAME=VALUE` - Header sent with each export request, typically an ingestion key (repeatable). Values are encrypted at rest and always returned redacted, so only header names are shown
- `kernel telemetry destinations update <id-or-name>` - Update an OTLP destination. Sessions already exporting pick up the new values without restarting, which makes this the way to rotate credentials without interrupting export
  - `--name <name>` / `--endpoint <url>` / `--description <text>` - Update those fields; pass `--description ""` to clear it
  - `--header NAME=VALUE` - Add or replace a header (repeatable). Headers you do not name are left as they are
  - `--remove-header NAME` - Delete a header (repeatable). Removals are applied before `--header` is merged, so a header given to both keeps its new value
- `kernel telemetry destinations delete <id-or-name>` - Delete an OTLP destination. Refused while sessions are still exporting to it, or while a managed auth connection still selects it
  - `-y, --yes` - Skip confirmation prompt

- `kernel browsers telemetry stream <id>` - Stream live telemetry events (NDJSON with `-o json`)
  - `--categories <list>` - Filter by event category (`console`, `network`, `page`, `interaction`, `control`, `connection`, `system`, `screenshot`, `captcha`, `monitor`)
  - `--types <list>` - Filter by event type (e.g. `network_response`, `console_error`)
  - `--seq <n>` - Resume after sequence number N (Last-Event-ID); replays events with `seq > N`. Omit to stream from now.
  - `--replay all` - Replay buffered events on connect, starting from the oldest retained event (mutually exclusive with `--seq`)
  - `-o, --output json` - Output newline-delimited JSON envelopes
  - Default output: tab-separated `<time>\t[<category>]\t<type>`, e.g. `15:04:05  [network]  network_response`
- `kernel browsers telemetry events <id>` - Read historical telemetry events (paged)
  - `--limit <n>` - Maximum number of events per page (1-100, default 20)
  - `--offset <cursor>` - Pagination cursor: pass the `X-Next-Offset` from a previous response
  - `--since <ts|dur>` / `--until <ts|dur>` - Time window (RFC-3339 timestamp or duration like `5m`). `--since` is ignored when `--offset` is set; `--until` still bounds the page
  - `--categories <list>` - Filter by event category (`console`, `network`, `page`, `interaction`, `control`, `connection`, `system`, `screenshot`, `captcha`, `monitor`); filtered server-side
  - `--types <list>` - Filter by event type (e.g. `network_response`, `console_error`); filtered client-side, so this walks every page in the window for complete results
  - `--all` - Walk every page in the window instead of just the first (ignores `--offset`; no `next_offset` is returned)
  - `-o, --output json` - Output `{ "events": [...], "next_offset": "..." }` (omit `next_offset` when there is no next page)

### Browser Process Control

- `kernel browsers process exec <id> [--] [command...]` - Execute a command synchronously
  - `--command <cmd>` - Command to execute (optional; if omitted, trailing args are executed via /bin/bash -c)
  - `--args <args>` - Command arguments
  - `--cwd <path>` - Working directory
  - `--timeout <seconds>` - Timeout in seconds
  - `--as-user <user>` - Run as user
  - `--as-root` - Run as root
  - `--env <KEY=VALUE>` - Environment variable to set for the process (repeatable)
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers process spawn <id> [--] [command...]` - Execute a command asynchronously
  - `--command <cmd>` - Command to execute (optional; if omitted, trailing args are executed via /bin/bash -c)
  - `--args <args>` - Command arguments
  - `--cwd <path>` - Working directory
  - `--timeout <seconds>` - Timeout in seconds
  - `--as-user <user>` - Run as user
  - `--as-root` - Run as root
  - `--env <KEY=VALUE>` - Environment variable to set for the process (repeatable)
  - `--allocate-tty` - Allocate a pseudo-terminal (PTY) for interactive shells
  - `--cols <n>` - Initial terminal columns (requires `--allocate-tty`)
  - `--rows <n>` - Initial terminal rows (requires `--allocate-tty`)
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers process kill <id> <process-id>` - Send a signal to a process
  - `--signal <signal>` - Signal to send: TERM, KILL, INT, HUP (default: TERM)
- `kernel browsers process status <id> <process-id>` - Get process status
- `kernel browsers process stdin <id> <process-id>` - Write to process stdin (base64)
  - `--data-b64 <data>` - Base64-encoded data to write to stdin (required)
- `kernel browsers process stdout-stream <id> <process-id>` - Stream process stdout/stderr

### Browser Filesystem

- `kernel browsers fs new-directory <id>` - Create a new directory
  - `--path <path>` - Absolute directory path to create (required)
  - `--mode <mode>` - Directory mode (octal string)
- `kernel browsers fs delete-directory <id>` - Delete a directory
  - `--path <path>` - Absolute directory path to delete (required)
- `kernel browsers fs delete-file <id>` - Delete a file
  - `--path <path>` - Absolute file path to delete (required)
- `kernel browsers fs download-dir-zip <id>` - Download a directory as zip
  - `--path <path>` - Absolute directory path to download (required)
  - `-o, --output <path>` - Output zip file path
- `kernel browsers fs file-info <id>` - Get file or directory info
  - `--path <path>` - Absolute file or directory path (required)
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers fs list-files <id>` - List files in a directory
  - `--path <path>` - Absolute directory path (required)
  - `--output json`, `-o json` - Output raw JSON array
- `kernel browsers fs move <id>` - Move or rename a file or directory
  - `--src <path>` - Absolute source path (required)
  - `--dest <path>` - Absolute destination path (required)
- `kernel browsers fs read-file <id>` - Read a file
  - `--path <path>` - Absolute file path (required)
  - `-o, --output <path>` - Output file path (optional)
- `kernel browsers fs set-permissions <id>` - Set file permissions or ownership
  - `--path <path>` - Absolute path (required)
  - `--mode <mode>` - File mode bits (octal string) (required)
  - `--owner <user>` - New owner username or UID
  - `--group <group>` - New group name or GID
- `kernel browsers fs upload <id>` - Upload one or more files
  - `--file <local:remote>` - Mapping local:remote (repeatable)
  - `--dest-dir <path>` - Destination directory for uploads
  - `--paths <paths>` - Local file paths to upload
- `kernel browsers fs upload-zip <id>` - Upload a zip and extract it
  - `--zip <path>` - Local zip file path (required)
  - `--dest-dir <path>` - Destination directory to extract to (required)
- `kernel browsers fs write-file <id>` - Write a file from local data
  - `--path <path>` - Destination absolute file path (required)
  - `--mode <mode>` - File mode (octal string)
  - `--source <path>` - Local source file path (required)

### Browser Extensions

- `kernel browsers extensions upload <id> <extension-path>...` - Ad-hoc upload of one or more unpacked extensions to a running browser instance.

### Browser Computer Controls

- `kernel browsers computer click-mouse <id>` - Click mouse at coordinates
  - `--x <coordinate>` - X coordinate (required)
  - `--y <coordinate>` - Y coordinate (required)
  - `--num-clicks <n>` - Number of clicks (default: 1)
  - `--button <button>` - Mouse button: left, right, middle, back, forward (default: left)
  - `--click-type <type>` - Click type: down, up, click (default: click)
  - `--hold-key <key>` - Modifier keys to hold (repeatable)
- `kernel browsers computer move-mouse <id>` - Move mouse to coordinates
  - `--x <coordinate>` - X coordinate (required)
  - `--y <coordinate>` - Y coordinate (required)
  - `--hold-key <key>` - Modifier keys to hold (repeatable)
- `kernel browsers computer screenshot <id>` - Capture a screenshot
  - `--to <path>` - Output file path for the PNG image (required)
  - `--x <coordinate>` - Top-left X for region capture (optional)
  - `--y <coordinate>` - Top-left Y for region capture (optional)
  - `--width <pixels>` - Region width (optional)
  - `--height <pixels>` - Region height (optional)
- `kernel browsers computer type <id>` - Type text on the browser instance

  - `--text <text>` - Text to type (required)
  - `--delay <ms>` - Delay in milliseconds between keystrokes (optional)

- `kernel browsers computer press-key <id>` - Press one or more keys

  - `--key <key>` - One X11 keysym or chord, such as `Return`, `Ctrl+t`, or `Ctrl+minus` (repeatable)
  - `--duration <ms>` - Duration to hold keys down in ms (0=tap)
  - `--hold-key <key>` - Modifier keys to hold (repeatable)

  Pass sequential or repeated key presses as separate `--key` values, for example `--key BackSpace --key BackSpace`. Use `computer type --text` to enter text instead of passing text to `press-key`.

- `kernel browsers computer scroll <id>` - Scroll the mouse wheel

  - `--x <coordinate>` - X coordinate (required)
  - `--y <coordinate>` - Y coordinate (required)
  - `--delta-x <pixels>` - Horizontal scroll amount (+right, -left)
  - `--delta-y <pixels>` - Vertical scroll amount (+down, -up)
  - `--hold-key <key>` - Modifier keys to hold (repeatable)

- `kernel browsers computer drag-mouse <id>` - Drag the mouse along a path
  - `--point <x,y>` - Add a point as x,y (repeatable)
  - `--delay <ms>` - Delay before dragging starts in ms
  - `--button <button>` - Mouse button: left, middle, right (default: left)
  - `--hold-key <key>` - Modifier keys to hold (repeatable)

### Browser Playwright

- `kernel browsers playwright execute <id> [code]` - Execute Playwright/TypeScript code against the browser
  - `--timeout <seconds>` - Maximum execution time in seconds (defaults server-side)
  - If `[code]` is omitted, code is read from stdin

### Profiles

- `kernel profiles update <id-or-name> --name <new-name>` - Rename a profile
  - `--name <name>` - New unique profile name (required)
  - `--output json`, `-o json` - Output raw JSON object
- `kernel profiles download <id-or-name> --to <dir>` - Download a profile archive
  - `--to <dir>` - Directory to extract the profile into (required)
  - `--format <format>` - Archive format to request: `tar.zst` (compressed, default) or `tar` (decompressed server-side)

### Projects

- `kernel projects list` - List projects (up to 100 by default)
  - `--limit <n>` - Maximum number of projects to return (1-100, default 100)
  - `--offset <n>` - Number of projects to skip; table indexes match this offset
  - `--output json`, `-o json` - Output `{ "projects": [...], "next_offset": <n> }`; `next_offset` is omitted on the last page
  - When more projects are available, the CLI prints the exact command to fetch the next page
- `kernel projects get <id-or-name>` - Show a project's details
- `kernel projects update <id-or-name>` - Update a project's name or status
  - `--name <name>` - New project name (1-255 characters; cannot contain `/` or `%`)
  - `--status <status>` - New project status: `active` or `archived`
  - `--output json`, `-o json` - Output raw JSON object
- `kernel projects delete <id-or-name>` - Soft-delete a project (must have no active resources)
- `kernel projects limits get <id-or-name>` - Show a project's resource limit overrides
  - `--output json`, `-o json` - Output raw JSON object
- `kernel projects limits set <id-or-name>` - Update a project's resource limit overrides
  - `--max-concurrent-sessions <n>` - Cap on concurrent browser sessions (0 removes the cap)
  - `--max-concurrent-invocations <n>` - Cap on concurrent invocations (0 removes the cap)
  - `--max-pooled-sessions <n>` - Cap on pooled browser sessions (0 removes the cap)
  - `--output json`, `-o json` - Output raw JSON object

Every `<id-or-name>` above is resolved by the API, so a project name works
anywhere a project ID does.

### Extension Management

- `kernel extensions list` - List all uploaded extensions, including available checksums
  - `--output json`, `-o json` - Output raw JSON array
- `kernel extensions get <id-or-name>` - Show extension metadata (id, name, checksum, created, size, last used)
  - `--output json`, `-o json` - Output raw JSON object
- `kernel extensions upload <directory>` - Upload an unpacked browser extension directory
  - `--name <name>` - Optional unique extension name
  - `--output json`, `-o json` - Output raw JSON object
  - Successful uploads show the stored archive checksum
- `kernel extensions download <id-or-name>` - Download an extension archive
  - `--to <directory>` - Output directory (required)
- `kernel extensions download-web-store <url>` - Download an extension from the Chrome Web Store
  - `--to <directory>` - Output directory (required)
  - `--os <os>` - Target OS: mac, win, or linux (default: linux)
- `kernel extensions delete <id-or-name>` - Delete an extension by ID or name
  - `-y, --yes` - Skip confirmation prompt

### Proxy Management

- `kernel proxies list` - List proxy configurations
  - `--output json`, `-o json` - Output raw JSON array
- `kernel proxies get <id>` - Get a proxy configuration by ID
  - `--output json`, `-o json` - Output raw JSON object
- `kernel proxies create` - Create a new proxy configuration
  - `--output json`, `-o json` - Output raw JSON object

  - `--name <name>` - Proxy configuration name (required)
  - `--type <type>` - Proxy type: datacenter, isp, residential, mobile, custom (required)
  - `--protocol <http|https>` - Protocol to use (default: https)
  - `--country <code>` - ISO 3166 country code or "EU" (location-based types)
  - `--city <name>` - City name (no spaces, e.g. sanfrancisco) (residential, mobile; requires `--country`)
  - `--state <code>` - Two-letter state code (residential, mobile)
  - `--zip <zip>` - US ZIP code (residential)
  - `--asn <asn>` - Autonomous system number (e.g., AS15169) (residential)
  - `--os <os>` - Operating system: windows, macos, android (residential)
  - `--host <host>` - Proxy host (custom; required)
  - `--port <port>` - Proxy port (custom; required)
  - `--username <username>` - Username for proxy authentication (custom)
  - `--password <password>` - Password for proxy authentication (custom)
  - `--ca-bundle <path>` - Path to a PEM-encoded CA certificate bundle (custom TLS-terminating proxies)

- `kernel proxies update <id>` - Rename a proxy configuration (recreate the proxy to change its type or config)
  - `--name <name>` - New proxy name (required)
  - `--output json`, `-o json` - Output raw JSON object
- `kernel proxies check <id>` - Run a health check on a proxy to verify it's working and update its status
  - `--url <url>` - Optional public HTTP or HTTPS URL to test reachability against
  - `--output json`, `-o json` - Output raw JSON object
- `kernel proxies delete <id>` - Delete a proxy configuration
  - `-y, --yes` - Skip confirmation prompt

### Auth Context

- `kernel auth context` - Show the identity and authorization context resolved for the current credentials: the authenticated principal, organization, credential scope, and the effective scope for the request. Credential secrets are never returned. Pass `--project <id>` to see the effective scope a project-scoped request would get.
  - `--output json`, `-o json` - Output raw JSON object

### Auth Connections

Managed auth connections (`kernel auth connections`). The commands below are new or gained new flags; run `kernel auth connections --help` for the full command list.

- `kernel auth connections timeline <id>` - List the connection's login, reauth, and health-check events, most recent first
  - `--type <type>` - Filter to a single event type: `login`, `reauth`, or `health_check`
  - `--page <n>` - Page number (1-based, default: 1)
  - `--per-page <n>` - Items per page (default: 20)
  - `--output json`, `-o json` - Output raw JSON array
- `kernel auth connections create` - New flags:
  - `--proxy-id <id>` / `--proxy-name <name>` / `--proxy-mode direct|default` - Proxy configuration for this connection's login, reauth, and health-check browser sessions (mutually exclusive). Omit to derive the default from stealth.
  - `--stealth` - Whether those browser sessions run in stealth mode (default: true); use `--stealth=false` to disable
  - `--telemetry=all` / `--telemetry=off` / `--telemetry=<categories>` - Default telemetry for this connection's browser sessions. Same semantics as `kernel browsers create`
  - `--telemetry-export-otlp <id-or-name>` - Export this connection's captured telemetry over OTLP to one of the org's configured destinations. Implies `--telemetry=all` when `--telemetry` is not set. Use `=off` to disable export.
- `kernel auth connections update <id>` - New flags:
  - `--proxy-id <id>` / `--proxy-name <name>` / `--proxy-mode direct|default` - Proxy configuration for future browser sessions (mutually exclusive). Use `--proxy-mode=default` to drop a selected proxy rather than passing an empty value.
  - `--stealth` - Set whether future browser sessions run in stealth mode; use `--stealth=false` to disable
  - `--telemetry=all` / `--telemetry=off` / `--telemetry=<categories>` - Update telemetry for future browser sessions
  - `--telemetry-export-otlp <id-or-name>` - Update where future sessions export captured telemetry. Naming a destination requires passing `--telemetry` in the same command, since the API validates capture and export together and enabling capture here would replace the connection's current category selection. Use `=off` to disable export.
- `kernel auth connections login <id>` - New flags:
  - `--proxy-id <id>` / `--proxy-name <name>` / `--proxy-mode direct|default` - Proxy override for this login's browser session (mutually exclusive); omitted properties inherit the connection defaults
  - `--stealth` - Stealth override for this login's browser session; use `--stealth=false` to disable
  - `--telemetry=all` / `--telemetry=off` / `--telemetry=<categories>` - Telemetry override for this login only, merged onto the connection's config
  - `--telemetry-export-otlp <id-or-name>` - Export override for this login only. Naming a destination requires passing `--telemetry` in the same command. Use `=off` to disable export.
- `kernel auth connections submit <id>` - New flags:
  - `--field-value <id=value>` - Canonical field-id=value pair from the connection's `fields` list (repeatable); preferred over the legacy `--field`
  - `--choice-id <id>` - Canonical choice ID from the connection's `choices` list

`kernel auth connections get` and `follow` list those IDs alongside the metadata the API captured for them, so you can tell the options apart before submitting. Fields show their type, ref, and any hint (which names the masked destination a one-time code was sent to); choices show their type, semantic MFA method (`sms`, `totp`, `push`, …), and masked destination.

### Agent Auth

Automated authentication for web services. The `run` command orchestrates the full auth flow automatically.

- `kernel agents auth run` - Run a complete authentication flow
  - `--domain <domain>` - Target domain for authentication (required)
  - `--profile <name>` - Profile name to use/create (required)
  - `--value <key=value>` - Field name=value pair (repeatable, e.g., `--value username=foo --value password=bar`)
  - `--credential <name>` - Existing credential name to use
  - `--save-credential-as <name>` - Save provided credentials under this name
  - `--totp-secret <secret>` - Base32 TOTP secret for automatic 2FA
  - `--proxy-id <id>` - Proxy ID to use
  - `--login-url <url>` - Custom login page URL
  - `--allowed-domain <domain>` - Additional allowed domains (repeatable)
  - `--timeout <duration>` - Maximum time to wait for auth completion (default: 5m)
  - `--open` - Open live view URL in browser when human intervention needed
  - `--output json`, `-o json` - Output JSONL events

- `kernel agents auth create` - Create an auth agent
  - `--domain <domain>` - Target domain for authentication (required)
  - `--profile-name <name>` - Name of the profile to use (required)
  - `--credential-name <name>` - Optional credential name to link
  - `--login-url <url>` - Optional login page URL
  - `--allowed-domain <domain>` - Additional allowed domains (repeatable)
  - `--proxy-id <id>` - Optional proxy ID to use
  - `--output json`, `-o json` - Output raw JSON object

- `kernel agents auth list` - List auth agents
  - `--domain <domain>` - Filter by domain
  - `--profile-name <name>` - Filter by profile name
  - `--limit <n>` - Maximum number of results to return
  - `--offset <n>` - Number of results to skip
  - `--output json`, `-o json` - Output raw JSON array

- `kernel agents auth get <id>` - Get an auth agent by ID
  - `--output json`, `-o json` - Output raw JSON object

- `kernel agents auth delete <id>` - Delete an auth agent
  - `-y, --yes` - Skip confirmation prompt

### Credentials

- `kernel credentials create` - Create a new credential
  - `--name <name>` - Unique name for the credential (required)
  - `--domain <domain>` - Target domain (required)
  - `--value <key=value>` - Field name=value pair (repeatable)
  - `--sso-provider <provider>` - SSO provider (google, github, microsoft)
  - `--totp-secret <secret>` - Base32-encoded TOTP secret for 2FA
  - `--output json`, `-o json` - Output raw JSON object

- `kernel credentials list` - List credentials
  - `--domain <domain>` - Filter by domain
  - `--output json`, `-o json` - Output raw JSON array

- `kernel credentials get <id-or-name>` - Get a credential by ID or name
  - `--output json`, `-o json` - Output raw JSON object

- `kernel credentials update <id-or-name>` - Update a credential
  - `--name <name>` - New name
  - `--value <key=value>` - Field values to update (repeatable)
  - `--sso-provider <provider>` - SSO provider
  - `--totp-secret <secret>` - TOTP secret
  - `--output json`, `-o json` - Output raw JSON object

- `kernel credentials delete <id-or-name>` - Delete a credential
  - `-y, --yes` - Skip confirmation prompt

- `kernel credentials totp-code <id-or-name>` - Get current TOTP code
  - `--output json`, `-o json` - Output raw JSON object

### API Keys

- `kernel api-keys create` - Create a new API key
  - `--name <name>` - API key name (required)
  - `--days-to-expire <days>` - Number of days until expiry (1-3650); omit for never
  - `--project-id <project_id>` - Create a project-scoped API key for this project ID; omit for org-wide. This is different from global `--project`, which scopes the CLI request to a project ID.
  - `--output json`, `-o json` - Output raw JSON object, including the one-time plaintext key

- `kernel api-keys list` - List API keys
  - `--limit <n>` - Maximum number of results to return
  - `--offset <n>` - Number of results to skip
  - `--output json`, `-o json` - Output raw JSON array

- `kernel api-keys get <id>` - Get an API key
  - `--include-deleted` - Include soft-deleted API keys in the lookup
  - `--output json`, `-o json` - Output raw JSON object

- `kernel api-keys update <id>` - Update an API key
  - `--name <name>` - New API key name
  - `--output json`, `-o json` - Output raw JSON object

- `kernel api-keys rotate <id>` - Issue a replacement API key; the rotated key keeps working for a grace period (7 days by default) so callers can migrate
  - `--days-to-expire <days>` - Lifetime in days for the new key (1-3650); omit to reuse the rotated key's lifetime
  - `--expire-in-days <days>` - Grace period in days before the rotated key expires; `0` expires it immediately, omit for the default 7 days
  - `-y, --yes` - Skip confirmation prompt
  - `--output json`, `-o json` - Output raw JSON object, including the one-time plaintext key

- `kernel api-keys delete <id>` - Delete an API key
  - `-y, --yes` - Skip confirmation prompt

### Org

- `kernel org entitlements` - Show the organization's effective plan, feature access, and limits
  - `--output json`, `-o json` - Output the raw entitlement response
- `kernel org limits get` - Show the organization's concurrency limit and the default per-project cap applied to projects without an explicit override
  - `--output json`, `-o json` - Output raw JSON object
- `kernel org limits set` - Set the default per-project concurrency cap applied to projects without an explicit override
  - `--default-project-max-concurrent-sessions <n>` - Default maximum concurrent browsers for projects without an explicit override (`0` to remove the default)
  - `--output json`, `-o json` - Output raw JSON object

## Examples

### Create a new app

```bash
# Interactive mode (prompts for all options)
kernel create

# Create a TypeScript app with sample template
kernel create --name my-app --language typescript --template sample-app

# Create a Python app with Browser Use
kernel create --name my-scraper --language python --template browser-use

# Create a TypeScript app with Stagehand
kernel create --name my-agent --language ts --template stagehand

# Create a Python Computer Use app
kernel create --name my-cu-app --language py --template anthropic-computer-use

# Create a Claude Agent SDK app (TypeScript or Python)
kernel create --name my-claude-agent --language ts --template claude-agent-sdk
```

### Deploy with environment variables

```bash
# Set individual variables
kernel deploy index.ts --env API_KEY=abc123 --env DEBUG=true

# Load from .env file
kernel deploy index.ts --env-file .env

# Combine both methods
kernel deploy index.ts --env-file .env --env OVERRIDE_VAR=value
```

### Invoke with payload

```bash
# Simple invoke
kernel invoke my-scraper scrape-page

# With JSON payload
kernel invoke my-scraper scrape-page --payload '{"url": "https://example.com"}'

# Read payload from a file
kernel invoke my-scraper scrape-page --payload-file payload.json

# Read payload from stdin
cat payload.json | kernel invoke my-scraper scrape-page --payload-file -

# Pipe from another command
echo '{"url": "https://example.com"}' | kernel invoke my-scraper scrape-page -f -

# Synchronous invoke (wait for completion)
kernel invoke my-scraper quick-task --sync
```

### Follow logs in real-time

```bash
# Follow logs
kernel logs my-app --follow

# Show recent logs with timestamps
kernel logs my-app --since 1h --with-timestamps
```

### Browser management

```bash
# List all browsers
kernel browsers list

# Create a new browser session
kernel browsers create

# Create a browser with a longer timeout (up to 72 hours)
kernel browsers create --timeout 3600

# Create a headless browser in stealth mode
kernel browsers create --headless --stealth

# Create a browser in kiosk mode
kernel browsers create --kiosk

# Create a browser with a profile for session state
kernel browsers create --profile-name my-profile

# Create a browser with a custom Chrome enterprise policy
kernel browsers create --chrome-policy '{"BookmarkBarEnabled": false}'
kernel browsers create --chrome-policy-file policy.json

# Delete a browser
kernel browsers delete browser123

# Get live view URL
kernel browsers view browser123

# Make an HTTP request through the browser session
kernel browsers curl browser123 https://example.com

# Include response headers and save the response to a file
kernel browsers curl browser123 -i -o page.html https://example.com

# Send JSON and print curl-style status metrics
kernel browsers curl browser123 https://api.example.com \
  -H "Content-Type: application/json" \
  -d '{"key":"value"}' \
  -w 'status=%{http_code} bytes=%{size_download}\n'

# Fail on HTTP errors without printing the response body
kernel browsers curl browser123 -f https://example.com/missing

# Stream browser logs
kernel browsers logs stream my-browser --source supervisor --follow --supervisor-process chromium

# Start a replay recording
kernel browsers replays start my-browser --framerate 30 --max-duration 300

# Execute a command in the browser VM
kernel browsers process exec my-browser -- ls -alh /tmp

# Upload files to the browser VM
kernel browsers fs upload my-browser --file "local.txt:remote.txt" --dest-dir "/tmp"

# List files in a directory
kernel browsers fs list-files my-browser --path "/tmp"

# Click the mouse at coordinates (100, 200)
kernel browsers computer click-mouse my-browser --x 100 --y 200

# Double-click the right mouse button
kernel browsers computer click-mouse my-browser --x 100 --y 200 --num-clicks 2 --button right

# Move the mouse to coordinates (500, 300)
kernel browsers computer move-mouse my-browser --x 500 --y 300

# Take a full screenshot
kernel browsers computer screenshot my-browser --to screenshot.png

# Take a screenshot of a specific region
kernel browsers computer screenshot my-browser --to region.png --x 0 --y 0 --width 800 --height 600

# Type text in the browser
kernel browsers computer type my-browser --text "Hello, World!"

# Type text with a 100ms delay between keystrokes
kernel browsers computer type my-browser --text "Slow typing..." --delay 100

```

### Playwright execution

```bash
# Execute inline Playwright (TypeScript) code
kernel browsers playwright execute my-browser 'await page.goto("https://example.com"); const title = await page.title(); return title;'

# Or pipe code from stdin
cat <<'TS' | kernel browsers playwright execute my-browser
await page.goto("https://example.com");
const title = await page.title();
return { title };
TS

# With a timeout in seconds
kernel browsers playwright execute my-browser --timeout 30 'await (await context.newPage()).goto("https://example.com")'

# Mini CDP connection load test (10s)
cat <<'TS' | kernel browsers playwright execute my-browser
const start = Date.now();
let ops = 0;
while (Date.now() - start < 10_000) {
  await page.evaluate("new Date();");
  ops++;
}
const durationMs = Date.now() - start;
const opsPerSec = ops / (durationMs / 1000);
return { opsPerSec, ops, durationMs };
TS
```

### Extension management

```bash
# List all uploaded extensions
kernel extensions list

# Upload an unpacked extension directory
kernel extensions upload ./my-extension --name my-custom-extension

# Download an extension from Chrome Web Store
kernel extensions download-web-store "https://chrome.google.com/webstore/detail/extension-id" --to ./downloaded-extension

# Download a previously uploaded extension
kernel extensions download my-extension-id --to ./my-extension

# Delete an extension
kernel extensions delete my-extension-name --yes

# Upload extensions to a running browser instance
kernel browsers extensions upload my-browser ./extension1 ./extension2
```

### Proxy management

```bash
# List proxy configurations
kernel proxies list

# Create a datacenter proxy
kernel proxies create --type datacenter --country US --name "US Datacenter"

# Create a datacenter proxy using HTTP protocol
kernel proxies create --type datacenter --country US --protocol http --name "US DC (HTTP)"

# Create a custom proxy
kernel proxies create --type custom --host proxy.example.com --port 8080 --username myuser --password mypass --name "My Custom Proxy"

# Create a custom TLS-terminating proxy with a CA bundle
kernel proxies create --type custom --host proxy.example.com --port 8080 --ca-bundle ./proxy-ca.pem --name "My TLS Proxy"

# Create a residential proxy with location and OS
kernel proxies create --type residential --country US --city sanfrancisco --state CA --zip 94107 --asn AS15169 --os windows --name "SF Residential"

# Create a mobile proxy
kernel proxies create --type mobile --country US --city sanfrancisco --name "US Mobile"

# Get proxy details
kernel proxies get prx_123

# Delete a proxy (skip confirmation)
kernel proxies delete prx_123 --yes
```

### Agent auth

```bash
# Run a complete auth flow with inline credentials
kernel agents auth run --domain github.com --profile my-github \
  --value username=myuser --value password=mypass

# Auth with TOTP for automatic 2FA handling
kernel agents auth run --domain github.com --profile my-github \
  --value username=myuser --value password=mypass \
  --totp-secret JBSWY3DPEHPK3PXP

# Save credentials for future re-auth
kernel agents auth run --domain github.com --profile my-github \
  --value username=myuser --value password=mypass \
  --save-credential-as github-creds

# Re-use existing saved credential
kernel agents auth run --domain github.com --profile my-github \
  --credential github-creds

# Auto-open browser when human intervention is needed
kernel agents auth run --domain github.com --profile my-github \
  --credential github-creds --open

# Use the authenticated profile with a browser
kernel browsers create --profile-name my-github
```

## Getting Help

- `kernel --help` - Show all available commands
- `kernel <command> --help` - Get help for a specific command

## Documentation

For complete documentation, visit:

- [📖 Documentation](https://www.kernel.sh/docs)
- [🚀 Quickstart Guide](https://www.kernel.sh/docs/quickstart)
- [📋 CLI Reference](https://www.kernel.sh/docs/reference/cli)

## Support

- [Discord Community](https://discord.gg/kernel)
- [GitHub Issues](https://github.com/onkernel/kernel/issues)
- [Documentation](https://www.kernel.sh/docs)

---

For development and contribution information, see [DEVELOPMENT.md](./DEVELOPMENT.md).
