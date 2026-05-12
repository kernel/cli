# Kernel Python Sample App - Tzafon Northstar Computer Use

This is a Kernel application that implements a CUA (computer use agent) loop using Tzafon's Northstar CUA Fast model with Kernel's Computer Controls API. The model is accessed via Tzafon's [Lightcone](https://docs.lightcone.ai) API platform.

[Northstar CUA Fast](https://docs.lightcone.ai) is a vision language model trained with reinforcement learning for computer use tasks.

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Tzafon**: [tzafon.ai](https://www.tzafon.ai)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your TZAFON_API_KEY
kernel deploy main.py --env-file .env
```

## Usage

```bash
kernel invoke python-tzafon-cua cua-task --payload '{"query": "Go to wikipedia.org and search for Alan Turing"}'
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke python-tzafon-cua cua-task --payload '{"query": "Navigate to https://example.com", "record_replay": true}'
```

When enabled, the response will include a `replay_url` field with a link to view the recorded session.

## Viewport Configuration

Northstar CUA Fast works well with a **1280x800** viewport, which is the default.

## Supported Actions

| Action | Description |
|--------|-------------|
| `click` | Left or right mouse click at coordinates |
| `double_click` | Double-click at coordinates |
| `point_and_type` | Click at coordinates then type text (with optional Enter) |
| `key` | Press key combo (e.g. `Enter`, `ctrl+a`) |
| `scroll` | Scroll at coordinates |
| `drag` | Click-and-drag from start to end coordinates |
| `done` | Signal task completion with a result summary |

## Resources

- [Lightcone API Documentation](https://docs.lightcone.ai)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
