# Eval Protocol Template

VLM browser agent evaluation using the [Eval Protocol](https://evalprotocol.com) framework with Kernel browser pools.

## Overview

This template provides tools for evaluating Vision-Language Model (VLM) browser agents on computer use tasks. It includes:

- **QwenAgent**: VLM-based agent for browser automation using Qwen3-VL models
- **WebJudge**: LLM-as-judge scoring for trajectory evaluation
- **Browser Pools**: Parallel browser execution for efficient evaluation
- **Agent Auth Benchmark**: Login discovery task dataset

## Setup

### Prerequisites

1. **Kernel API Key**: Get from [onkernel.com](https://onkernel.com)
2. **Fireworks API Key**: Get from [fireworks.ai](https://fireworks.ai) (for VLM inference)
3. **OpenAI API Key**: Get from [platform.openai.com](https://platform.openai.com) (for WebJudge scoring)

### Installation

```bash
# Install dependencies
uv sync

# Copy environment variables
cp .env.example .env
# Edit .env with your API keys
```

### Create Browser Pool (for parallel evaluation)

```bash
kernel pools create eval-browser-pool --size 20
```

## Usage

### Deploy the App

```bash
kernel deploy main.py --env-file .env
```

### Actions

#### 1. Run Single Rollout

Execute a single browser rollout for testing or debugging:

```bash
kernel invoke python-eval-protocol run-rollout --payload '{
  "task": "Navigate to github.com and find the sign in page",
  "initial_url": "https://github.com",
  "max_steps": 15,
  "score": true
}'
```

#### 2. Run Evaluation

Run parallel evaluation on the task dataset. The response includes `results_jsonl` containing the scored trajectories in Fireworks RFT dataset format — pass this directly to `create-rft-job`:

```bash
# With existing pool
kernel invoke python-eval-protocol run-evaluation --payload '{
  "pool_name": "eval-browser-pool",
  "max_tasks": 10
}'

# With ephemeral pool
kernel invoke python-eval-protocol run-evaluation --payload '{
  "pool_size": 20,
  "max_tasks": 50
}'
```

The response `results_jsonl` field contains the JSONL content to pass to `create-rft-job`.

#### 3. Create RFT Job

Create a reinforcement fine-tuning job via the Fireworks API (no CLI required). Takes the inline JSONL from `run-evaluation` and a pre-created evaluator on Fireworks.

**One-time setup:** Create an evaluator in the [Fireworks dashboard](https://app.fireworks.ai/dashboard/evaluators) (or via their API). Note your **Account ID** (from dashboard URL or account settings) and **Evaluator ID**.

```bash
# Pass results_jsonl from run-evaluation response
kernel invoke python-eval-protocol create-rft-job --payload '{
  "account_id": "YOUR_FIREWORKS_ACCOUNT_ID",
  "results_jsonl": "<paste results_jsonl from run-evaluation output>",
  "evaluator_id": "YOUR_EVALUATOR_ID",
  "base_model": "accounts/fireworks/models/qwen3-vl-8b-instruct",
  "epochs": 4
}'
```

Returns `job_id`, `dataset_id`, `status`, and `dashboard_url` for monitoring.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     run-evaluation Action                       │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  1. Load tasks from tasks.jsonl                          │  │
│  │  2. Create/use browser pool                              │  │
│  │  3. Run tasks in parallel (concurrency = pool size)      │  │
│  │  4. Score each trajectory with WebJudge                  │  │
│  │  5. Build JSONL content (returned inline as results_jsonl)  │  │
│  │  6. Aggregate and return results                            │  │
│  └──────────────────────────────────────────────────────────┘  │
│                            │                                    │
│                            ▼                                    │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Per-Task Rollout                                        │  │
│  │    1. Acquire browser from pool                          │  │
│  │    2. Navigate to initial URL                            │  │
│  │    3. Run QwenAgent loop                                 │  │
│  │       - Screenshot → VLM predict → Execute → Repeat      │  │
│  │    4. Capture trajectory (screenshots, actions)          │  │
│  │    5. Release browser back to pool                       │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                             │
                             ▼
              ┌─────────────────────────────┐
              │      Kernel Browser Pool     │
              │  ┌─────┐ ┌─────┐ ┌─────┐    │
              │  │  🌐 │ │  🌐 │ │  🌐 │    │
              │  └─────┘ └─────┘ └─────┘    │
              └─────────────────────────────┘
```

## Project Structure

```
eval-protocol/
├── main.py                    # Entry point with 3 Kernel actions
├── core/                      # Vendored agent code
│   ├── actions.py             # OSWorld-compatible action types
│   ├── agent.py               # QwenAgent VLM agent
│   ├── agent_loop.py          # Multi-step agent loop
│   ├── browser.py             # Kernel browser adapter
│   ├── fireworks_api.py       # Fireworks REST API (dataset + RFT job)
│   ├── prompts.py             # System prompt builder
│   ├── utils.py               # Image processing utilities
│   └── reward_models/
│       ├── base.py            # Base reward model interface
│       └── webjudge.py        # WebJudge LLM-as-judge scorer
├── agent_auth/                # Agent Auth benchmark
│   ├── actions.py             # FoundInputsAction for form discovery
│   └── config.py              # System prompt configuration
├── tasks.jsonl                # Agent Auth task dataset
├── pyproject.toml             # Python dependencies
├── .env.example               # Environment variable template
└── README.md
```

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `KERNEL_API_KEY` | Kernel API key for browser control |
| `FIREWORKS_API_KEY` | Fireworks API key for VLM inference |
| `OPENAI_API_KEY` | OpenAI API key for WebJudge scoring |

### Action Parameters

#### run-rollout

Uses on-demand browsers (`kernel.browsers.create()`) for one-off testing/debugging jobs.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `task` | string | required | Task instruction |
| `initial_url` | string | required | Starting URL |
| `max_steps` | int | 15 | Max agent steps |
| `model` | string | qwen3-vl-30b-a3b | VLM model |
| `score` | bool | false | Score with WebJudge |

#### run-evaluation

Uses browser pools for scaled parallel evaluation.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `tasks_file` | string | tasks.jsonl | Path to tasks file |
| `pool_name` | string | null | Existing pool name |
| `pool_size` | int | 20 | Ephemeral pool size |
| `max_steps` | int | 15 | Max steps per task |
| `model` | string | qwen3-vl-30b-a3b | VLM model |
| `score_threshold` | float | 0.5 | Pass threshold |
| `max_tasks` | int | null | Limit tasks |

Response includes `results_jsonl` (JSONL content) for use with `create-rft-job`.

#### create-rft-job

Accepts inline JSONL from `run-evaluation`, uploads it to Fireworks, and creates an RFT job via API. Requires a pre-created evaluator on Fireworks.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `account_id` | string | yes | Fireworks account ID (from dashboard) |
| `results_jsonl` | string | yes | JSONL content from `run-evaluation` response |
| `evaluator_id` | string | yes | Fireworks evaluator ID or resource name |
| `base_model` | string | yes | Base model to fine-tune (e.g. accounts/fireworks/models/qwen3-vl-8b-instruct) |
| `output_model` | string | no | Custom output model ID |
| `chunk_size` | int | no | 50 |
| `max_context_length` | int | no | 32768 |
| `batch_size` | int | no | 32768 |
| `epochs` | int | no | 4 |

## Related

- [Eval Protocol](https://evalprotocol.com) - LLM evaluation framework
- [kernel-quickstart](https://github.com/eval-protocol/kernel-quickstart) - Source repo
- [Kernel Docs](https://docs.onkernel.com) - Browser-as-a-service
