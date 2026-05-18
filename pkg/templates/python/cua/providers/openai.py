"""OpenAI CUA provider adapter using the Responses API."""

from __future__ import annotations

import asyncio
import base64
import json
import os
from datetime import datetime

import httpx

from . import CuaProvider, TaskOptions, TaskResult

KEYSYM_MAP = {
    "ENTER": "Return", "Enter": "Return", "RETURN": "Return",
    "BACKSPACE": "BackSpace", "Backspace": "BackSpace",
    "DELETE": "Delete", "TAB": "Tab", "ESCAPE": "Escape", "Escape": "Escape",
    "SPACE": "space", "Space": "space",
    "UP": "Up", "DOWN": "Down", "LEFT": "Left", "RIGHT": "Right",
    "HOME": "Home", "END": "End",
    "PAGEUP": "Prior", "PAGE_UP": "Prior", "PageUp": "Prior",
    "PAGEDOWN": "Next", "PAGE_DOWN": "Next", "PageDown": "Next",
    "CTRL": "Control_L", "Ctrl": "Control_L", "CONTROL": "Control_L", "Control": "Control_L",
    "ALT": "Alt_L", "Alt": "Alt_L",
    "SHIFT": "Shift_L", "Shift": "Shift_L",
    "META": "Super_L", "Meta": "Super_L", "CMD": "Super_L", "COMMAND": "Super_L",
}

MODIFIER_KEYSYMS = {
    "Control_L", "Control_R", "Alt_L", "Alt_R",
    "Shift_L", "Shift_R", "Super_L", "Super_R",
}


def _translate_keys(keys: list[str]) -> list[str]:
    return [KEYSYM_MAP.get(k, k) for k in keys]


def _expand_and_translate(
    keys: list[str], hold_keys: list[str],
) -> tuple[list[str], list[str]]:
    expanded = []
    for raw in keys:
        for part in (raw.split("+") if "+" in raw else [raw]):
            trimmed = part.strip()
            if trimmed:
                expanded.append(trimmed)

    translated = _translate_keys(expanded)
    translated_hold = _translate_keys(hold_keys)

    hold_from_keys = [k for k in translated if k in MODIFIER_KEYSYMS]
    primary = [k for k in translated if k not in MODIFIER_KEYSYMS]

    if not primary:
        return translated, translated_hold

    merged = list(dict.fromkeys(translated_hold + hold_from_keys))
    return primary, merged


def _translate_action(action: dict) -> list[dict]:
    action_type = action.get("type", "")

    if action_type == "click":
        button = action.get("button", "left")
        if button == "back":
            return [{"type": "press_key", "press_key": {"keys": ["Left"], "hold_keys": ["Alt_L"]}}]
        if button == "forward":
            return [{"type": "press_key", "press_key": {"keys": ["Right"], "hold_keys": ["Alt_L"]}}]
        if button == "wheel":
            return [{"type": "scroll", "scroll": {
                "x": action.get("x", 0), "y": action.get("y", 0),
                "delta_x": action.get("scroll_x", 0), "delta_y": action.get("scroll_y", 0),
            }}]
        btn = "left"
        if isinstance(button, int):
            btn = {2: "middle", 3: "right"}.get(button, "left")
        elif isinstance(button, str):
            btn = button
        return [{"type": "click_mouse", "click_mouse": {"x": action.get("x", 0), "y": action.get("y", 0), "button": btn}}]

    if action_type == "double_click":
        return [{"type": "click_mouse", "click_mouse": {"x": action.get("x", 0), "y": action.get("y", 0), "num_clicks": 2}}]

    if action_type == "type":
        return [{"type": "type_text", "type_text": {"text": action.get("text", "")}}]

    if action_type == "keypress":
        primary, hold = _expand_and_translate(action.get("keys", []), action.get("hold_keys", []))
        result: dict = {"type": "press_key", "press_key": {"keys": primary}}
        if hold:
            result["press_key"]["hold_keys"] = hold
        return [result]

    if action_type == "scroll":
        return [{"type": "scroll", "scroll": {
            "x": action.get("x", 0), "y": action.get("y", 0),
            "delta_x": action.get("scroll_x", 0), "delta_y": action.get("scroll_y", 0),
        }}]

    if action_type == "move":
        return [{"type": "move_mouse", "move_mouse": {"x": action.get("x", 0), "y": action.get("y", 0)}}]

    if action_type == "drag":
        path = action.get("path", [])
        points = []
        for p in path:
            if isinstance(p, dict):
                points.append([p["x"], p["y"]])
            elif isinstance(p, (list, tuple)) and len(p) >= 2:
                points.append([p[0], p[1]])
        if len(points) < 2:
            raise ValueError("drag requires at least 2 path points")
        return [{"type": "drag_mouse", "drag_mouse": {"path": points}}]

    if action_type == "wait":
        return [{"type": "sleep", "sleep": {"duration_ms": action.get("ms", 1000)}}]

    if action_type == "goto":
        url = action.get("url", "")
        return [
            {"type": "press_key", "press_key": {"keys": ["l"], "hold_keys": ["Control_L"]}},
            {"type": "sleep", "sleep": {"duration_ms": 200}},
            {"type": "press_key", "press_key": {"keys": ["a"], "hold_keys": ["Control_L"]}},
            {"type": "type_text", "type_text": {"text": url}},
            {"type": "press_key", "press_key": {"keys": ["Return"]}},
        ]

    if action_type == "back":
        return [{"type": "press_key", "press_key": {"keys": ["Left"], "hold_keys": ["Alt_L"]}}]

    if action_type == "screenshot":
        return []

    raise ValueError(f"Unknown CUA action: {action_type}")


async def _create_response(api_key: str, **kwargs) -> dict:
    """Call the OpenAI Responses API with retry."""
    async with httpx.AsyncClient(timeout=120) as client:
        for attempt in range(4):
            try:
                resp = await client.post(
                    "https://api.openai.com/v1/responses",
                    headers={
                        "Authorization": f"Bearer {api_key}",
                        "Content-Type": "application/json",
                    },
                    json=kwargs,
                )
                resp.raise_for_status()
                return resp.json()
            except httpx.HTTPStatusError as exc:
                if exc.response.status_code >= 500 and attempt < 3:
                    await asyncio.sleep(2 ** attempt)
                    continue
                raise
    raise RuntimeError("Max retries exceeded")


class OpenAIProvider:
    name = "openai"

    def __init__(self) -> None:
        self._api_key = os.environ.get("OPENAI_API_KEY", "")

    def is_configured(self) -> bool:
        return len(self._api_key) > 0

    async def run_task(self, options: TaskOptions) -> TaskResult:
        computer = options.kernel.browsers.computer

        # Navigate to starting page
        goto_actions = _translate_action({"type": "goto", "url": "https://duckduckgo.com"})
        await asyncio.to_thread(
            computer.batch, options.session_id, actions=goto_actions,
        )

        input_items = [
            {
                "role": "system",
                "content": (
                    f"Current date: {datetime.now().isoformat()}. "
                    "For long pages, prefer PageUp/PageDown style scrolling over repeated "
                    "mouse-wheel scrolling. Use wheel scrolling mainly for small adjustments."
                ),
            },
            {"type": "message", "role": "user", "content": [{"type": "input_text", "text": options.query}]},
        ]
        items: list[dict] = []

        for _turn in range(50):
            response = await _create_response(
                self._api_key,
                model=options.model or "gpt-5.4",
                input=input_items + items,
                tools=[{"type": "computer"}],
                truncation="auto",
                reasoning={"effort": "low", "summary": "concise"},
            )

            output = response.get("output", [])
            if not output:
                raise RuntimeError("No output from model")

            for item in output:
                items.append(item)

                if item.get("type") == "computer_call":
                    action_list = item.get("actions") or ([item["action"]] if "action" in item else [])

                    batch: list[dict] = []
                    for a in action_list:
                        batch.extend(_translate_action(a))
                    if batch:
                        await asyncio.to_thread(
                            computer.batch, options.session_id, actions=batch,
                        )

                    # Safety checks
                    for check in item.get("pending_safety_checks", []):
                        print(f"Safety check: {check.get('message', '')}")

                    await asyncio.sleep(0.3)
                    resp = await asyncio.to_thread(
                        computer.capture_screenshot, options.session_id,
                    )
                    screenshot = base64.b64encode(resp.read()).decode()

                    items.append({
                        "type": "computer_call_output",
                        "call_id": item["call_id"],
                        "acknowledged_safety_checks": item.get("pending_safety_checks", []),
                        "output": {
                            "type": "computer_screenshot",
                            "image_url": f"data:image/png;base64,{screenshot}",
                        },
                    })

            # Check for final assistant message
            last = output[-1] if output else {}
            if last.get("role") == "assistant":
                content = last.get("content", [])
                texts = [c.get("text", "") for c in content if isinstance(c, dict) and "text" in c]
                return TaskResult(result=" ".join(texts) or "(no response)", provider=self.name)

        return TaskResult(result="(max turns reached)", provider=self.name)
