"""
Moondream computer-use agent loop.
"""

from __future__ import annotations

import asyncio
import json
import re
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Optional, Tuple

from kernel import Kernel

from moondream import MoondreamClient
from tools import (
    ComputerAction,
    ComputerTool,
    COORDINATE_SCALE,
    DEFAULT_SCREEN_SIZE,
    ScreenSize,
)


URL_RE = re.compile(r"https?://[^\s)]+", re.IGNORECASE)


@dataclass
class AgentOptions:
    max_retries: int = 3
    retry_delay_ms: int = 1000
    strict: bool = False


@dataclass
class StepLog:
    step: int
    action: str
    status: str
    detail: str
    output: Optional[str] = None


async def run_agent(
    *,
    query: Optional[str],
    steps: Optional[List[Dict[str, Any]]],
    moondream: MoondreamClient,
    kernel: Kernel,
    session_id: str,
    options: AgentOptions,
) -> Dict[str, Any]:
    computer = ComputerTool(kernel, session_id)

    parsed_steps = steps or parse_steps(query or "")
    if not parsed_steps:
        raise ValueError("No steps could be derived from the query. Provide steps or a query.")

    logs: List[StepLog] = []
    answers: List[str] = []
    last_screenshot: Optional[str] = None
    error: Optional[str] = None

    for index, step in enumerate(parsed_steps, start=1):
        action = str(step.get("action", "")).strip().lower()
        if not action:
            logs.append(StepLog(index, "unknown", "failed", "Missing action"))
            if options.strict:
                error = "Missing action in step"
                break
            continue

        try:
            if step.get("pre_wait_ms"):
                await asyncio.sleep(float(step["pre_wait_ms"]) / 1000)

            if action in {"open_web_browser", "open"}:
                result = await computer.execute_action(ComputerAction.OPEN_WEB_BROWSER, {})
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), "Opened browser"))

            elif action == "navigate":
                url = step.get("url") or _find_url(query or "")
                if not url:
                    raise ValueError("navigate requires url")
                result = await computer.execute_action(ComputerAction.NAVIGATE, {"url": url})
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), f"Navigated to {url}"))

            elif action == "go_back":
                result = await computer.execute_action(ComputerAction.GO_BACK, {})
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), "Went back"))

            elif action == "go_forward":
                result = await computer.execute_action(ComputerAction.GO_FORWARD, {})
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), "Went forward"))

            elif action == "search":
                result = await computer.execute_action(ComputerAction.SEARCH, {})
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), "Focused address bar"))

            elif action == "wait":
                seconds = float(step.get("seconds", 1))
                await asyncio.sleep(seconds)
                logs.append(StepLog(index, action, "success", f"Waited {seconds:.2f}s"))

            elif action == "key":
                keys = step.get("keys")
                if not keys:
                    raise ValueError("key action requires keys")
                result = await computer.execute_action(ComputerAction.KEY_COMBINATION, {"keys": keys})
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), f"Pressed {keys}"))

            elif action == "scroll":
                direction = step.get("direction", "down")
                magnitude = step.get("magnitude")
                if "x" in step and "y" in step:
                    x_norm, y_norm = normalize_point(
                        float(step["x"]),
                        float(step["y"]),
                        computer.screen_size,
                    )
                    args: Dict[str, Any] = {
                        "x": x_norm,
                        "y": y_norm,
                        "direction": direction,
                    }
                    if magnitude is not None:
                        args["magnitude"] = int(magnitude)
                    result = await computer.execute_action(ComputerAction.SCROLL_AT, args)
                else:
                    args = {"direction": direction}
                    if magnitude is not None:
                        args["magnitude"] = int(magnitude)
                    result = await computer.execute_action(ComputerAction.SCROLL_DOCUMENT, args)
                last_screenshot = _update_screenshot(result, last_screenshot)
                logs.append(StepLog(index, action, _status(result), f"Scrolled {direction}"))

            elif action in {"click", "type"}:
                target = step.get("target")
                retries = int(step.get("retries", options.max_retries))
                delay_ms = int(step.get("retry_delay_ms", options.retry_delay_ms))

                coords = await _resolve_target_coords(
                    step,
                    target,
                    moondream,
                    computer,
                    last_screenshot,
                    retries,
                    delay_ms,
                )

                if not coords:
                    raise ValueError(f"Unable to locate target: {target}")

                x_norm, y_norm = coords
                if action == "click":
                    result = await computer.execute_action(
                        ComputerAction.CLICK_AT,
                        {"x": x_norm, "y": y_norm},
                    )
                    last_screenshot = _update_screenshot(result, last_screenshot)
                    logs.append(StepLog(index, action, _status(result), f"Clicked {target}"))
                else:
                    text = step.get("text")
                    if text is None:
                        raise ValueError("type action requires text")
                    result = await computer.execute_action(
                        ComputerAction.TYPE_TEXT_AT,
                        {
                            "x": x_norm,
                            "y": y_norm,
                            "text": str(text),
                            "press_enter": bool(step.get("press_enter", False)),
                            "clear_before_typing": bool(step.get("clear_before_typing", True)),
                        },
                    )
                    last_screenshot = _update_screenshot(result, last_screenshot)
                    logs.append(
                        StepLog(index, action, _status(result), f"Typed into {target}")
                    )

            elif action == "query":
                question = step.get("question") or query
                if not question:
                    raise ValueError("query action requires question")
                screenshot = await _ensure_screenshot(computer, last_screenshot)
                last_screenshot = screenshot
                answer = await moondream.query(screenshot, str(question))
                answers.append(answer)
                logs.append(StepLog(index, action, "success", "Answered question", output=answer))

            elif action == "caption":
                length = step.get("length", "normal")
                screenshot = await _ensure_screenshot(computer, last_screenshot)
                last_screenshot = screenshot
                caption = await moondream.caption(screenshot, str(length))
                answers.append(caption)
                logs.append(StepLog(index, action, "success", "Generated caption", output=caption))

            else:
                raise ValueError(f"Unknown action: {action}")

        except Exception as exc:
            message = str(exc)
            logs.append(StepLog(index, action, "failed", message))
            error = message
            if options.strict:
                break

    summary = f"Completed {sum(1 for log in logs if log.status == 'success')}/{len(logs)} steps"
    result_payload = {
        "summary": summary,
        "steps": [log.__dict__ for log in logs],
        "answers": answers,
    }

    return {
        "final_response": json.dumps(result_payload, indent=2),
        "error": error,
    }


def parse_steps(query: str) -> List[Dict[str, Any]]:
    query = query.strip()
    if not query:
        return []

    if query.startswith("{") or query.startswith("["):
        try:
            data = json.loads(query)
            if isinstance(data, list):
                return data
            if isinstance(data, dict) and isinstance(data.get("steps"), list):
                return data["steps"]
        except json.JSONDecodeError:
            pass

    steps: List[Dict[str, Any]] = []
    url = _find_url(query)
    if url:
        steps.append({"action": "navigate", "url": url})

    question = _strip_url_and_navigation(query)
    wants_caption = any(term in query.lower() for term in ["describe", "caption"])

    if wants_caption:
        steps.append({"action": "caption"})
    elif question:
        steps.append({"action": "query", "question": question})
    elif url:
        steps.append({"action": "caption"})
    else:
        steps.append({"action": "query", "question": query})

    return steps


def _find_url(query: str) -> Optional[str]:
    match = URL_RE.search(query)
    return match.group(0) if match else None


def _strip_url_and_navigation(query: str) -> str:
    cleaned = URL_RE.sub("", query)
    cleaned = re.sub(r"\b(navigate|open|go|visit)\b", "", cleaned, flags=re.IGNORECASE)
    cleaned = cleaned.replace("to", " ")
    cleaned = re.sub(r"\s+", " ", cleaned).strip(" ,.;:-")
    return cleaned


def normalize_point(x: float, y: float, screen_size: Optional[ScreenSize] = None) -> Tuple[int, int]:
    if 0 <= x <= 1 and 0 <= y <= 1:
        return int(x * COORDINATE_SCALE), int(y * COORDINATE_SCALE)
    width = screen_size.width if screen_size else DEFAULT_SCREEN_SIZE.width
    height = screen_size.height if screen_size else DEFAULT_SCREEN_SIZE.height
    return int((x / width) * COORDINATE_SCALE), int((y / height) * COORDINATE_SCALE)


def _status(result: Any) -> str:
    return "failed" if result and getattr(result, "error", None) else "success"


def _update_screenshot(result: Any, last_screenshot: Optional[str]) -> Optional[str]:
    if result and getattr(result, "base64_image", None):
        return result.base64_image
    return last_screenshot


async def _ensure_screenshot(computer: ComputerTool, last_screenshot: Optional[str]) -> str:
    if last_screenshot:
        return last_screenshot
    result = await computer.screenshot()
    if result.error or not result.base64_image:
        raise RuntimeError(result.error or "Failed to capture screenshot")
    return result.base64_image


async def _resolve_target_coords(
    step: Dict[str, Any],
    target: Optional[str],
    moondream: MoondreamClient,
    computer: ComputerTool,
    last_screenshot: Optional[str],
    retries: int,
    delay_ms: int,
) -> Optional[Tuple[int, int]]:
    if "x" in step and "y" in step:
        return normalize_point(float(step["x"]), float(step["y"]), computer.screen_size)

    if not target:
        return None

    attempts = max(1, retries)
    for attempt in range(attempts):
        screenshot = await _ensure_screenshot(computer, last_screenshot)
        point = await moondream.point(screenshot, str(target))
        if point:
            return normalize_point(point.x, point.y, computer.screen_size)
        if attempt < attempts - 1:
            await asyncio.sleep(delay_ms / 1000)
            last_screenshot = None

    return None
