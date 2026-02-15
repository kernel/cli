"""
Eval Protocol Template for Kernel.

This template provides VLM browser agent evaluation capabilities using the
Eval Protocol framework with Kernel browser pools.

Actions:
- run-rollout: Execute a single browser rollout for testing/debugging
- run-evaluation: Run parallel evaluation on a task dataset with WebJudge scoring
- create-rft-job: Create an RFT (Reinforcement Fine-Tuning) job from results

Environment Variables:
- KERNEL_API_KEY: Kernel API key (required)
- FIREWORKS_API_KEY: Fireworks API key for VLM inference (required for run-rollout/run-evaluation)
- OPENAI_API_KEY: OpenAI API key for WebJudge scoring (required for scoring)
"""

import asyncio
import base64
import io
import json
import logging
import os
from datetime import datetime
from pathlib import Path
from typing import Optional, TypedDict

import kernel
from kernel import Kernel
from PIL import Image

from core.agent import AgentConfig, QwenAgent
from core.agent_loop import run_agent_loop
from core.browser import KernelBrowserAdapter, acquired_browser
from core.fireworks_api import (
    create_dataset,
    create_rft_job as api_create_rft_job,
    dataset_resource_name,
    evaluator_resource_name,
    upload_dataset,
    wait_for_dataset_ready,
)
from core.prompts import get_system_prompt
from core.reward_models.base import Trajectory
from core.reward_models.webjudge import WebJudge
from agent_auth.actions import AGENT_AUTH_ACTIONS
from agent_auth.config import get_agent_auth_system_prompt

logger = logging.getLogger(__name__)

# Default configuration
DEFAULT_MODEL = "accounts/fireworks/models/qwen3-vl-30b-a3b-thinking"
DEFAULT_MAX_STEPS = 15
DEFAULT_POOL_SIZE = 20
DEFAULT_SCORE_THRESHOLD = 0.5
DEFAULT_BROWSER_TIMEOUT_SECONDS = 3600  # 1 hour
FIREWORKS_BASE_URL = "https://api.fireworks.ai/inference/v1"


def encode_screenshots(images: list[Image.Image]) -> list[str]:
    """Encode PIL Images to base64 strings for JSON storage."""
    encoded = []
    for img in images:
        buffer = io.BytesIO()
        img.save(buffer, format="PNG")
        encoded.append(base64.b64encode(buffer.getvalue()).decode("utf-8"))
    return encoded


def decode_screenshots(encoded: list[str]) -> list[Image.Image]:
    """Decode base64 strings back to PIL Images."""
    images = []
    for b64 in encoded:
        buffer = io.BytesIO(base64.b64decode(b64))
        images.append(Image.open(buffer))
    return images


# =============================================================================
# Input/Output Types
# =============================================================================


class RolloutInput(TypedDict, total=False):
    task: str  # Task instruction (required)
    initial_url: str  # Starting URL (required)
    max_steps: Optional[int]  # Max agent steps (default: 15)
    model: Optional[str]  # VLM model (default: qwen3-vl-30b-a3b-thinking)
    score: Optional[bool]  # Whether to score with WebJudge (default: False)
    system_prompt: Optional[str]  # Custom system prompt (default: agent_auth prompt)


class RolloutOutput(TypedDict):
    screenshots_b64: list[str]  # Base64 PNG screenshots
    action_history: list[str]  # Actions taken
    messages: list[dict]  # Full conversation history with tool_calls
    final_url: str
    steps_completed: int
    termination_reason: str
    score: Optional[float]  # WebJudge score (if score=True)
    score_reason: Optional[str]  # WebJudge reasoning (if score=True)


class EvaluationInput(TypedDict, total=False):
    tasks_file: Optional[str]  # Path to tasks.jsonl (default: bundled tasks.jsonl)
    pool_name: Optional[str]  # Existing browser pool name (creates ephemeral if not provided)
    pool_size: Optional[int]  # Size for ephemeral pool (default: 20)
    max_steps: Optional[int]  # Max steps per rollout (default: 15)
    model: Optional[str]  # VLM model
    score_threshold: Optional[float]  # Pass threshold (default: 0.5)
    max_tasks: Optional[int]  # Limit number of tasks to run


class EvaluationOutput(TypedDict):
    total_tasks: int
    passed: int
    failed: int
    average_score: float
    results: list[dict]  # Per-task results with scores, trajectories
    pool_used: str  # Name of pool used
    results_jsonl: str  # JSONL content (Fireworks RFT dataset format) — pass to create-rft-job


class RftInput(TypedDict, total=False):
    account_id: str  # Fireworks account ID from dashboard (required)
    results_jsonl: str  # JSONL content from run-evaluation's results_jsonl field (required)
    evaluator_id: str  # Fireworks evaluator resource name or ID (required)
    base_model: str  # e.g. "accounts/fireworks/models/qwen3-vl-8b-instruct" (required)
    output_model: Optional[str]  # Custom output model ID (optional)
    chunk_size: Optional[int]  # default: 50
    max_context_length: Optional[int]  # default: 32768
    batch_size: Optional[int]  # default: 32768
    epochs: Optional[int]  # default: 4


class RftOutput(TypedDict):
    job_id: str
    status: str
    dataset_id: str  # Uploaded dataset ID
    dashboard_url: str  # Link to Fireworks dashboard for monitoring


# =============================================================================
# Kernel App
# =============================================================================

app = kernel.App("python-eval-protocol")


@app.action("run-rollout")
async def run_rollout(
    ctx: kernel.KernelContext,
    payload: RolloutInput,
) -> RolloutOutput:
    """
    Execute a single browser rollout for a task.

    This is useful for testing individual tasks or debugging agent behavior.

    Args:
        ctx: Kernel context
        payload: RolloutInput with task, initial_url, and optional configuration

    Returns:
        RolloutOutput with trajectory data and optional WebJudge score
    """
    # Validate required fields
    if not payload.get("task"):
        raise ValueError("task is required")
    if not payload.get("initial_url"):
        raise ValueError("initial_url is required")

    task = payload["task"]
    initial_url = payload["initial_url"]
    max_steps = payload.get("max_steps", DEFAULT_MAX_STEPS)
    model = payload.get("model", DEFAULT_MODEL)
    should_score = payload.get("score", False)
    custom_system_prompt = payload.get("system_prompt")

    # Get API key
    api_key = os.getenv("FIREWORKS_API_KEY")
    if not api_key:
        raise ValueError("FIREWORKS_API_KEY environment variable is required")

    # Create Kernel client
    kernel = Kernel()

    # Determine system prompt
    system_prompt = custom_system_prompt or get_agent_auth_system_prompt()

    # Create agent config
    agent_config = AgentConfig(
        model=model,
        base_url=FIREWORKS_BASE_URL,
        api_key=api_key,
        system_prompt=system_prompt,
        extra_actions=AGENT_AUTH_ACTIONS,
    )

    browser = kernel.browsers.create(stealth=True, timeout_seconds=DEFAULT_BROWSER_TIMEOUT_SECONDS)
    adapter = KernelBrowserAdapter(kernel, browser)
    result = await _run_single_rollout(
        adapter, agent_config, task, initial_url, max_steps
    )

    # Score with WebJudge if requested
    score = None
    score_reason = None
    if should_score:
        openai_key = os.getenv("OPENAI_API_KEY")
        if not openai_key:
            raise ValueError("OPENAI_API_KEY is required for scoring")

        webjudge = WebJudge(api_key=openai_key)
        trajectory = Trajectory(
            task_id="single-rollout",
            task=task,
            action_history=result["action_history"],
            screenshots=decode_screenshots(result["screenshots_b64"]),
            initial_url=initial_url,
            final_url=result["final_url"],
        )
        judge_result = await webjudge.evaluate(trajectory)
        score = judge_result.score
        score_reason = judge_result.reasoning

    return RolloutOutput(
        screenshots_b64=result["screenshots_b64"],
        action_history=result["action_history"],
        messages=result["messages"],
        final_url=result["final_url"],
        steps_completed=result["steps_completed"],
        termination_reason=result["termination_reason"],
        score=score,
        score_reason=score_reason,
    )


async def _run_single_rollout(
    adapter: KernelBrowserAdapter,
    agent_config: AgentConfig,
    task: str,
    initial_url: str,
    max_steps: int,
) -> dict:
    """Helper to run a single rollout with an adapter."""
    # Navigate to initial URL
    initial_screenshot = adapter.navigate(initial_url)

    # Create agent
    agent = QwenAgent(config=agent_config)

    # Run agent loop (in thread pool since it has blocking calls)
    loop_result = await asyncio.to_thread(
        run_agent_loop,
        agent=agent,
        adapter=adapter,
        task=task,
        initial_screenshot=initial_screenshot,
        max_steps=max_steps,
        image_max_size=512,
    )

    # Get final URL
    final_url = adapter.get_current_url()

    # Get message history
    messages = agent.get_messages()

    return {
        "screenshots_b64": encode_screenshots(loop_result.screenshots),
        "action_history": loop_result.action_history,
        "messages": messages,
        "final_url": final_url,
        "steps_completed": loop_result.steps_completed,
        "termination_reason": loop_result.termination_reason,
    }


@app.action("run-evaluation")
async def run_evaluation(
    ctx: kernel.KernelContext,
    payload: EvaluationInput,
) -> EvaluationOutput:
    """
    Run parallel evaluation on a task dataset with WebJudge scoring.

    Uses browser pools for parallel execution.

    Args:
        ctx: Kernel context
        payload: EvaluationInput with optional configuration

    Returns:
        EvaluationOutput with aggregated results
    """
    # Load configuration
    tasks_file = payload.get("tasks_file")
    pool_name = payload.get("pool_name")
    pool_size = payload.get("pool_size", DEFAULT_POOL_SIZE)
    max_steps = payload.get("max_steps", DEFAULT_MAX_STEPS)
    model = payload.get("model", DEFAULT_MODEL)
    score_threshold = payload.get("score_threshold", DEFAULT_SCORE_THRESHOLD)
    max_tasks = payload.get("max_tasks")

    # Get API keys
    fireworks_key = os.getenv("FIREWORKS_API_KEY")
    if not fireworks_key:
        raise ValueError("FIREWORKS_API_KEY environment variable is required")

    openai_key = os.getenv("OPENAI_API_KEY")
    if not openai_key:
        raise ValueError("OPENAI_API_KEY environment variable is required for scoring")

    # Load tasks
    if tasks_file:
        tasks_path = Path(tasks_file)
    else:
        tasks_path = Path(__file__).parent / "tasks.jsonl"

    if not tasks_path.exists():
        raise ValueError(f"Tasks file not found: {tasks_path}")

    tasks = []
    with open(tasks_path) as f:
        for line in f:
            if line.strip():
                tasks.append(json.loads(line))

    if max_tasks:
        tasks = tasks[:max_tasks]

    if not tasks:
        raise ValueError("No tasks found in tasks file")

    print(f"Loaded {len(tasks)} tasks from {tasks_path}")

    # Create Kernel client
    kernel = Kernel()

    # Create or use existing pool
    ephemeral_pool = False
    if not pool_name:
        pool_name = f"eval-ephemeral-{os.getpid()}"
        print(f"Creating ephemeral pool: {pool_name} with {pool_size} browsers")
        kernel.browser_pools.create(
            name=pool_name,
            size=pool_size,
            timeout_seconds=DEFAULT_BROWSER_TIMEOUT_SECONDS,
        )
        ephemeral_pool = True
    else:
        print(f"Using existing pool: {pool_name}")

    # Create WebJudge
    webjudge = WebJudge(api_key=openai_key)

    # Agent config
    agent_config = AgentConfig(
        model=model,
        base_url=FIREWORKS_BASE_URL,
        api_key=fireworks_key,
        system_prompt=get_agent_auth_system_prompt(),
        extra_actions=AGENT_AUTH_ACTIONS,
    )

    # Run tasks with semaphore for concurrency control
    semaphore = asyncio.Semaphore(pool_size)
    results = []

    async def run_task(task_data: dict) -> dict:
        async with semaphore:
            task_id = task_data.get("id", "unknown")
            task = task_data.get("task", "")
            initial_url = task_data.get("initial_url", "")

            try:
                with acquired_browser(kernel, pool_name) as adapter:
                    rollout_result = await _run_single_rollout(
                        adapter, agent_config, task, initial_url, max_steps
                    )

                # Score with WebJudge
                trajectory = Trajectory(
                    task_id=task_id,
                    task=task,
                    action_history=rollout_result["action_history"],
                    screenshots=decode_screenshots(rollout_result["screenshots_b64"]),
                    initial_url=initial_url,
                    final_url=rollout_result["final_url"],
                )
                judge_result = await webjudge.evaluate(trajectory)

                return {
                    "task_id": task_id,
                    "task": task,
                    "initial_url": initial_url,
                    "score": judge_result.score,
                    "passed": judge_result.score >= score_threshold,
                    "reasoning": judge_result.reasoning,
                    "steps_completed": rollout_result["steps_completed"],
                    "termination_reason": rollout_result["termination_reason"],
                    "error": None,
                }

            except Exception as e:
                logger.error(f"Task {task_id} failed: {e}")
                return {
                    "task_id": task_id,
                    "task": task,
                    "initial_url": initial_url,
                    "score": 0.0,
                    "passed": False,
                    "reasoning": "",
                    "steps_completed": 0,
                    "termination_reason": "error",
                    "error": str(e),
                }

    # Run all tasks in parallel
    print(f"Running {len(tasks)} tasks with concurrency {pool_size}...")
    task_futures = [run_task(t) for t in tasks]
    results = await asyncio.gather(*task_futures)

    # Clean up ephemeral pool
    if ephemeral_pool:
        print(f"Deleting ephemeral pool: {pool_name}")
        try:
            kernel.browser_pools.delete(pool_name, force=True)
        except Exception as e:
            logger.warning(f"Failed to delete ephemeral pool: {e}")

    # Aggregate results
    passed = sum(1 for r in results if r["passed"])
    failed = len(results) - passed
    total_score = sum(r["score"] for r in results)
    avg_score = total_score / len(results) if results else 0.0

    print(f"Evaluation complete: {passed}/{len(results)} passed ({avg_score:.2%} avg score)")

    # Build JSONL content in Fireworks RFT dataset format
    system_prompt = get_agent_auth_system_prompt()
    jsonl_lines = []
    for r in results:
        jsonl_lines.append(json.dumps({
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": r["task"]},
            ],
            "score": r["score"],
        }))
    results_jsonl = "\n".join(jsonl_lines) + "\n"
    print(f"Generated {len(jsonl_lines)} JSONL records for RFT dataset")

    return EvaluationOutput(
        total_tasks=len(results),
        passed=passed,
        failed=failed,
        average_score=avg_score,
        results=results,
        pool_used=pool_name,
        results_jsonl=results_jsonl,
    )


@app.action("create-rft-job")
async def create_rft_job(
    ctx: kernel.KernelContext,
    payload: RftInput,
) -> RftOutput:
    """
    Create an RFT (Reinforcement Fine-Tuning) job from evaluation results.

    Accepts inline JSONL content (from run-evaluation's results_jsonl output),
    uploads it to Fireworks as a dataset, and creates the RFT job via API.
    Requires a pre-created evaluator on Fireworks (see README).

    Args:
        ctx: Kernel context
        payload: RftInput with account_id, results_jsonl, evaluator_id, base_model, and optional config

    Returns:
        RftOutput with job ID, status, dataset_id, and dashboard URL
    """
    if not payload.get("account_id"):
        raise ValueError("account_id is required (get from Fireworks dashboard)")
    if not payload.get("results_jsonl"):
        raise ValueError("results_jsonl is required (JSONL content from run-evaluation)")
    if not payload.get("evaluator_id"):
        raise ValueError("evaluator_id is required (Fireworks evaluator ID or resource name)")
    if not payload.get("base_model"):
        raise ValueError("base_model is required")

    account_id = payload["account_id"]
    results_jsonl = payload["results_jsonl"]
    evaluator_id = payload["evaluator_id"]
    base_model = payload["base_model"]
    output_model = payload.get("output_model")
    chunk_size = payload.get("chunk_size", 50)
    max_context_length = payload.get("max_context_length", 32768)
    batch_size = payload.get("batch_size", 32768)
    epochs = payload.get("epochs", 4)

    api_key = os.getenv("FIREWORKS_API_KEY")
    if not api_key:
        raise ValueError("FIREWORKS_API_KEY environment variable is required")

    # Write inline JSONL to a temp file for upload
    import tempfile
    tmp = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False)
    tmp.write(results_jsonl)
    tmp.close()
    results_path = Path(tmp.name)

    # Run API calls in thread pool to avoid blocking
    def _do_create() -> dict:
        try:
            timestamp = datetime.utcnow().strftime("%Y%m%d-%H%M%S")
            dataset_id = f"eval-dataset-{timestamp}"
            create_dataset(account_id, dataset_id, api_key)
            print(f"Created dataset: {dataset_id}")
            upload_dataset(account_id, dataset_id, results_path, api_key)
            print("Uploaded results file")
            wait_for_dataset_ready(account_id, dataset_id, api_key)
            print("Dataset ready")

            dataset_resource = dataset_resource_name(account_id, dataset_id)
            evaluator_resource = evaluator_resource_name(account_id, evaluator_id)
            training_config = {
                "baseModel": base_model,
                "maxContextLength": max_context_length,
                "batchSize": batch_size,
                "epochs": epochs,
            }
            if output_model:
                training_config["outputModel"] = output_model

            job = api_create_rft_job(
                account_id,
                api_key,
                dataset_resource,
                evaluator_resource,
                training_config,
                display_name=f"eval-rft-{timestamp}",
                chunk_size=chunk_size,
            )
            return {"job": job, "dataset_id": dataset_id}
        finally:
            results_path.unlink(missing_ok=True)

    result = await asyncio.to_thread(_do_create)
    job = result["job"]
    dataset_id = result["dataset_id"]

    job_name = job.get("name", "")
    job_id = job_name.split("/")[-1] if job_name else "unknown"
    state = job.get("state", "UNKNOWN")
    dashboard_url = f"https://app.fireworks.ai/dashboard/fine-tuning/jobs/{job_id}" if job_id != "unknown" else "https://app.fireworks.ai/dashboard/fine-tuning"

    print(f"RFT job created: {job_id}")
    print(f"Dashboard: {dashboard_url}")

    return RftOutput(
        job_id=job_id,
        status=state.lower() if state else "created",
        dataset_id=dataset_id,
        dashboard_url=dashboard_url,
    )
