# Kernel TypeScript Sample App - Gemini Computer Use

A Kernel application that demonstrates Computer Use Agent (CUA) capabilities using Google's Gemini 2.5 model with Stagehand for browser automation.

## What It Does

This app uses [Gemini 2.5's computer use model](https://blog.google/technology/google-deepmind/gemini-computer-use-model/) capabilities to autonomously navigate websites and complete tasks. The agent can interact with web pages just like a human would - clicking, typing, scrolling, and extracting information.

## Setup

1. **Add your API keys as environment variables:**
   - `KERNEL_API_KEY` - Get from [Kernel dashboard](https://dashboard.onkernel.com/sign-in)
   - `GOOGLE_API_KEY` - Get from [Google AI Studio](https://aistudio.google.com/apikey)

## Running Locally

Execute the script directly with tsx:

```bash
npx tsx index.ts
```

This runs the agent without a Kernel invocation context and provides the browser live view URL for debugging.

## Deploying to Kernel

1. **Copy the example env file, add your API keys, and deploy:**
   ```bash
   cp .example.env .env
   kernel deploy index.ts --env-file .env
   ```

2. **Invoke the action:**
   ```bash
   kernel invoke ts-gemini-cua gemini-cua-task
   ```

The action creates a Kernel-managed browser and associates it with the invocation for tracking and monitoring.

## Alternative Model Providers

Stagehand's CUA agent supports multiple model providers. You can switch from Gemini to OpenAI or Anthropic by changing the model configuration in `index.ts` and redeploying your Kernel app:

**OpenAI Computer Use:**
```typescript
model: {
    modelName: "openai/computer-use-preview",
    apiKey: process.env.OPENAI_API_KEY
}
```

**Anthropic Claude Sonnet:**
```typescript
model: {
    modelName: "anthropic/claude-sonnet-4-20250514",
    apiKey: process.env.ANTHROPIC_API_KEY
}
```

When using alternative providers, make sure to:
1. Add the corresponding API key to your environment variables
2. Update the deploy command to include the new API key (e.g., `--env OPENAI_API_KEY=XXX`)

## Documentation

- [Kernel Documentation](https://docs.onkernel.com/quickstart)
- [Kernel Stagehand Guide](https://www.onkernel.com/docs/integrations/stagehand)
- [Gemini 2.5 Computer Use](https://blog.google/technology/google-deepmind/gemini-computer-use-model/)
