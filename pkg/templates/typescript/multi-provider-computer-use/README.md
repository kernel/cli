# Multi-Provider Computer Use Template

A unified Kernel template that supports multiple AI providers for computer use tasks. Select the provider at runtime via the action payload.

## Supported Providers

- **Anthropic** - Uses Claude for computer use tasks
- **Gemini** - Uses Google's Gemini for computer use tasks

## Setup

1. Copy `.env.example` to `.env`:
   ```bash
   cp .env.example .env
   ```

2. Add your API key(s) to `.env`:
   - For Anthropic: Set `ANTHROPIC_API_KEY`
   - For Gemini: Set `GOOGLE_API_KEY`

3. Deploy to Kernel:
   ```bash
   kernel deploy index.ts --env-file .env
   ```

## Usage

Invoke the `cua-task` action with the required `provider` and `query` fields:

### Anthropic Provider

```bash
kernel invoke ts-multi-provider-cua cua-task --payload '{
  "provider": "anthropic",
  "query": "Navigate to example.com and describe what you see"
}'
```

### Gemini Provider

```bash
kernel invoke ts-multi-provider-cua cua-task --payload '{
  "provider": "gemini",
  "query": "Navigate to example.com and describe what you see"
}'
```

### With Replay Recording

```bash
kernel invoke ts-multi-provider-cua cua-task --payload '{
  "provider": "anthropic",
  "query": "Navigate to example.com",
  "record_replay": true
}'
```

## Payload Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | Yes | The task for the computer use agent |
| `provider` | string | Yes | The AI provider: `"anthropic"` or `"gemini"` |
| `record_replay` | boolean | No | Record a video replay of the session |

## Response

```json
{
  "result": "The task result from the AI agent",
  "replay_url": "https://...",  // If record_replay was true
  "error": "..."  // If something went wrong
}
```

## Provider-Specific Notes

### Anthropic

- Uses Claude Sonnet 4.5 (`claude-sonnet-4-5-20250929`)
- Default viewport: 1024x768

### Gemini

- Uses Gemini 2.5 Computer Use Preview (`gemini-2.5-computer-use-preview-10-2025`)
- Default viewport: 1200x800
- Uses normalized coordinates (0-1000 scale internally)

## Known Limitations

### URL Reporting (Gemini)

The Gemini Computer Use API requires a URL in all function responses. However, the Kernel Computer Controls API doesn't provide a method to retrieve the current page URL.

As a workaround, this template reports `about:blank` as the URL in all responses. This works because Gemini primarily uses the screenshot to understand page state - the URL is a required field but not critical for functionality.
