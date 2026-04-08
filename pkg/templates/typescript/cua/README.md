# Kernel Unified CUA Template

A unified Computer Use Agent (CUA) template that supports multiple providers with automatic fallback.

## Supported Providers

| Provider | Model | Env Var |
|----------|-------|---------|
| Anthropic | `claude-sonnet-4-6` | `ANTHROPIC_API_KEY` |
| OpenAI | `gpt-5.4` | `OPENAI_API_KEY` |
| Gemini | `gemini-2.5-computer-use-preview-10-2025` | `GOOGLE_API_KEY` |

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Anthropic**: [console.anthropic.com](https://console.anthropic.com)
   - **OpenAI**: [platform.openai.com](https://platform.openai.com)
   - **Google**: [aistudio.google.com](https://aistudio.google.com)

2. Configure and deploy:
```bash
kernel login
cp .env.example .env  # Add your API keys and configure providers
kernel deploy index.ts --env-file .env
```

## Configuration

Set these environment variables in your `.env` file:

```bash
# Primary provider (default: anthropic)
CUA_PROVIDER=anthropic

# Fallback providers, tried in order on provider errors (optional)
CUA_FALLBACK_PROVIDERS=openai,gemini

# API keys for each provider you want to use
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
GOOGLE_API_KEY=AI...
```

## Usage

```bash
# Use default provider
kernel invoke ts-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'

# Override provider per-request
kernel invoke ts-cua cua-task --payload '{"query": "Search for kernel.sh", "provider": "openai"}'

# With replay recording
kernel invoke ts-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

## How Fallback Works

The fallback mechanism triggers on **provider-level errors** only:
- Rate limits (429)
- Server errors (500, 502, 503)
- Authentication failures
- Network errors

It does **not** fall back on task-level failures (e.g., the model completed but gave an incorrect answer). This prevents wasting API calls when the issue is with the task, not the provider.

## Architecture

```
index.ts              Entry point, Kernel app registration
session.ts            Browser lifecycle management (shared)
tools.ts              Common action types + Kernel API executor (shared)
providers/
  index.ts            Provider factory + fallback logic
  anthropic.ts        Anthropic Claude computer use adapter
  openai.ts           OpenAI CUA adapter
  gemini.ts           Google Gemini computer use adapter
```

## Resources

- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
- [Anthropic Computer Use](https://docs.anthropic.com/en/docs/build-with-claude/computer-use)
- [OpenAI Computer Use](https://platform.openai.com/docs/guides/computer-use)
- [Gemini Computer Use](https://ai.google.dev/gemini-api/docs/computer-use)
