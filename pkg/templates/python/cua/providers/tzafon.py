"""Tzafon CUA provider adapter using the Lightcone Responses API.

Uses function tools (click, type, key, scroll, drag, done) with a
normalised 0-999 coordinate grid.

@see https://docs.lightcone.ai
"""

from __future__ import annotations

import asyncio
import base64
import json
import os
from typing import Any

from tzafon import Lightcone

from . import CuaProvider, TaskOptions, TaskResult

DEFAULT_MODEL = "tzafon.northstar-cua-fast"

INSTRUCTIONS = (
    "Use a mouse and keyboard to interact with a Chromium browser and take screenshots.\n"
    "* Chromium is already open on a Kernel cloud browser. If a startup wizard appears, ignore it.\n"
    "* The screen's coordinate space is a 0-999 grid.\n"
    "* To navigate to a URL, use point_and_type on the address bar, or key('ctrl+l') to focus it first.\n"
    "* Some pages may take time to load. Wait and take successive screenshots to confirm the result.\n"
    "* Whenever you click on an element, consult the screenshot to determine coordinates first.\n"
    "* Click buttons, links, and icons in the center of the element, not on edges.\n"
    "* If a click didn't work, try adjusting the coordinates slightly.\n"
    "* For full-page scrolling, prefer key('PageDown') / key('PageUp') over the scroll tool.\n"
    "* After each action, evaluate the screenshot to confirm it succeeded before moving on.\n"
    "* When the task is complete, call done() with a summary of what you found or accomplished.\n"
)

TOOLS = [
    {
        "type": "function", "name": "click",
        "description": "Single click at (x, y) in 0-999 grid.",
        "parameters": {
            "type": "object",
            "properties": {
                "x": {"type": "integer", "description": "X in 0-999 grid"},
                "y": {"type": "integer", "description": "Y in 0-999 grid"},
                "button": {"type": "string", "enum": ["left", "right"]},
            },
            "required": ["x", "y"],
        },
    },
    {
        "type": "function", "name": "double_click",
        "description": "Double click at (x, y) in 0-999 grid.",
        "parameters": {
            "type": "object",
            "properties": {
                "x": {"type": "integer", "description": "X in 0-999 grid"},
                "y": {"type": "integer", "description": "Y in 0-999 grid"},
            },
            "required": ["x", "y"],
        },
    },
    {
        "type": "function", "name": "point_and_type",
        "description": "Click at position then type text. For input fields, search bars, address bars.",
        "parameters": {
            "type": "object",
            "properties": {
                "x": {"type": "integer", "description": "X in 0-999 grid"},
                "y": {"type": "integer", "description": "Y in 0-999 grid"},
                "text": {"type": "string"},
                "press_enter": {"type": "boolean", "description": "Press Enter after typing"},
            },
            "required": ["x", "y", "text"],
        },
    },
    {
        "type": "function", "name": "key",
        "description": "Press key combo (e.g. 'Enter', 'ctrl+a', 'Tab').",
        "parameters": {
            "type": "object",
            "properties": {"keys": {"type": "string"}},
            "required": ["keys"],
        },
    },
    {
        "type": "function", "name": "scroll",
        "description": "Scroll at (x, y) in 0-999 grid. Positive dy = down, negative = up.",
        "parameters": {
            "type": "object",
            "properties": {
                "x": {"type": "integer", "description": "X in 0-999 grid"},
                "y": {"type": "integer", "description": "Y in 0-999 grid"},
                "dy": {"type": "integer", "description": "Scroll notches. 3=down, -3=up."},
            },
            "required": ["x", "y", "dy"],
        },
    },
    {
        "type": "function", "name": "drag",
        "description": "Drag from (x1, y1) to (x2, y2) in 0-999 grid.",
        "parameters": {
            "type": "object",
            "properties": {
                "x1": {"type": "integer", "description": "Start X in 0-999 grid"},
                "y1": {"type": "integer", "description": "Start Y in 0-999 grid"},
                "x2": {"type": "integer", "description": "End X in 0-999 grid"},
                "y2": {"type": "integer", "description": "End Y in 0-999 grid"},
            },
            "required": ["x1", "y1", "x2", "y2"],
        },
    },
    {
        "type": "function", "name": "done",
        "description": "Task complete. Report findings.",
        "parameters": {
            "type": "object",
            "properties": {"result": {"type": "string"}},
            "required": ["result"],
        },
    },
]

KEY_MAP: dict[str, str] = {
    "return": "Return", "enter": "Return",
    "space": "space", "tab": "Tab",
    "backspace": "BackSpace", "delete": "Delete",
    "escape": "Escape", "esc": "Escape", "insert": "Insert",
    "up": "Up", "down": "Down", "left": "Left", "right": "Right",
    "home": "Home", "end": "End",
    "pageup": "Page_Up", "page_up": "Page_Up",
    "pagedown": "Page_Down", "page_down": "Page_Down",
    **{f"f{i}": f"F{i}" for i in range(1, 13)},
}

MODIFIER_MAP: dict[str, str] = {
    "ctrl": "ctrl", "control": "ctrl",
    "alt": "alt", "shift": "shift",
    "meta": "super", "cmd": "super", "command": "super", "win": "super",
}


def _map_key(key_combo: str) -> str:
    parts = key_combo.split("+") if "+" in key_combo else [key_combo]
    mapped = []
    for part in parts:
        k = part.strip().lower()
        mapped.append(MODIFIER_MAP.get(k) or KEY_MAP.get(k, part.strip()))
    return "+".join(mapped)


def _coord(val: Any) -> int:
    if val is None:
        return 0
    s = str(val)
    if "," in s:
        s = s.split(",")[0].strip()
    return int(float(s))


class TzafonProvider:
    name = "tzafon"

    def __init__(self) -> None:
        self._api_key = os.environ.get("TZAFON_API_KEY", "")

    def is_configured(self) -> bool:
        return len(self._api_key) > 0

    async def run_task(self, options: TaskOptions) -> TaskResult:
        model = options.model or DEFAULT_MODEL
        tzafon = Lightcone(api_key=self._api_key)
        computer = options.kernel.browsers.computer
        width = options.viewport_width
        height = options.viewport_height
        max_steps = 50

        def scale(x: Any, y: Any) -> tuple[int, int]:
            cx, cy = _coord(x), _coord(y)
            px = max(0, min(cx * (width - 1) // 999, width - 1))
            py = max(0, min(cy * (height - 1) // 999, height - 1))
            return px, py

        def capture_screenshot() -> str:
            res = computer.capture_screenshot(options.session_id)
            b64 = base64.b64encode(res.read()).decode()
            return f"data:image/png;base64,{b64}"

        async def execute_function(name: str, args: dict) -> None:
            if name == "click":
                px, py = scale(args["x"], args["y"])
                computer.click_mouse(
                    options.session_id, x=px, y=py, button=args.get("button", "left"),
                )
            elif name == "double_click":
                px, py = scale(args["x"], args["y"])
                computer.click_mouse(options.session_id, x=px, y=py, num_clicks=2)
            elif name == "point_and_type":
                px, py = scale(args["x"], args["y"])
                computer.click_mouse(options.session_id, x=px, y=py)
                await asyncio.sleep(0.3)
                computer.type_text(options.session_id, text=args["text"])
                if args.get("press_enter"):
                    await asyncio.sleep(0.1)
                    computer.press_key(options.session_id, keys=["Return"])
            elif name == "key":
                computer.press_key(
                    options.session_id, keys=[_map_key(args["keys"])],
                )
            elif name == "scroll":
                px, py = scale(args.get("x", 500), args.get("y", 500))
                dy = max(-10, min(10, int(args.get("dy", 3))))
                computer.scroll(
                    options.session_id, x=px, y=py, delta_x=0, delta_y=dy,
                )
            elif name == "drag":
                px1, py1 = scale(args["x1"], args["y1"])
                px2, py2 = scale(args["x2"], args["y2"])
                computer.drag_mouse(
                    options.session_id, path=[[px1, py1], [px2, py2]],
                )
            else:
                raise ValueError(f"Unknown function: {name}")

        def img(screenshot_url: str, text: str = "screenshot") -> dict:
            return {
                "role": "user",
                "content": [
                    {"type": "input_text", "text": text},
                    {"type": "input_image", "image_url": screenshot_url, "detail": "auto"},
                ],
            }

        screenshot_url = capture_screenshot()
        items: list[Any] = [img(screenshot_url, text=f"{options.query}\n\nCurrent screenshot:")]
        resp: Any = None

        for step in range(max_steps):
            # Prevent unbounded payload growth
            if len(items) > 30:
                items = items[:2] + items[-20:]

            resp = tzafon.responses.create(
                model=model, input=items, tools=TOOLS,
                instructions=INSTRUCTIONS,
                temperature=0, max_output_tokens=4096,
            )

            calls: list[tuple[str, str, dict]] = []
            for item in resp.output or []:
                if item.type == "message":
                    for block in item.content or []:
                        text = block.text or ""
                        if text:
                            items.append({"role": "assistant", "content": text})

                elif item.type == "function_call":
                    call_id = item.call_id
                    fn_name = item.name
                    raw_args = item.arguments or "{}"
                    try:
                        args = json.loads(raw_args) if isinstance(raw_args, str) else raw_args
                    except (json.JSONDecodeError, TypeError):
                        args = {}
                    calls.append((call_id, fn_name, args))
                    items.append({
                        "type": "function_call", "call_id": call_id, "name": fn_name,
                        "arguments": raw_args if isinstance(raw_args, str) else json.dumps(raw_args),
                    })

            if not calls:
                continue

            for call_id, fn_name, args in calls:
                if fn_name == "done":
                    return TaskResult(result=args.get("result", ""), provider=self.name)

                try:
                    await execute_function(fn_name, args)
                except Exception as e:
                    items.append({"type": "function_call_output", "call_id": call_id, "output": f"Error: {e}"})
                    continue

                await asyncio.sleep(0.5)
                screenshot_url = capture_screenshot()

                # Replace old screenshots with placeholders
                for it in items[:-1]:
                    c = it.get("content") if isinstance(it, dict) else None
                    if isinstance(c, list):
                        has_img = any(isinstance(p, dict) and p.get("type") == "input_image" for p in c)
                        if has_img:
                            it["content"] = [
                                p for p in c if not (isinstance(p, dict) and p.get("type") == "input_image")
                            ] or "(old screenshot)"

                items.append({"type": "function_call_output", "call_id": call_id, "output": "[screenshot]"})
                items.append(img(screenshot_url))

        messages: list[str] = []
        if resp:
            for item in resp.output or []:
                if item.type == "message":
                    for block in item.content or []:
                        if block.text:
                            messages.append(block.text)

        return TaskResult(
            result=" ".join(messages) if messages else "(max iterations reached)",
            provider=self.name,
        )
