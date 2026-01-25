# Kernel TypeScript Sample App - OpenAI Computer Use

This is a Kernel application that demonstrates using the Computer Use Agent (CUA) from OpenAI.

It generally follows the [OpenAI CUA Sample App Reference](https://github.com/openai/openai-cua-sample-app) and uses Kernel's [Computer Controls API](https://www.kernel.sh/docs/browsers/computer-controls) for browser automation.

## Features

- **OS-level input emulation**: Uses Kernel's Computer Controls API for mouse, keyboard, and screenshot operations instead of CDP/Playwright
- **Reduced bot detection**: OS-level controls avoid CDP traces that bot detection systems look for
- **Replay recording**: Optional video replay of browser sessions for debugging
- **Stealth mode**: Built-in stealth mode to further reduce detection

## Architecture

The app uses two main components:

1. **KernelComputer** (`lib/kernel-computer.ts`): Implements browser control using Kernel's Computer Controls API
2. **KernelBrowserSession** (`lib/session.ts`): Manages browser lifecycle with optional replay recording

A Playwright-based fallback is also available for local development (`lib/playwright/`).

## Usage

```bash
# Login to Kernel
kernel login

# Deploy with your OpenAI API key
kernel deploy index.ts -e OPENAI_API_KEY=sk-xxx --force

# Invoke the action
kernel invoke ts-openai-cua cua-task -p '{"task":"search for the current weather in San Francisco"}'

# With replay recording enabled
kernel invoke ts-openai-cua cua-task -p '{"task":"search for the current weather", "record_replay": true}'
```

See the [Kernel docs](https://www.kernel.sh/docs/quickstart) for more information.
