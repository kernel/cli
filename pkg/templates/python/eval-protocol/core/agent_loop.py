"""
Shared agent loop for running VLM agents.

This module provides a reusable agent loop that can be used by both
interactive scripts (run_agent.py) and batch evaluation (evaluate.py).
"""

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Callable, Protocol

from PIL import Image

from .actions import Action
from .utils import resize_image

if TYPE_CHECKING:
    from .agent import QwenAgent
    from .browser import KernelBrowserAdapter


@dataclass
class StepResult:
    """Result of a single agent step."""

    step: int
    action: Action | None
    action_desc: str
    screenshot_after: Image.Image | None
    predict_time: float
    exec_time: float
    total_time: float
    is_terminal: bool = False
    error: str | None = None


@dataclass
class AgentLoopResult:
    """Result of running the full agent loop."""

    screenshots: list[Image.Image]
    action_history: list[str]
    step_results: list[StepResult]
    termination_reason: str  # "max_steps", "terminal_action", "parse_failure", "action_failure", "error"
    steps_completed: int
    error: str | None = None
    terminal_action: str | None = None


class StepCallback(Protocol):
    """Protocol for step callbacks."""

    def __call__(
        self,
        step: int,
        action: Action | None,
        action_desc: str,
        screenshot: Image.Image,
        result: StepResult,
    ) -> None:
        """Called after each step completes."""
        ...


def run_agent_loop(
    agent: "QwenAgent",
    adapter: "KernelBrowserAdapter",
    task: str,
    initial_screenshot: Image.Image,
    max_steps: int,
    *,
    image_max_size: int | None = None,
    on_step_start: Callable[[int, Image.Image], None] | None = None,
    on_step_complete: Callable[[int, StepResult], None] | None = None,
    on_action_overlay: Callable[[int, Action, Image.Image], None] | None = None,
) -> AgentLoopResult:
    """
    Run the agent loop for a given task.

    This is the core loop shared between run_agent.py and evaluate.py.
    It handles:
    - Getting predictions from the agent
    - Executing actions on the browser
    - Capturing screenshots
    - Detecting terminal actions
    - Tracking timing and results

    Args:
        agent: The QwenAgent instance (should already be reset if needed)
        adapter: The browser adapter
        task: The task instruction
        initial_screenshot: Screenshot after initial navigation
        max_steps: Maximum number of steps to run
        image_max_size: If set, resize screenshots to this max dimension
        on_step_start: Optional callback called at start of each step
        on_step_complete: Optional callback called after each step
        on_action_overlay: Optional callback for action overlay images (e.g., click visualization)

    Returns:
        AgentLoopResult with all screenshots, actions, and metadata
    """
    screenshots: list[Image.Image] = [initial_screenshot]
    action_history: list[str] = []
    step_results: list[StepResult] = []
    termination_reason = "max_steps"
    error: str | None = None
    terminal_action: str | None = None

    for step in range(1, max_steps + 1):
        t_step_start = time.perf_counter()

        # Get the latest screenshot for prediction
        screenshot = screenshots[-1]

        # Optionally resize for the agent
        screenshot_for_agent = screenshot
        if image_max_size:
            screenshot_for_agent = resize_image(screenshot, max_size=image_max_size)

        # Callback: step starting
        if on_step_start:
            on_step_start(step, screenshot_for_agent)

        # Get agent prediction
        t_predict_start = time.perf_counter()
        try:
            action = agent.predict(task, screenshot_for_agent)
        except Exception as e:
            error = f"Agent error at step {step}: {e}"
            step_result = StepResult(
                step=step,
                action=None,
                action_desc="",
                screenshot_after=None,
                predict_time=time.perf_counter() - t_predict_start,
                exec_time=0.0,
                total_time=time.perf_counter() - t_step_start,
                error=error,
            )
            step_results.append(step_result)
            if on_step_complete:
                on_step_complete(step, step_result)
            termination_reason = "error"
            break

        t_predict = time.perf_counter() - t_predict_start

        # Handle parse failure
        if action is None:
            error = f"Failed to parse action at step {step}"
            step_result = StepResult(
                step=step,
                action=None,
                action_desc="",
                screenshot_after=None,
                predict_time=t_predict,
                exec_time=0.0,
                total_time=time.perf_counter() - t_step_start,
                error=error,
            )
            step_results.append(step_result)
            if on_step_complete:
                on_step_complete(step, step_result)
            termination_reason = "parse_failure"
            break

        # Format action description
        action_desc = action.to_description()
        if action.model_description:
            action_desc = f"{action.model_description} ({action_desc})"
        action_history.append(action_desc)

        # Generate overlay image if callback provided
        if on_action_overlay:
            overlay = action.overlay_on_image(screenshot)
            if overlay is not None:
                on_action_overlay(step, action, overlay)

        # Check for terminal action
        if getattr(action, "is_terminal", False):
            terminal_action = getattr(action, "action_type", "unknown")
            step_result = StepResult(
                step=step,
                action=action,
                action_desc=action_desc,
                screenshot_after=screenshot,  # Terminal actions don't change state
                predict_time=t_predict,
                exec_time=0.0,
                total_time=time.perf_counter() - t_step_start,
                is_terminal=True,
            )
            step_results.append(step_result)
            screenshots.append(screenshot)  # Append current screenshot for consistency
            if on_step_complete:
                on_step_complete(step, step_result)
            termination_reason = "terminal_action"
            break

        # Execute action
        t_exec_start = time.perf_counter()
        try:
            baseline = adapter.capture_screenshot()
            should_continue = adapter.execute_action(action)

            if not should_continue:
                terminal_action = getattr(action, "action_type", "unknown")
                step_result = StepResult(
                    step=step,
                    action=action,
                    action_desc=action_desc,
                    screenshot_after=None,
                    predict_time=t_predict,
                    exec_time=time.perf_counter() - t_exec_start,
                    total_time=time.perf_counter() - t_step_start,
                )
                step_results.append(step_result)
                if on_step_complete:
                    on_step_complete(step, step_result)
                termination_reason = "action_failure"
                break

            # Wait for screen to settle
            if not getattr(action, "skip_screen_settle", False):
                adapter.wait_for_screen_settle(baseline=baseline)

            # Capture new screenshot
            new_screenshot = adapter.capture_screenshot()

            # Optionally resize for storage
            if image_max_size:
                new_screenshot = resize_image(new_screenshot, max_size=image_max_size)

            screenshots.append(new_screenshot.copy())

        except Exception as e:
            error = f"Execution error at step {step}: {e}"
            step_result = StepResult(
                step=step,
                action=action,
                action_desc=action_desc,
                screenshot_after=None,
                predict_time=t_predict,
                exec_time=time.perf_counter() - t_exec_start,
                total_time=time.perf_counter() - t_step_start,
                error=error,
            )
            step_results.append(step_result)
            if on_step_complete:
                on_step_complete(step, step_result)
            termination_reason = "error"
            break

        t_exec = time.perf_counter() - t_exec_start
        t_total = time.perf_counter() - t_step_start

        step_result = StepResult(
            step=step,
            action=action,
            action_desc=action_desc,
            screenshot_after=new_screenshot,
            predict_time=t_predict,
            exec_time=t_exec,
            total_time=t_total,
        )
        step_results.append(step_result)

        if on_step_complete:
            on_step_complete(step, step_result)

    return AgentLoopResult(
        screenshots=screenshots,
        action_history=action_history,
        step_results=step_results,
        termination_reason=termination_reason,
        steps_completed=len(step_results),
        error=error,
        terminal_action=terminal_action,
    )
