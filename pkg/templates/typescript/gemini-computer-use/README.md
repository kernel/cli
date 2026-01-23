# Kernel TypeScript Sample App - Gemini Computer Use

This is a Kernel application that implements a prompt loop using Google's Gemini Computer Use model with Kernel's Computer Controls API.

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Google AI**: [aistudio.google.com/api-keys](https://aistudio.google.com/api-keys)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your GOOGLE_API_KEY
kernel deploy index.ts --env-file .env
```

## Usage

```bash
kernel invoke ts-gemini-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke ts-gemini-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Gemini Computer Use Actions

The Gemini model can execute the following browser actions:

| Action | Description |
|--------|-------------|
| `click_at` | Click at coordinates (x, y) |
| `hover_at` | Move mouse to coordinates (x, y) |
| `type_text_at` | Click and type text at coordinates |
| `scroll_document` | Scroll the page (up/down/left/right) |
| `scroll_at` | Scroll at specific coordinates |
| `navigate` | Navigate to a URL |
| `go_back` | Go back in browser history |
| `go_forward` | Go forward in browser history |
| `key_combination` | Press key combination (e.g., "ctrl+c") |
| `drag_and_drop` | Drag from one point to another |
| `wait_5_seconds` | Wait for 5 seconds |

## Resources

- [Google Gemini Computer Use Documentation](https://ai.google.dev/gemini-api/docs/computer-use)
- [Kernel Computer Controls](https://www.kernel.sh/docs/browsers/computer-controls)
