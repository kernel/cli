# Kernel TypeScript Sample App - Moondream + Groq Computer Use

This Kernel app runs a lightweight computer-use agent powered by Moondream vision models, Groq fast LLM orchestration.

## Setup

1. Get your API keys:
   - **Moondream**: [moondream.ai](https://moondream.ai)
   - **Groq**: [console.groq.com](https://console.groq.com)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your MOONDREAM_API_KEY and GROQ_API_KEY
kernel deploy index.ts --env-file .env
```

## Usage

Natural-language query (Groq LLM orchestrates Moondream + Kernel):
```bash
kernel invoke ts-moondream-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'
```

Structured steps (optional fallback for deterministic automation):
```bash
kernel invoke ts-moondream-cua cua-task --payload '{
  "steps": [
    {"action": "navigate", "url": "https://example.com"},
    {"action": "caption"},
    {"action": "click", "target": "More information link", "retries": 4},
    {"action": "type", "target": "Search input", "text": "kernel", "press_enter": true}
  ]
}'
```

## Step Actions

Each step is a JSON object with an `action` field. Supported actions:

- `navigate`: `{ "url": "https://..." }`
- `click`: `{ "target": "Button label or description" }`
- `type`: `{ "target": "Input field description", "text": "...", "press_enter": false }`
- `scroll`: `{ "direction": "down" }` or `{ "x": 0.5, "y": 0.5, "direction": "down" }`
- `query`: `{ "question": "Is there a login button?" }`
- `caption`: `{ "length": "short" | "normal" | "long" }`
- `wait`: `{ "seconds": 2.5 }`
- `key`: `{ "keys": "ctrl+l" }`
- `go_back`, `go_forward`, `search`, `open_web_browser`

Optional step fields:
- `retries`: override retry attempts for point/click/type
- `retry_delay_ms`: wait between retries
- `x`, `y`: normalized (0-1) or pixel coordinates to bypass Moondream pointing (pixel coords use detected screenshot size)

## Replay Recording

Add `"record_replay": true` to the payload to capture a video replay.

## Notes

- The agent uses Moondream for visual reasoning and pointing.
- Kernel screenshots are PNG; Moondream queries are sent as base64 data URLs.
- The Groq LLM orchestrates JSON actions; the agent repairs and parses JSON with jsonrepair.
