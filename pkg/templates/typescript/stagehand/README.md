# Kernel TypeScript Sample App - Stagehand (v4)

A [Stagehand v4](https://docs.stagehand.dev) browser automation app that extracts team size information from Y Combinator company pages, running against a Kernel cloud browser.

## What it does

The `teamsize-task` searches for a startup on Y Combinator's company directory and extracts the team size (number of employees).

## How Stagehand v4 connects to a Kernel browser

Stagehand v4 no longer drives the browser purely over CDP — it runs as a **Chrome extension** next to the browser. To use it with a remote Kernel browser, this template:

1. Creates a Kernel browser.
2. Uploads the Stagehand extension (shipped inside the `@browserbasehq/stagehand` package) onto the running browser's filesystem, at the path Stagehand loads it from.
3. Connects with `localBrowser.connect({ cdpUrl })` — with no `extensionId`, Stagehand loads the extension into the running browser over CDP (`Extensions.loadUnpacked`) — and `Stagehand.create({ browser })`.

## Input

```json
{
  "company": "kernel"  // Startup name to search (optional, defaults to "kernel")
}
```

## Output

```json
{
  "teamSize": "11"  // Team size as shown on YC company page
}
```

## Setup

Create a `.env` file (defaults to OpenAI):

```
MODEL_API_KEY=your-openai-api-key
```

To use a different provider, also set `MODEL` (with a provider prefix):

```
MODEL=google/gemini-2.5-flash
MODEL_API_KEY=your-gemini-api-key
```

## Deploy

```bash
kernel login
kernel deploy index.ts --env-file .env
```

## Invoke

Default query (searches for "kernel"):
```bash
kernel invoke ts-stagehand teamsize-task
```

Custom query:
```bash
kernel invoke ts-stagehand teamsize-task --payload '{"company": "Mixpanel"}'
```
