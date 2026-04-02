# Cloudglue Session Recap

Analyze browser session recordings with [Cloudglue](https://cloudglue.dev) to get a detailed scene-by-scene recap — thumbnails, timestamps, user actions, and screen descriptions.

## What it does

The **`session-recap`** action takes any video URL (a Kernel session recording, screen capture, or video file) and produces:

- A generated title and summary
- A thumbnail grid overview (4 columns)
- A detailed scene breakdown with timestamps, descriptions, user actions, URLs, and screen state
- A complete markdown document combining all of the above

Uses Cloudglue describe (for visual timeline + thumbnails) and segment-level extract (for structured user actions) in parallel.

## Input

```json
{
  "recording_url": "https://example.com/recordings/replay.mp4",
  "title": "Login Flow Test",
  "max_seconds": 8
}
```

Only `recording_url` is required. `title` defaults to "Session Recap of \<url\>" and `max_seconds` defaults to 8.

## Output

```json
{
  "title": "Login Flow Test",
  "recording_url": "https://example.com/recordings/replay.mp4",
  "summary": "A user navigates to the login page, enters credentials, and is redirected to the dashboard.",
  "duration_seconds": 22.5,
  "scene_count": 4,
  "scenes": [
    {
      "timestamp": "00:00",
      "thumbnail_url": "https://...",
      "description": "Login page loaded with email and password input fields.",
      "user_action": "Navigated to the login page",
      "screen_description": "Login form with email field, password field, and Sign In button."
    },
    {
      "timestamp": "00:06",
      "thumbnail_url": "https://...",
      "description": "User enters email and password into the login form.",
      "user_action": "Typed credentials into the email and password fields",
      "screen_description": "Login form with filled-in fields and cursor in the password input."
    }
  ],
  "markdown": "# Login Flow Test\n\n- **Recording:** ..."
}
```

## Setup

Create a `.env` file:

```
CLOUDGLUE_API_KEY=your-cloudglue-api-key
```

Get your API key at [app.cloudglue.dev](https://app.cloudglue.dev).

## Deploy

```bash
kernel login
kernel deploy index.ts --env-file .env
```

## Invoke

```bash
# Basic — auto-generates title from URL
kernel invoke ts-cloudglue-session-recap session-recap \
  --payload '{"recording_url": "https://your-recording-url.com/replay.mp4"}'

# With custom title
kernel invoke ts-cloudglue-session-recap session-recap \
  --payload '{"recording_url": "https://...", "title": "Login Flow Test"}'

# With longer scene windows (default is 8s, max 60s)
kernel invoke ts-cloudglue-session-recap session-recap \
  --payload '{"recording_url": "https://...", "max_seconds": 15}'
```
