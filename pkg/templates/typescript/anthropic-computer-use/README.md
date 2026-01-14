# Kernel TypeScript Sample App - Anthropic Computer Use

This is a Kernel application that implements a prompt loop using Anthropic Computer Use with Kernel's Computer Controls API.

It generally follows the [Anthropic Reference Implementation](https://github.com/anthropics/anthropic-quickstarts/tree/main/computer-use-demo) but uses Kernel's Computer Controls API instead of `xdotool` and `gnome-screenshot`.

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

## Known Limitations

### Cursor Position

The `cursor_position` action is not supported with Kernel's Computer Controls API. If the model attempts to use this action, an error will be returned. This is a known limitation that does not significantly impact most computer use workflows, as the model typically tracks cursor position through screenshots.

## Resources

- [Anthropic Computer Use Documentation](https://docs.anthropic.com/en/docs/build-with-claude/computer-use)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
