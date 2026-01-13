<p align="center">
  <img src="https://raw.githubusercontent.com/onkernel/kernel-images/main/static/images/Kernel-Wordmark_Accent.svg" alt="Kernel Logo" width="55%">
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

This opens your browser to complete the authentication flow. Your credentials are securely stored and automatically refreshed.

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
- **Profiles**: `create`, `list`, `get`
- **Extensions**: `upload`, `list`
- **Proxies**: `create`, `list`, `get`
- **Apps**: `list`, `history`
- **Deploy**: `deploy` (JSONL streaming), `history`
- **Invoke**: `invoke` (JSONL streaming), `history`
- **Browser Sub-commands**: `replays list/start`, `process exec/spawn`, `fs file-info/list-files`

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
  - `magnitude` - Magnitude framework sample (TypeScript only)
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
  - `--output json`, `-o json` - Output raw JSON array
- `kernel browsers create` - Create a new browser session
  - `-s, --stealth` - Launch browser in stealth mode to avoid detection
  - `-H, --headless` - Launch browser without GUI access
  - `--kiosk` - Launch browser in kiosk mode
  - `--pool-id <id>` - Acquire a browser from the specified pool (mutually exclusive with --pool-name; ignores other session flags)
  - `--pool-name <name>` - Acquire a browser from the pool name (mutually exclusive with --pool-id; ignores other session flags)
  - `--output json`, `-o json` - Output raw JSON object
  - _Note: When a pool is specified, omit other session configuration flags—pool settings determine profile, proxy, viewport, etc._
- `kernel browsers delete <id>` - Delete a browser
  - `-y, --yes` - Skip confirmation prompt
- `kernel browsers view <id>` - Get live view URL for a browser
  - `--output json`, `-o json` - Output JSON with liveViewUrl
- `kernel browsers get <id>` - Get detailed browser session info
  - `--output json`, `-o json` - Output raw JSON object

### Browser Pools

- `kernel browser-pools list` - List browser pools
  - `--output json`, `-o json` - Output raw JSON array
- `kernel browser-pools create` - Create a browser pool
  - `--name <name>` - Optional unique name for the pool
  - `--size <n>` - Number of browsers in the pool (required)
  - `--fill-rate <n>` - Percentage of the pool to fill per minute
  - `--timeout <seconds>` - Idle timeout for browsers acquired from the pool
  - `--stealth`, `--headless`, `--kiosk` - Default pool configuration
  - `--profile-id`, `--profile-name`, `--save-changes`, `--proxy-id`, `--extension`, `--viewport` - Same semantics as `kernel browsers create`
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools get <id-or-name>` - Get pool details
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools update <id-or-name>` - Update pool configuration
  - Same flags as create plus `--discard-all-idle` to discard all idle browsers in the pool and refill at the specified fill rate
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browser-pools delete <id-or-name>` - Delete a pool
  - `--force` - Force delete even if browsers are leased
- `kernel browser-pools acquire <id-or-name>` - Acquire a browser from the pool
  - `--timeout <seconds>` - Acquire timeout before returning 204
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

### Browser Process Control

- `kernel browsers process exec <id> [--] [command...]` - Execute a command synchronously
  - `--command <cmd>` - Command to execute (optional; if omitted, trailing args are executed via /bin/bash -c)
  - `--args <args>` - Command arguments
  - `--cwd <path>` - Working directory
  - `--timeout <seconds>` - Timeout in seconds
  - `--as-user <user>` - Run as user
  - `--as-root` - Run as root
  - `--output json`, `-o json` - Output raw JSON object
- `kernel browsers process spawn <id> [--] [command...]` - Execute a command asynchronously
  - `--command <cmd>` - Command to execute (optional; if omitted, trailing args are executed via /bin/bash -c)
  - `--args <args>` - Command arguments
  - `--cwd <path>` - Working directory
  - `--timeout <seconds>` - Timeout in seconds
  - `--as-user <user>` - Run as user
  - `--as-root` - Run as root
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

  - `--key <key>` - Key symbols to press (repeatable)
  - `--duration <ms>` - Duration to hold keys down in ms (0=tap)
  - `--hold-key <key>` - Modifier keys to hold (repeatable)

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

### Extension Management

- `kernel extensions list` - List all uploaded extensions
  - `--output json`, `-o json` - Output raw JSON array
- `kernel extensions upload <directory>` - Upload an unpacked browser extension directory
  - `--name <name>` - Optional unique extension name
  - `--output json`, `-o json` - Output raw JSON object
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

  - `--name <name>` - Proxy configuration name
  - `--type <type>` - Proxy type: datacenter, isp, residential, mobile, custom (required)
  - `--protocol <http|https>` - Protocol to use (default: https)
  - `--country <code>` - ISO 3166 country code or "EU" (location-based types)
  - `--city <name>` - City name (no spaces, e.g. sanfrancisco) (residential, mobile; requires `--country`)
  - `--state <code>` - Two-letter state code (residential, mobile)
  - `--zip <zip>` - US ZIP code (residential, mobile)
  - `--asn <asn>` - Autonomous system number (e.g., AS15169) (residential, mobile)
  - `--os <os>` - Operating system: windows, macos, android (residential)
  - `--carrier <carrier>` - Mobile carrier (mobile)
  - `--host <host>` - Proxy host (custom; required)
  - `--port <port>` - Proxy port (custom; required)
  - `--username <username>` - Username for proxy authentication (custom)
  - `--password <password>` - Password for proxy authentication (custom)

- `kernel proxies delete <id>` - Delete a proxy configuration
  - `-y, --yes` - Skip confirmation prompt

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

# Delete a browser
kernel browsers delete browser123 --yes

# Get live view URL
kernel browsers view browser123

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

# Create a residential proxy with location and OS
kernel proxies create --type residential --country US --city sanfrancisco --state CA --zip 94107 --asn AS15169 --os windows --name "SF Residential"

# Create a mobile proxy with carrier
kernel proxies create --type mobile --country US --carrier verizon --name "US Mobile"

# Get proxy details
kernel proxies get prx_123

# Delete a proxy (skip confirmation)
kernel proxies delete prx_123 --yes
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
