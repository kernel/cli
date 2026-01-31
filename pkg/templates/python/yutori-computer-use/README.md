# Kernel Python Sample App - Yutori n1 Computer Use

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
kernel deploy main.py --env-file .env
```

## Usage

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke python-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Viewport Configuration

Yutori n1 recommends a **1280×800 (WXGA, 16:10)** viewport for best grounding accuracy. Kernel's closest supported viewport is **1200×800 at 25Hz**, which this template uses by default.

> **Note:** n1 outputs coordinates in a 1000×1000 relative space, which are automatically scaled to the actual viewport dimensions. The slight width difference (1200 vs 1280) should have minimal impact on accuracy.

See [Kernel Viewport Documentation](https://www.kernel.sh/docs/browsers/viewport) for all supported configurations.

## n1 Supported Actions

| Action | Description |
|--------|-------------|
| `click` | Left mouse click at coordinates |
| `scroll` | Scroll page in a direction |
| `type` | Type text into focused element |
| `key_press` | Send keyboard input |
| `hover` | Move mouse without clicking |
| `drag` | Click-and-drag operation |
| `wait` | Pause for UI to update |
| `refresh` | Reload current page |
| `go_back` | Navigate back in history |
| `goto_url` | Navigate to a URL |
| `stop` | End task with final answer |

## Resources

- [Yutori n1 API Documentation](https://docs.yutori.com/reference/n1)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
