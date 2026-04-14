# Unified CUA Template

A multi-provider Computer Use Agent (CUA) template for [Kernel](https://kernel.sh). Supports **Anthropic**, **OpenAI**, **Google Gemini**, **Tzafon**, and **Yutori** as interchangeable backends with automatic fallback.

## Quick start

### 1. Install dependencies

```bash
npm install
```

### 2. Configure environment

Copy the example env file and add your API keys:

```bash
cp .env.example .env
```

Set `CUA_PROVIDER` to your preferred provider and add the matching API key:

| Provider    | Env var for key      | Model used                                 |
|-------------|----------------------|--------------------------------------------|
| `anthropic` | `ANTHROPIC_API_KEY`  | `claude-sonnet-4-6`                        |
| `openai`    | `OPENAI_API_KEY`     | `gpt-5.4`                                 |
| `gemini`    | `GOOGLE_API_KEY`     | `gemini-2.5-computer-use-preview-10-2025`  |
| `tzafon`    | `TZAFON_API_KEY`     | `tzafon.northstar-cua-fast`                |
| `yutori`    | `YUTORI_API_KEY`     | `n1-latest`                                |

### 3. Deploy to Kernel

```bash
kernel deploy index.ts --env-file .env
```

### 4. Invoke

```bash
kernel invoke ts-cua cua-task --payload '{"query": "Go to https://news.ycombinator.com and get the top 5 stories"}'
```

## Multi-provider fallback

Set `CUA_FALLBACK_PROVIDERS` to automatically try another provider if the primary fails:

```env
CUA_PROVIDER=anthropic
CUA_FALLBACK_PROVIDERS=openai,gemini
```

This will try Anthropic first, then OpenAI, then Gemini. Only providers with valid API keys are used.

## Replay recording

Pass `record_replay: true` in the payload to capture a video replay of the browser session:

```bash
kernel invoke ts-cua cua-task --payload '{"query": "Navigate to example.com", "record_replay": true}'
```

The response will include a `replay_url` you can open in your browser.

## Project structure

```
index.ts              — Kernel app entrypoint
session.ts            — Browser session lifecycle with replay support
providers/
  index.ts            — Provider factory and fallback logic
  anthropic.ts        — Anthropic Claude adapter
  openai.ts           — OpenAI GPT adapter
  gemini.ts           — Google Gemini adapter
  tzafon.ts           — Tzafon Northstar adapter
  yutori.ts           — Yutori n1 adapter
```

## Customization

Each provider adapter is self-contained. To customize a provider's behavior (system prompt, model, tool handling), edit the corresponding file in `providers/`.

To add a new provider, create a new file that implements the `CuaProvider` interface and register it in `providers/index.ts`.

## Resources

- [Kernel Docs](https://docs.kernel.sh)
- [Anthropic Computer Use](https://docs.anthropic.com/en/docs/agents-and-tools/computer-use)
- [OpenAI Computer Use](https://platform.openai.com/docs/guides/computer-use)
- [Google Gemini Computer Use](https://ai.google.dev/gemini-api/docs/computer-use)
- [Tzafon Lightcone](https://docs.lightcone.ai)
- [Yutori n1](https://docs.yutori.com/reference/n1)
