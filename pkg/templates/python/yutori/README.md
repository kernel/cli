# Kernel Python Sample App - Yutori n1.5 Computer Use

This Kernel app implements a prompt loop using Yutori's Navigator n1.5 with Kernel's Computer Controls API.

[Navigator n1.5](https://yutori.com/blog/introducing-n1-5) is Yutori's pixels-to-actions LLM that predicts browser actions from screenshots.

This template runs n1.5 in **computer-use-only mode**. n1.5 also supports a hybrid vision + DOM/JavaScript path (page-state extraction, custom JS, structured JSON output) for multi-field forms and bulk data extraction, but those tools are intentionally disabled here — see [Disabled tools](#disabled-tools).

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Yutori**: [yutori.com](https://yutori.com)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your YUTORI_API_KEY
kernel deploy main.py --env-file .env
```

## Usage

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Navigate to https://www.yutori.com and list the team member names."}'
```

Optional payload fields:

- `record_replay` (bool) — capture a video of the session (paid plans only).
- `kiosk` (bool) — launch the browser without address bar / tabs ([see below](#kiosk-mode)).
- `user_timezone` (IANA, e.g. `"America/New_York"`) and `user_location` (free text, e.g. `"New York, NY, US"`) — appended to the task message so the model has accurate temporal/locational grounding.

More involved example (Kanban drag-and-drop):

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Go to https://www.magnitasks.com, Click the Tasks option in the left-side bar, and drag the 5 items in the To Do and In Progress columns to the Done section of the Kanban board. You are done successfully when the items are dragged to Done. Do not click into the items."}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Kiosk mode

Prefer **non-kiosk mode** by default and when the agent is expected to switch domains via URL. Use **kiosk (`"kiosk": true`)** when: (1) you're recording sessions and want a cleaner UI in the replay, or (2) you're automating on a single website and the combination of the complex site layout and browser chrome (address bar, tabs) may confuse the agent. 

Note: In kiosk mode the agent may still try to use the address bar to enter URLs; it's not available, so it will eventually use `goto_url`, but those attempts may result in slowdown of the overall session.

Default (non-kiosk):

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com, then navigate to ign.com and describe the page"}'
```

With kiosk (single-site or recording):

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Enter https://example.com in the search box and then describe the page.", "kiosk": true}'
```

## Viewport Configuration

Yutori n1.5 recommends a **1280×800 (WXGA, 16:10)** viewport for best grounding accuracy.

> **Note:** n1.5 outputs coordinates in a 1000×1000 relative space, which are automatically scaled to the actual viewport dimensions.

See [Kernel Viewport Documentation](https://www.kernel.sh/docs/browsers/viewport) for all supported configurations.

## Screenshots

Screenshots are automatically converted to WebP format for better compression across multi-step trajectories, as recommended by Yutori.

## n1.5-latest Supported Actions

This template uses the `browser_tools_core-20260403` tool set — coordinate-based browser actions that operate on screenshots only.

| Action | Description |
|--------|-------------|
| `left_click` | Left mouse click at coordinates (supports `modifier`) |
| `double_click` | Double-click at coordinates (supports `modifier`) |
| `triple_click` | Triple-click at coordinates (supports `modifier`) |
| `middle_click` | Middle mouse click at coordinates |
| `right_click` | Right mouse click at coordinates |
| `mouse_move` | Move mouse to coordinates without clicking |
| `mouse_down` | Press the left mouse button at coordinates |
| `mouse_up` | Release the left mouse button at coordinates |
| `scroll` | Scroll page in a direction |
| `type` | Type text into focused element |
| `key_press` | Send a single key or key combination |
| `hold_key` | Hold a key for a duration |
| `drag` | Click-and-drag operation |
| `wait` | Pause for UI to update |
| `refresh` | Reload current page |
| `go_back` | Navigate back in history |
| `go_forward` | Navigate forward in history |
| `goto_url` | Navigate to a URL |

### Disabled tools

The DOM/Playwright-based "expanded" tools (`extract_elements`, `find`, `set_element_value`, `execute_js`) are intentionally disabled via the `disable_tools` request parameter — this template runs computer-use only and does not expose a Playwright page to the model.

## Resources

- [Yutori n1.5 API Documentation](https://docs.yutori.com/reference/n1-5)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
