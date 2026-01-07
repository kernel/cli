# Kernel TypeScript Sample App - Stagehand

A Stagehand-powered browser automation app that extracts team size information from Y Combinator company pages.

## What it does

The `teamsize-task` searches for a startup on Y Combinator's company directory and extracts the team size (number of employees).

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

Create a `.env` file:

```
OPENAI_API_KEY=your-openai-api-key
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
