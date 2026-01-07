# Claude Agent SDK + Kernel Browser Automation

This template demonstrates how to use the [Claude Agent SDK](https://platform.claude.com/docs/en/agent-sdk/overview) with [Kernel's](https://onkernel.com) browser automation capabilities.

## Prerequisites

### Claude Code Installation

The Claude Agent SDK requires Claude Code to be installed. Follow the [official installation guide](https://platform.claude.com/docs/en/agent-sdk/overview#get-started):

```bash
# macOS/Linux/WSL
curl -fsSL https://claude.ai/install.sh | bash
```

> **Note:** When deploying to Kernel, the app automatically installs Claude Code on the remote infrastructure.

### API Keys

You'll need:
- **ANTHROPIC_API_KEY**: Get from the [Anthropic Console](https://console.anthropic.com/)
- **KERNEL_API_KEY**: Get from the [Kernel Dashboard](https://dashboard.onkernel.com/api-keys)

## Overview

The Claude Agent SDK provides a powerful way to build AI agents that can autonomously perform tasks. This example combines it with Kernel's Playwright Execution API to create an agent that can browse the web and interact with websites.

## Features

- **Claude Agent SDK**: Uses Claude's agent capabilities with built-in tool management
- **Kernel Browser**: Cloud-based browser with stealth mode and live view
- **Playwright Execution**: Execute Playwright code directly in the browser VM
- **In-process MCP Server**: Custom tool exposed via MCP for the agent to use
- **Dual Execution**: Run locally via CLI or deploy as a Kernel app

## Setup

1. Install dependencies:

```bash
uv sync
```

2. Set up environment variables:

```bash
cp .env.example .env
# Edit .env with your API keys
```

## Running Locally

Run the agent directly from the command line:

```bash
# Default task (Hacker News top stories)
uv run main.py

# Custom task
uv run main.py "Go to duckduckgo.com and search for 'Kernel browser automation'"

# Another example
uv run main.py "Go to https://github.com/trending and list the top 5 trending repositories"
```

## Deploying to Kernel

Deploy and invoke the app on Kernel's infrastructure:

```bash
# Login to Kernel
kernel login

# Deploy the app with environment variables
kernel deploy main.py --env-file .env

# Invoke the action
kernel invoke py-claude-agent-sdk agent-task -p '{"task": "Go to https://news.ycombinator.com and get the top 3 stories"}'

# Watch logs in a separate terminal
kernel logs py-claude-agent-sdk -f
```

## How It Works

1. **Browser Creation**: A Kernel browser session is created with stealth mode enabled
2. **MCP Server**: An in-process MCP server is created with an `execute_playwright` tool
3. **Agent Execution**: The Claude Agent SDK runs with access to the Playwright tool
4. **Task Completion**: Claude autonomously uses the tool to complete the given task
5. **Cleanup**: The browser session is deleted when done

When running on Kernel, the app first installs Claude Code on the remote infrastructure before executing the agent.

## Example Tasks

```bash
# Get top Hacker News stories
uv run main.py "Go to https://news.ycombinator.com and tell me the top 3 stories"

# Search for something
uv run main.py "Go to duckduckgo.com and search for 'Kernel browser automation'"

# Extract data from a page
uv run main.py "Go to https://github.com/trending and list the top 5 trending repositories"
```

## Environment Variables

| Variable            | Description                                |
| ------------------- | ------------------------------------------ |
| `ANTHROPIC_API_KEY` | Your Anthropic API key for Claude          |
| `KERNEL_API_KEY`    | Your Kernel API key for browser automation |

## API Response

When invoked via Kernel, the action returns:

```json
{
  "result": "The agent's final response text",
  "cost_usd": 0.1234,
  "duration_ms": 45000
}
```

## Learn More

- [Claude Agent SDK Documentation](https://platform.claude.com/docs/en/agent-sdk/overview)
- [Claude Agent SDK Get Started](https://platform.claude.com/docs/en/agent-sdk/overview#get-started)
- [Kernel Documentation](https://onkernel.com/docs)
- [Kernel Playwright Execution API](https://onkernel.com/docs/browsers/playwright-execution)
