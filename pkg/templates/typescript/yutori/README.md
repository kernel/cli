# Kernel TypeScript Sample App - Yutori n1 Computer Use

This is a Kernel application that implements a prompt loop using Yutori's n1 computer use model with Kernel's Computer Controls API.

[n1](https://yutori.com/blog/introducing-navigator) is Yutori's pixels-to-actions LLM that predicts browser actions from screenshots.

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Yutori**: [yutori.com](https://yutori.com)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your YUTORI_API_KEY
kernel deploy index.ts --env-file .env
```

## Usage

```bash
kernel invoke ts-yutori-cua cua-task --payload '{"query": "Go to https://www.magnitasks.com, Click the Tasks option in the left-side bar, and drag the 5 items in the To Do and In Progress columns to the Done section of the Kanban board. You are done successfully when the items are dragged to Done. Do not click into the items."}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke ts-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Kiosk mode

Prefer **non-kiosk mode** by default and when the agent is expected to switch domains via URL. Use **kiosk (`"kiosk": true`)** when: (1) you're recording sessions and want a cleaner UI in the replay, or (2) you're automating on a single website and the combination of the complex site layout and browser chrome (address bar, tabs) may confuse the agent. 

Note: In kiosk mode the agent may still try to use the address bar to enter URLs; it's not available, so it will eventually use `goto_url`, but those attempts may result in slowdown of the overall session.

Default (non-kiosk):

```bash
kernel invoke ts-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com, then navigate to ign.com and describe the page"}'
```

With kiosk (single-site or recording):

```bash
kernel invoke ts-yutori-cua cua-task --payload '{"query": "Enter https://example.com in the search box and then describe the page.", "kiosk": true}'
```

## Viewport Configuration

Yutori n1 recommends a **1280×800 (WXGA, 16:10)** viewport for best grounding accuracy.

> **Note:** n1 outputs coordinates in a 1000×1000 relative space, which are automatically scaled to the actual viewport dimensions.

See [Kernel Viewport Documentation](https://www.kernel.sh/docs/browsers/viewport) for all supported configurations.

## Screenshots

Screenshots are automatically converted to WebP format for better compression across multi-step trajectories, as recommended by Yutori.

## n1-latest Supported Actions

| Action | Description |
|--------|-------------|
| `left_click` | Left mouse click at coordinates |
| `double_click` | Double-click at coordinates |
| `triple_click` | Triple-click at coordinates |
| `right_click` | Right mouse click at coordinates |
| `scroll` | Scroll page in a direction |
| `type` | Type text into focused element |
| `key_press` | Send keyboard input |
| `hover` | Move mouse without clicking |
| `drag` | Click-and-drag operation |
| `wait` | Pause for UI to update |
| `refresh` | Reload current page |
| `go_back` | Navigate back in history |
| `goto_url` | Navigate to a URL |

## Resources

- [Yutori n1 API Documentation](https://docs.yutori.com/reference/n1)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
