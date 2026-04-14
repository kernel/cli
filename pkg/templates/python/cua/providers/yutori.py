"""Yutori CUA provider adapter using the n1-latest model.

Uses an OpenAI-compatible API with tool_calls. Coordinates are returned in
1000x1000 space and scaled to viewport dimensions. Screenshots are converted
to WebP for better compression.

@see https://docs.yutori.com/reference/n1
"""

from __future__ import annotations

import asyncio
import base64
import json
import os
from io import BytesIO

from openai import OpenAI
from PIL import Image

from . import CuaProvider, TaskOptions, TaskResult

DEFAULT_MODEL = "n1-latest"
TYPING_DELAY_MS = 12
SCREENSHOT_DELAY_S = 0.3
ACTION_DELAY_S = 0.3

KEY_MAP = {
    "Enter": "Return", "Escape": "Escape", "Backspace": "BackSpace",
    "Tab": "Tab", "Delete": "Delete",
    "ArrowUp": "Up", "ArrowDown": "Down", "ArrowLeft": "Left", "ArrowRight": "Right",
    "Home": "Home", "End": "End", "PageUp": "Page_Up", "PageDown": "Page_Down",
    **{f"F{i}": f"F{i}" for i in range(1, 13)},
}

MODIFIER_MAP = {
    "control": "ctrl", "ctrl": "ctrl", "alt": "alt", "shift": "shift",
    "meta": "super", "command": "super", "cmd": "super",
}


def _map_key(key: str) -> str:
    if "+" in key:
        parts = key.split("+")
        mapped = []
        for part in parts:
            trimmed = part.strip()
            lower = trimmed.lower()
            if lower in MODIFIER_MAP:
                mapped.append(MODIFIER_MAP[lower])
            else:
                mapped.append(KEY_MAP.get(trimmed, trimmed))
        return "+".join(mapped)
    return KEY_MAP.get(key, key)


class YutoriProvider:
    name = "yutori"

    def __init__(self) -> None:
        self._api_key = os.environ.get("YUTORI_API_KEY", "")

    def is_configured(self) -> bool:
        return len(self._api_key) > 0

    async def run_task(self, options: TaskOptions) -> TaskResult:
        model = options.model or DEFAULT_MODEL
        client = OpenAI(api_key=self._api_key, base_url="https://api.yutori.com/v1")
        computer = options.kernel.browsers.computer
        width = options.viewport_width
        height = options.viewport_height
        max_iterations = 50

        def capture_screenshot() -> str:
            res = computer.capture_screenshot(options.session_id)
            png_bytes = res.read()
            img = Image.open(BytesIO(png_bytes))
            webp_buf = BytesIO()
            img.save(webp_buf, "WEBP", quality=80)
            return base64.b64encode(webp_buf.getvalue()).decode("utf-8")

        def scale_coords(coords: list) -> list:
            return [
                round((coords[0] / 1000) * width),
                round((coords[1] / 1000) * height),
            ]

        def get_coords(coords: list | None) -> dict:
            if coords is None or len(coords) != 2:
                return {"x": width // 2, "y": height // 2}
            return {"x": int(coords[0]), "y": int(coords[1])}

        async def execute_action(action: dict) -> str | None:
            action_type = action.get("action_type")

            if action_type in ("left_click", "double_click", "triple_click", "right_click"):
                c = get_coords(action.get("coordinates"))
                button = "right" if action_type == "right_click" else "left"
                num = 2 if action_type == "double_click" else 3 if action_type == "triple_click" else 1
                computer.click_mouse(
                    options.session_id, x=c["x"], y=c["y"],
                    button=button, click_type="click", num_clicks=num,
                )
                await asyncio.sleep(SCREENSHOT_DELAY_S)
                return capture_screenshot()

            elif action_type == "scroll":
                c = get_coords(action.get("coordinates"))
                notches = max(action.get("amount", 3), 1)
                direction = action.get("direction", "down")
                dx = dy = 0
                if direction == "up": dy = -notches
                elif direction == "down": dy = notches
                elif direction == "left": dx = -notches
                elif direction == "right": dx = notches
                computer.scroll(
                    options.session_id, x=c["x"], y=c["y"], delta_x=dx, delta_y=dy,
                )
                await asyncio.sleep(SCREENSHOT_DELAY_S)
                return capture_screenshot()

            elif action_type == "type":
                text = action.get("text")
                if not text:
                    raise ValueError("text is required for type action")
                if action.get("clear_before_typing"):
                    computer.press_key(options.session_id, keys=["ctrl+a"])
                    await asyncio.sleep(0.1)
                    computer.press_key(options.session_id, keys=["BackSpace"])
                    await asyncio.sleep(0.1)
                computer.type_text(options.session_id, text=text, delay=TYPING_DELAY_MS)
                if action.get("press_enter_after"):
                    await asyncio.sleep(0.1)
                    computer.press_key(options.session_id, keys=["Return"])
                await asyncio.sleep(SCREENSHOT_DELAY_S)
                return capture_screenshot()

            elif action_type == "key_press":
                key_comb = action.get("key_comb")
                if not key_comb:
                    raise ValueError("key_comb is required for key_press action")
                computer.press_key(options.session_id, keys=[_map_key(key_comb)])
                await asyncio.sleep(SCREENSHOT_DELAY_S)
                return capture_screenshot()

            elif action_type == "hover":
                c = get_coords(action.get("coordinates"))
                computer.move_mouse(options.session_id, x=c["x"], y=c["y"])
                await asyncio.sleep(SCREENSHOT_DELAY_S)
                return capture_screenshot()

            elif action_type == "drag":
                start = get_coords(action.get("start_coordinates"))
                end = get_coords(action.get("coordinates"))
                computer.drag_mouse(
                    options.session_id,
                    path=[[start["x"], start["y"]], [end["x"], end["y"]]],
                    button="left",
                )
                await asyncio.sleep(SCREENSHOT_DELAY_S)
                return capture_screenshot()

            elif action_type == "wait":
                await asyncio.sleep(2)
                return capture_screenshot()

            elif action_type == "refresh":
                computer.press_key(options.session_id, keys=["F5"])
                await asyncio.sleep(2)
                return capture_screenshot()

            elif action_type == "go_back":
                computer.press_key(options.session_id, keys=["alt+Left"])
                await asyncio.sleep(1.5)
                return capture_screenshot()

            elif action_type == "goto_url":
                url = action.get("url")
                if not url:
                    raise ValueError("url is required for goto_url action")
                computer.press_key(options.session_id, keys=["ctrl+l"])
                await asyncio.sleep(ACTION_DELAY_S)
                computer.press_key(options.session_id, keys=["ctrl+a"])
                await asyncio.sleep(0.1)
                computer.type_text(options.session_id, text=url, delay=TYPING_DELAY_MS)
                await asyncio.sleep(ACTION_DELAY_S)
                computer.press_key(options.session_id, keys=["Return"])
                await asyncio.sleep(2)
                return capture_screenshot()

            else:
                raise ValueError(f"Unknown action type: {action_type}")

        # Take initial screenshot
        initial_screenshot = capture_screenshot()
        conversation: list[dict] = [
            {
                "role": "user",
                "content": [
                    {"type": "text", "text": options.query},
                    {"type": "image_url", "image_url": {"url": f"data:image/webp;base64,{initial_screenshot}"}},
                ],
            }
        ]

        for _iteration in range(max_iterations):
            response = client.chat.completions.create(
                model=model,
                messages=conversation,
                max_completion_tokens=4096,
                temperature=0.3,
            )

            choice = response.choices[0]
            if not choice.message:
                raise ValueError("No response from model")

            conversation.append(choice.message.model_dump(exclude_none=True))

            tool_calls = choice.message.tool_calls
            if not tool_calls:
                return TaskResult(
                    result=choice.message.content or "(no response)",
                    provider=self.name,
                )

            for tc in tool_calls:
                try:
                    args = json.loads(tc.function.arguments)
                except json.JSONDecodeError:
                    conversation.append({
                        "role": "tool", "tool_call_id": tc.id,
                        "content": "Error: failed to parse arguments",
                    })
                    continue

                action = {"action_type": tc.function.name, **args}

                # Scale coordinates from 1000x1000 to viewport
                if action.get("coordinates"):
                    action["coordinates"] = scale_coords(action["coordinates"])
                if action.get("start_coordinates"):
                    action["start_coordinates"] = scale_coords(action["start_coordinates"])

                try:
                    screenshot = await execute_action(action)
                    if screenshot:
                        conversation.append({
                            "role": "tool", "tool_call_id": tc.id,
                            "content": [
                                {"type": "image_url", "image_url": {"url": f"data:image/webp;base64,{screenshot}"}},
                            ],
                        })
                    else:
                        conversation.append({
                            "role": "tool", "tool_call_id": tc.id, "content": "OK",
                        })
                except Exception as e:
                    conversation.append({
                        "role": "tool", "tool_call_id": tc.id,
                        "content": f"Action failed: {e}",
                    })

        return TaskResult(result="(max iterations reached)", provider=self.name)
