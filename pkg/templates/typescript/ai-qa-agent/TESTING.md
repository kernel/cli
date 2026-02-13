# Testing the QA Agent

## Prerequisites

1. **Install dependencies:**
   ```bash
   npm install
   # or
   pnpm install
   ```

2. **Set up environment variables:**
   ```bash
   cp env.example .env
   # Edit .env with your actual values:
   # - ANTHROPIC_API_KEY (required for Computer Use navigation)
   # - OPENAI_API_KEY (optional, for GPT-4o analysis)
   # - GOOGLE_API_KEY (optional, for Gemini analysis)
   ```

3. **Login to Kernel:**
   ```bash
   kernel login
   ```

## Option 1: Test via Web UI (Easiest)

The web UI provides a visual interface for testing:

```bash
npm run ui
# or
pnpm ui
```

Then open http://localhost:3000 in your browser. You can:
- Enter a URL to test
- Select which checks to run
- Choose the analysis model
- View results in real-time

## Option 2: Deploy and Test on Kernel (Recommended)

This is the standard way to test since the template uses Kernel's browser API.

### Step 1: Deploy

```bash
kernel deploy index.ts --env-file .env
```

### Step 2: Test Basic QA Analysis

```bash
kernel invoke ts-qa-agent qa-test \
  --payload '{
    "url": "https://example.com",
    "model": "claude",
    "checks": {
      "compliance": {
        "accessibility": true,
        "legal": true
      },
      "policyViolations": {
        "content": true,
        "security": true
      },
      "brokenUI": true
    }
  }'
```

### Step 3: Test with Industry Context

```bash
kernel invoke ts-qa-agent qa-test \
  --payload '{
    "url": "https://cash.app",
    "model": "claude",
    "checks": {
      "compliance": {
        "accessibility": true,
        "legal": true,
        "regulatory": true
      }
    },
    "context": {
      "industry": "finance"
    }
  }'
```

### Step 4: Test with Custom Policies

```bash
kernel invoke ts-qa-agent qa-test \
  --payload '{
    "url": "https://example.com",
    "model": "claude",
    "checks": {
      "policyViolations": {
        "content": true
      }
    },
    "context": {
      "customPolicies": "No profanity allowed. All claims must be substantiated."
    }
  }'
```

## Option 3: Local Type Check

Verify TypeScript compilation without deploying:

```bash
npx tsc --noEmit
```

## Option 4: View Deployment Logs

While testing, you can watch logs in real-time:

```bash
# Get the deployment ID from the deploy output, then:
kernel deploy logs <deployment_id> --follow
```

## Example Test Flow

```bash
# 1. Setup
cd /path/to/ai-qa-agent
npm install
cp env.example .env
# Edit .env with your ANTHROPIC_API_KEY

# 2. Deploy
kernel deploy index.ts --env-file .env

# 3. Test basic analysis
kernel invoke ts-qa-agent qa-test \
  --payload '{
    "url": "https://example.com",
    "model": "claude",
    "checks": {
      "compliance": {"accessibility": true, "legal": true},
      "policyViolations": {"content": true},
      "brokenUI": true
    }
  }'

# 4. Check logs (in another terminal)
kernel deploy logs <deployment_id> --follow
```

## Testing Different Models

You can test with different vision models for analysis:

### Claude (Default, Recommended)
```bash
kernel invoke ts-qa-agent qa-test \
  --payload '{"url": "https://example.com", "model": "claude", ...}'
```

### GPT-4o
```bash
# Requires OPENAI_API_KEY in .env
kernel invoke ts-qa-agent qa-test \
  --payload '{"url": "https://example.com", "model": "gpt4o", ...}'
```

### Gemini
```bash
# Requires GOOGLE_API_KEY in .env
kernel invoke ts-qa-agent qa-test \
  --payload '{"url": "https://example.com", "model": "gemini", ...}'
```

## Troubleshooting

### "ANTHROPIC_API_KEY is required"
- Make sure your `.env` file has `ANTHROPIC_API_KEY` set
- This is required for Computer Use navigation (even if using GPT-4o/Gemini for analysis)

### "OPENAI_API_KEY is required" or "GOOGLE_API_KEY is required"
- Only needed if you're using `"model": "gpt4o"` or `"model": "gemini"`
- For Claude analysis, only `ANTHROPIC_API_KEY` is needed

### Browser session fails
- Check that you're logged into Kernel: `kernel login`
- Verify your Kernel API key is valid
- Check deployment logs for errors

### Claude can't navigate to the URL
- The URL might be blocking automated browsers
- Try enabling `"dismissPopups": true` in the payload
- Check the live view URL in logs to see what Claude sees
- You may need to customize the system prompt in `src/qa-computer-use.ts` for specific sites

### No issues found on a problematic page
- Try a different analysis model (Claude is most thorough)
- Check that the page loaded correctly in the live view URL
- Verify the checks you're running are enabled in the payload

## Expected Behavior

When testing, you should see:

1. **Navigation logs**: Claude navigating to the URL, scrolling, dismissing popups
2. **Screenshot capture**: Multiple screenshots as Claude explores the page
3. **Analysis logs**: Compliance, policy, and visual checks running
4. **Results**: JSON and HTML reports with found issues

The process typically takes 30-60 seconds depending on:
- Page complexity
- Number of checks enabled
- Model used for analysis
