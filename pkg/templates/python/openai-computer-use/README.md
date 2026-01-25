# Kernel Python Sample App - OpenAI Computer Use

This is a Kernel application that demonstrates using the Computer Use Agent (CUA) from OpenAI.

It generally follows the [OpenAI CUA Sample App Reference](https://github.com/openai/openai-cua-sample-app) and uses Kernel's [Computer Controls API](https://www.kernel.sh/docs/browsers/computer-controls) for browser automation.

## Features

- **OS-level input emulation**: Uses Kernel's Computer Controls API for mouse, keyboard, and screenshot operations instead of CDP/Playwright
- **Reduced bot detection**: OS-level controls avoid CDP traces that bot detection systems look for
- **Replay recording**: Optional video replay of browser sessions for debugging
- **Stealth mode**: Built-in stealth mode to further reduce detection

## Architecture

The app uses two main components:

1. **KernelComputer** (`computers/default/kernel_computer.py`): Implements browser control using Kernel's Computer Controls API
2. **KernelBrowserSession** (`session.py`): Manages browser lifecycle with optional replay recording

A Playwright-based fallback is also available for local development (`computers/default/local_playwright.py` and `kernel.py`).

## Usage

```bash
# Login to Kernel
kernel login

# Deploy with your OpenAI API key
kernel deploy main.py -e OPENAI_API_KEY=sk-xxx --force

# Invoke the action
kernel invoke python-openai-cua cua-task -p '{"task":"go to https://news.ycombinator.com and list top 5 articles"}'

# With replay recording enabled
kernel invoke python-openai-cua cua-task -p '{"task":"search for the current weather", "record_replay": true}'
```

See the [Kernel docs](https://www.kernel.sh/docs/quickstart) for more information.