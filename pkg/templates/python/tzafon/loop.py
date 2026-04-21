"""
Tzafon Northstar Sampling Loop

Runs the Northstar CUA model via the Lightcone Responses API using explicit
function tools (click, type, key, scroll, drag, done). Full conversation
history is maintained in the input array — each tool result includes a fresh
screenshot so the model always sees the current screen state.

@see https://docs.lightcone.ai
"""

import asyncio
import json
from typing import Any
from kernel import Kernel
from tzafon import Lightcone

from tools import ComputerTool

MODEL = "tzafon.northstar-cua-fast"

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


def _img(screenshot_url: str, text: str = "screenshot") -> dict:
    return {
        "role": "user",
        "content": [
            {"type": "input_text", "text": text},
            {"type": "input_image", "image_url": screenshot_url, "detail": "auto"},
        ],
    }


async def sampling_loop(
    *,
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    model: str = MODEL,
    max_steps: int = 50,
    viewport_width: int = 1280,
    viewport_height: int = 800,
) -> dict[str, Any]:
    """Run the Northstar CUA loop until the model calls done() or max steps."""
    tzafon = Lightcone(api_key=api_key)
    computer = ComputerTool(kernel, session_id, viewport_width, viewport_height)

    screenshot_url = computer.capture_screenshot()
    items: list[Any] = [_img(screenshot_url, text=f"{task}\n\nCurrent screenshot:")]
    resp: Any = None

    for step in range(max_steps):
        print(f"\n=== Step {step + 1}/{max_steps} ===")

        # Prevent unbounded payload growth — keep the task prompt + recent history
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
                        print(f"  Model: {text[:150]}")

            elif item.type == "function_call":
                call_id = item.call_id
                name = item.name
                raw_args = item.arguments or "{}"
                try:
                    args = json.loads(raw_args) if isinstance(raw_args, str) else raw_args
                except (json.JSONDecodeError, TypeError):
                    args = {}
                calls.append((call_id, name, args))
                items.append({
                    "type": "function_call", "call_id": call_id, "name": name,
                    "arguments": raw_args if isinstance(raw_args, str) else json.dumps(raw_args),
                })

        if not calls:
            continue

        for call_id, name, args in calls:
            print(f"  [{step + 1}] {name}({json.dumps(args)[:100]})")

            if name == "done":
                result = args.get("result", "")
                items.append({"type": "function_call_output", "call_id": call_id, "output": "ok"})
                print(f"  Done: {result}")
                return {"messages": [], "final_result": result}

            try:
                await computer.execute_function(name, args)
            except Exception as e:
                print(f"  Action failed: {e}")
                items.append({"type": "function_call_output", "call_id": call_id, "output": f"Error: {e}"})
                continue

            await asyncio.sleep(0.5)
            screenshot_url = computer.capture_screenshot()

            # Replace old screenshots with placeholders to save payload space
            for it in items[:-1]:
                c = it.get("content") if isinstance(it, dict) else None
                if isinstance(c, list):
                    has_img = any(isinstance(p, dict) and p.get("type") == "input_image" for p in c)
                    if has_img:
                        it["content"] = [p for p in c if not (isinstance(p, dict) and p.get("type") == "input_image")] or "(old screenshot)"

            items.append({"type": "function_call_output", "call_id": call_id, "output": "[screenshot]"})
            items.append(_img(screenshot_url))

    messages: list[str] = []
    if resp:
        for item in resp.output or []:
            if item.type == "message":
                for block in item.content or []:
                    if block.text:
                        messages.append(block.text)

    return {"messages": messages, "final_result": None}
