# Kernel TypeScript Sample App - OpenAI Computer Use

This is a Kernel application that runs an OpenAI computer-use agent against a Kernel cloud browser.

It uses [`@onkernel/cua-agent`](https://www.npmjs.com/package/@onkernel/cua-agent) to run the computer-use loop: the `CuaAgent` class translates OpenAI's computer-use tool calls into Kernel browser controls and feeds a fresh screenshot back on every turn. OpenAI's computer tool has no native URL navigation, so the template enables `computerUseExtra` to give the model a `goto`/`back`/`forward`/`url` helper.

## Deploy to Kernel

```bash
kernel login
cp .env.example .env  # Add your OPENAI_API_KEY
kernel deploy index.ts --env-file .env
kernel invoke ts-openai-cua cua-task -p '{"task":"Go to https://news.ycombinator.com and get the top 5 articles"}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"replay": true` to your payload to capture a video of the browser session. When enabled, the response includes a `replay_url` field with a link to view the recording.

```bash
kernel invoke ts-openai-cua cua-task -p '{"task":"Go to https://news.ycombinator.com", "replay": true}'
```

## Resources

- [@onkernel/cua-agent](https://www.npmjs.com/package/@onkernel/cua-agent)
- [OpenAI Computer Use Documentation](https://platform.openai.com/docs/guides/tools-computer-use)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
