# Kernel TypeScript Sample App - Anthropic Computer Use

This is a Kernel application that runs Anthropic Computer Use against a Kernel cloud browser.

It uses [`@onkernel/cua-agent`](https://www.npmjs.com/package/@onkernel/cua-agent) to run the computer-use loop: the `CuaAgent` class translates Claude's computer-use tool calls into Kernel browser controls and feeds a fresh screenshot back on every turn. The app entry point just provisions a browser, hands it to `CuaAgent`, and returns the final answer.

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Anthropic**: [console.anthropic.com](https://console.anthropic.com)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your ANTHROPIC_API_KEY
kernel deploy index.ts --env-file .env
```

## Usage

```bash
kernel invoke ts-anthropic-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke ts-anthropic-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Resources

- [@onkernel/cua-agent](https://www.npmjs.com/package/@onkernel/cua-agent)
- [Anthropic Computer Use Documentation](https://docs.anthropic.com/en/docs/build-with-claude/computer-use)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
