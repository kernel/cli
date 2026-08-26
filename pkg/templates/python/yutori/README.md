# Kernel Python Sample App - Yutori n2 Computer Use

This Kernel app implements a prompt loop using Yutori's Navigator n2 with Kernel's Computer Controls API.

[Navigator n2](https://docs.yutori.com/reference/n2) is Yutori's computer-use model. It reads a screenshot of the whole screen and answers with a batch of mouse and keyboard actions, and it can run shell commands and edit files on the machine it is driving.

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
- `reasoning_effort` (`"none"`, `"low"`, `"medium"`, `"xhigh"`) — n2 reasons at `medium` by default. `xhigh` gives the longest traces and does best on hard multi-step tasks; `none` turns reasoning off.
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

## Screen Configuration

n2 is a desktop model: it expects a screenshot of the **whole screen**, browser chrome included, and it navigates by clicking the address bar rather than through a dedicated navigation action. A Kernel `viewport` sets the browser window size, and screenshots from Computer Controls capture that window with its tabs and toolbar — which is exactly what n2 wants, so this template does not use kiosk mode.

This template runs at **1280x800**. Yutori also lists 1920x1080 and 1280x720 as resolutions in regular use; grounding may degrade at extreme aspect ratios.

> **Note:** n2 outputs coordinates in a 1000x1000 relative space, which are scaled to the actual screen dimensions per action.

See [Kernel Viewport Documentation](https://www.kernel.sh/docs/browsers/viewport) for all supported configurations.

## Screenshots

Screenshots are converted to WebP before they are sent. Yutori caps requests at 10 MB, and a full-screen PNG trajectory blows past that on its own.

The loop also drops screenshots the model will not read: n2 keeps images from only the last 2 image-bearing messages, so older ones are stripped from each request while every message's text is kept.

## Tools

This template pins the `computer_use_tools-20260825` tool set. n2 rejects `disable_tools`, so all five tools are always served and the loop answers all of them.

### `computer_batch`

n2 returns a whole action list in one call. The loop runs them in order, stops at the first error, and replies with a single tool result carrying one screenshot taken after the last action that ran. Every coordinate in a batch refers to the screenshot from *before* the batch started.

| Action | Description |
|--------|-------------|
| `left_click` | Left mouse click at coordinates (supports `modifier`) |
| `double_click` | Double-click at coordinates (supports `modifier`) |
| `triple_click` | Triple-click at coordinates (supports `modifier`) |
| `middle_click` | Middle mouse click at coordinates (supports `modifier`) |
| `right_click` | Right mouse click at coordinates (supports `modifier`) |
| `scroll` | Scroll up or down at coordinates, in wheel notches |
| `type` | Type text into the focused element |
| `key_press` | Press a key, combination, or sequence |
| `drag` | Drag from `start_coordinates` to `coordinates` |
| `mouse_move` | Move the mouse to coordinates without clicking |
| `mouse_down` | Press and hold the left button (coordinates optional) |
| `mouse_up` | Release the left button (coordinates optional) |
| `hold_key` | Hold a key down for a duration |
| `wait` | Pause without interacting |
| `screenshot` | Look without acting |

Horizontal scrolling is not available, and there are no `goto_url` / `go_back` / `go_forward` / `refresh` actions — n2 drives the address bar and browser buttons like a person would.

### `bash`, `read`, `write`, `edit`

n2 can also work on the browser VM directly. These map onto Kernel's process and filesystem APIs:

| Tool | Backed by |
|------|-----------|
| `bash` | `browsers.process.exec` — each call is a separate process; the working directory carries over between calls, environment variables do not. `run_in_background` detaches the command and reports its pid and log path. |
| `read` | `browsers.fs.read_file` — returns `cat -n` output, with `offset` / `limit` paging |
| `write` | `browsers.fs.write_file` |
| `edit` | read, exact-string replace, write. Refuses to edit a file that has not been read in this session. |

## Resources

- [Yutori n2 API Documentation](https://docs.yutori.com/reference/n2)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
