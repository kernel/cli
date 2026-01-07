# QA Testing for `kernel create`

This command runs QA testing for all `kernel create` template permutations.

## Overview

You will build the CLI, create all template variations, deploy them, and provide invoke commands for manual testing.

## Step 1: Build the CLI

From the cli repository root:

```bash
make build
```

The built binary will be at `./bin/kernel`.

## Step 2: Create QA Directory

Create a timestamped QA directory:

```bash
QA_DIR="./qa-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$QA_DIR"
cd "$QA_DIR"
```

## Step 3: Get KERNEL_API_KEY

**STOP and ask the human for their `KERNEL_API_KEY`.**

This is required for all deployments. Store it for use in deploy commands:

```bash
export KERNEL_API_KEY="<value from human>"
```

## Step 4: Create All Templates

Use the built CLI binary with non-interactive flags. The command format is:

```bash
../bin/kernel create -n <name> -l <language> -t <template>
```

### Template Matrix

Here are all valid language + template combinations:

| Language   | Template               | Folder Name       | Deployed App Name     | Needs Env File | Required Env Vars              |
| ---------- | ---------------------- | ----------------- | --------------------- | -------------- | ------------------------------ |
| typescript | sample-app             | ts-sample-app     | ts-basic              | No             | -                              |
| typescript | captcha-solver         | ts-captcha-solver | ts-captcha-solver     | No             | -                              |
| typescript | stagehand              | ts-stagehand      | ts-stagehand          | Yes            | OPENAI_API_KEY                 |
| typescript | anthropic-computer-use | ts-anthropic-cua  | ts-anthropic-cua      | Yes            | ANTHROPIC_API_KEY              |
| typescript | magnitude              | ts-magnitude      | ts-magnitude          | Yes            | ANTHROPIC_API_KEY              |
| typescript | openai-computer-use    | ts-openai-cua     | ts-openai-cua         | Yes            | OPENAI_API_KEY                 |
| typescript | gemini-computer-use    | ts-gemini-cua     | ts-gemini-cua         | Yes            | GOOGLE_API_KEY                 |
| python     | sample-app             | py-sample-app     | python-basic          | No             | -                              |
| python     | captcha-solver         | py-captcha-solver | python-captcha-solver | No             | -                              |
| python     | browser-use            | py-browser-use    | python-bu             | Yes            | OPENAI_API_KEY                 |
| python     | anthropic-computer-use | py-anthropic-cua  | python-anthropic-cua  | Yes            | ANTHROPIC_API_KEY              |
| python     | openai-computer-use    | py-openai-cua     | python-openai-cua     | Yes            | OPENAI_API_KEY                 |
| python     | openagi-computer-use   | py-openagi-cua    | python-openagi-cua    | Yes            | OAGI_API_KEY                   |

### Create Commands

Run each of these (they are non-interactive when all flags are provided):

```bash
# TypeScript templates
../bin/kernel create -n ts-sample-app -l typescript -t sample-app
../bin/kernel create -n ts-captcha-solver -l typescript -t captcha-solver
../bin/kernel create -n ts-stagehand -l typescript -t stagehand
../bin/kernel create -n ts-anthropic-cua -l typescript -t anthropic-computer-use
../bin/kernel create -n ts-magnitude -l typescript -t magnitude
../bin/kernel create -n ts-openai-cua -l typescript -t openai-computer-use
../bin/kernel create -n ts-gemini-cua -l typescript -t gemini-computer-use

# Python templates
../bin/kernel create -n py-sample-app -l python -t sample-app
../bin/kernel create -n py-captcha-solver -l python -t captcha-solver
../bin/kernel create -n py-browser-use -l python -t browser-use
../bin/kernel create -n py-anthropic-cua -l python -t anthropic-computer-use
../bin/kernel create -n py-openai-cua -l python -t openai-computer-use
../bin/kernel create -n py-openagi-cua -l python -t openagi-computer-use
```

## Step 5: Deploy Each Template

For each template directory, you need to:

1. `cd` into the directory
2. If `NeedsEnvFile` is true, **STOP and ask the human** for the required API keys
3. Create a `.env` file with those values
4. Run the deploy command

### Deploy Commands by Template

#### Templates WITHOUT env files (deploy directly):

```bash
# ts-sample-app
cd ts-sample-app && ../bin/kernel deploy index.ts && cd ..

# ts-captcha-solver
cd ts-captcha-solver && ../bin/kernel deploy index.ts && cd ..

# py-sample-app
cd py-sample-app && ../bin/kernel deploy main.py && cd ..

# py-captcha-solver
cd py-captcha-solver && ../bin/kernel deploy main.py && cd ..
```

#### Templates WITH env files (prompt human first):

For each of these, **STOP and ask the human** for the required API key(s), then create the `.env` file and deploy:

**ts-stagehand** (needs OPENAI_API_KEY):

```bash
cd ts-stagehand
echo "OPENAI_API_KEY=<value from human>" > .env
../bin/kernel deploy index.ts --env-file .env
cd ..
```

**ts-anthropic-cua** (needs ANTHROPIC_API_KEY):

```bash
cd ts-anthropic-cua
echo "ANTHROPIC_API_KEY=<value from human>" > .env
../bin/kernel deploy index.ts --env-file .env
cd ..
```

**ts-magnitude** (needs ANTHROPIC_API_KEY):

```bash
cd ts-magnitude
echo "ANTHROPIC_API_KEY=<value from human>" > .env
../bin/kernel deploy index.ts --env-file .env
cd ..
```

**ts-openai-cua** (needs OPENAI_API_KEY):

```bash
cd ts-openai-cua
echo "OPENAI_API_KEY=<value from human>" > .env
../bin/kernel deploy index.ts --env-file .env
cd ..
```

**ts-gemini-cua** (needs GOOGLE_API_KEY):

```bash
cd ts-gemini-cua
echo "GOOGLE_API_KEY=<value from human>" > .env
../bin/kernel deploy index.ts --env-file .env
cd ..
```

**py-browser-use** (needs OPENAI_API_KEY):

```bash
cd py-browser-use
echo "OPENAI_API_KEY=<value from human>" > .env
../bin/kernel deploy main.py --env-file .env
cd ..
```

**py-anthropic-cua** (needs ANTHROPIC_API_KEY):

```bash
cd py-anthropic-cua
echo "ANTHROPIC_API_KEY=<value from human>" > .env
../bin/kernel deploy main.py --env-file .env
cd ..
```

**py-openai-cua** (needs OPENAI_API_KEY):

```bash
cd py-openai-cua
echo "OPENAI_API_KEY=<value from human>" > .env
../bin/kernel deploy main.py --env-file .env
cd ..
```

**py-openagi-cua** (needs OAGI_API_KEY):

```bash
cd py-openagi-cua
echo "OAGI_API_KEY=<value from human>" > .env
../bin/kernel deploy main.py --env-file .env
cd ..
```

## Step 6: Provide Invoke Commands

Once all deployments are complete, present the human with these invoke commands to test manually:

```bash
# TypeScript apps
kernel invoke ts-basic get-page-title --payload '{"url": "https://www.google.com"}'
kernel invoke ts-captcha-solver test-captcha-solver
kernel invoke ts-stagehand teamsize-task --payload '{"company": "Kernel"}'
kernel invoke ts-anthropic-cua cua-task --payload '{"query": "Return the first url of a search result for NYC restaurant reviews Pete Wells"}'
kernel invoke ts-magnitude mag-url-extract --payload '{"url": "https://en.wikipedia.org/wiki/Special:Random"}'
kernel invoke ts-openai-cua cua-task --payload '{"task": "Go to https://news.ycombinator.com and get the top 5 articles"}'
kernel invoke ts-gemini-cua gemini-cua-task --payload '{"startingUrl": "https://www.magnitasks.com/", "instruction": "Click the Tasks option in the left-side bar, and move the 5 items in the To Do and In Progress items to the Done section of the Kanban board? You are done successfully when the items are moved."}'

# Python apps
kernel invoke python-basic get-page-title --payload '{"url": "https://www.google.com"}'
kernel invoke python-captcha-solver test-captcha-solver
kernel invoke python-bu bu-task --payload '{"task": "Compare the price of gpt-4o and DeepSeek-V3"}'
kernel invoke python-anthropic-cua cua-task --payload '{"query": "Return the first url of a search result for NYC restaurant reviews Pete Wells"}'
kernel invoke python-openai-cua cua-task --payload '{"task": "Go to https://news.ycombinator.com and get the top 5 articles"}'
kernel invoke python-openagi-cua openagi-default-task -p '{"instruction": "Navigate to https://agiopen.org and click the What is Computer Use? button"}'
```

## Step 7: Automated Runtime Testing (Optional)

**STOP and ask the human:** "Would you like me to automatically invoke all 13 templates and report back on their runtime status?"

If the human agrees, invoke each template and collect results. Present findings in this format:

### Testing Guidelines
- **Parallel execution:** You may run multiple invocations in parallel to speed up testing.
- **Error handling:** Capture any runtime errors and include them in the Notes column.

### Test Results

| Template          | App Name              | Status  | Notes |
| ----------------- | --------------------- | ------- | ----- |
| ts-sample-app     | ts-basic              |         |       |
| ts-captcha-solver | ts-captcha-solver     |         |       |
| ts-stagehand      | ts-stagehand          |         |       |
| ts-anthropic-cua  | ts-anthropic-cua      |         |       |
| ts-magnitude      | ts-magnitude          |         |       |
| ts-openai-cua     | ts-openai-cua         |         |       |
| ts-gemini-cua     | ts-gemini-cua         |         |       |
| py-sample-app     | python-basic          |         |       |
| py-captcha-solver | python-captcha-solver |         |       |
| py-browser-use    | python-bu             |         |       |
| py-anthropic-cua  | python-anthropic-cua  |         |       |
| py-openai-cua     | python-openai-cua     |         |       |
| py-openagi-cua    | python-openagi-cua    |         |       |

Status values:
- **SUCCESS**: App started and returned a result
- **FAILED**: App encountered a runtime error

Notes should include brief error messages for failures or confirmation of successful output.

## Summary Checklist

- [ ] Built CLI with `make build`
- [ ] Created QA directory
- [ ] Got KERNEL_API_KEY from human
- [ ] Created all 13 template variations
- [ ] Got required API keys from human (OPENAI_API_KEY, ANTHROPIC_API_KEY, GOOGLE_API_KEY, OAGI_API_KEY)
- [ ] Deployed all 13 apps
- [ ] Provided invoke commands to human for manual testing
- [ ] (Optional) Ran automated runtime testing and reviewed results

## Cleanup

After QA is complete, the human can remove the QA directory:

```bash
cd ..
rm -rf "$QA_DIR"
```
