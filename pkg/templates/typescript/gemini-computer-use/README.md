# Kernel TypeScript Sample App - Gemini Computer Use

This is a Kernel application that runs Google's Gemini computer-use model against a Kernel cloud browser.

It uses [`@onkernel/cua-agent`](https://www.npmjs.com/package/@onkernel/cua-agent) to run the computer-use loop: the `CuaAgent` class translates Gemini's computer-use function calls into Kernel browser controls and feeds a fresh screenshot back on every turn. The app runs the `gemini-3-flash-preview` model.

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

## Resources

- [@onkernel/cua-agent](https://www.npmjs.com/package/@onkernel/cua-agent)
- [Google Gemini Computer Use Documentation](https://ai.google.dev/gemini-api/docs/computer-use)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
