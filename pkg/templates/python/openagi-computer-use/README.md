# Kernel Python Sample App - OpenAGI Computer Use

This is a Kernel application that demonstrates using OpenAGI's Lux computer-use models for browser automation.

## Overview

This template provides two agent types from the [OpenAGI SDK](https://github.com/onkernel/kernel-oagi):

### AsyncDefaultAgent
Best for high-level tasks with immediate execution. Supports two models:
- `lux-actor-1`: Fast execution (~1s/step), simple linear tasks
- `lux-thinker-1`: Complex planning, comparison tasks, handling ambiguity

### TaskerAgent
Best for structured workflows with predefined steps (todos).

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **OpenAGI**: [developer.agiopen.org](https://developer.agiopen.org)

2. Deploy the app:
```bash
kernel login
cp .env.example .env
kernel deploy main.py --env-file .env
```

## Usage

### AsyncDefaultAgent

Execute high-level tasks with optional model selection:

```bash
# Default model (lux-actor-1)
kernel invoke python-openagi-cua openagi-default-task \
  -p '{"instruction": "Navigate to https://agiopen.org and click the What is Computer Use? button"}'

# With specific model
kernel invoke python-openagi-cua openagi-default-task \
  -p '{"instruction": "Navigate to https://developer.agiopen.org/docs and find the Lux model pricing page.", "model": "lux-thinker-1"}'
```

### TaskerAgent

Execute structured workflows with predefined steps:

```bash
kernel invoke python-openagi-cua openagi-tasker-task \
  -p '{"task": "Navigate to OAGI documentation and navigate to the What is Computer Use? section", "todos": ["Go to https://agiopen.org", "Click on the What is Computer Use? button", "Highlight point number 2 about computer use."]}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Both actions support optional video replay recording. Add `"record_replay": "True"` to your payload to capture a video of the browser session:

```bash
kernel invoke python-openagi-cua openagi-default-task \
  -p '{"instruction": "Navigate to https://agiopen.org", "record_replay": "True"}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Model Selection Guide

| Model | Best For | Avoid When |
|-------|----------|------------|
| `lux-actor-1` | Fast execution, simple linear tasks (10-20 steps) | Complex reasoning, comparison tasks |
| `lux-thinker-1` | Complex planning, comparison tasks, handling ambiguity | Low latency needs, simple click-paths |

## Resources

- [OpenAGI Documentation](https://developer.agiopen.org)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
- [Kernel + OpenAGI Template Repository](https://github.com/onkernel/kernel-oagi)
