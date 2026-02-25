# Kernel Python Sample App - OpenAI Computer Use

This is a Kernel application that demonstrates using the Computer Use Agent (CUA) from OpenAI with Kernel's native browser control API.

It uses Kernel's computer control endpoints (screenshot, click, type, scroll, batch, etc.) instead of Playwright, and includes a `batch_computer_actions` tool that executes multiple actions in a single API call for lower latency.

## Local testing

You can test against a remote Kernel browser without deploying:

```bash
cp .env.example .env
# Fill in OPENAI_API_KEY and KERNEL_API_KEY in .env
uv run test_local.py
```

## Deploy to Kernel

```bash
kernel deploy main.py --env-file .env
kernel invoke python-openai-cua cua-task -p '{"task":"go to https://news.ycombinator.com and list top 5 articles"}'
```

See the [docs](https://www.kernel.sh/docs/quickstart) for more information.
