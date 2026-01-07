# Kernel TypeScript Magnitude.run Template

This template demonstrates integrating Magnitude.run with a Kernel app.

It defines a single action that:
- Navigates to a given URL (via Magnitude `agent.act()`)
- Extracts up to 5 absolute URLs on the page (via Magnitude `agent.extract()`)

## Quickstart

- Deploy:
  kernel login  # or: export KERNEL_API_KEY=<your_api_key>
  kernel deploy index.ts --env ANTHROPIC_API_KEY=XXX

- Invoke:
  kernel invoke ts-magnitude mag-url-extract --payload '{"url": "https://fandom.com"}'

## Notes
- Uses Anthropic as the model provider with model: `anthropic/claude-sonnet-4`.
- Requires `ANTHROPIC_API_KEY` in the deployment environment.
- The agent connects to the Kernel-managed browser via CDP for live view & observability.
