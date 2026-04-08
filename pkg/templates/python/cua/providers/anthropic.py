"""Anthropic CUA provider adapter using Claude computer-use API."""

from __future__ import annotations

import asyncio
import base64
import os
from datetime import datetime

import anthropic

from . import CuaProvider, TaskOptions, TaskResult

SYSTEM_PROMPT = """<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* After each step, take a screenshot and carefully evaluate if you have achieved the right outcome.
* Only when you confirm a step was executed correctly should you move on to the next one.
* The current date is {date}.
</SYSTEM_CAPABILITY>

<IMPORTANT>
* When using Chromium, if a startup wizard appears, IGNORE IT.
* Click on the search bar and enter the appropriate URL there.
</IMPORTANT>"""

KEY_MAP = {
    "Return": "Return", "Enter": "Return", "Backspace": "BackSpace",
    "Tab": "Tab", "Escape": "Escape", "space": "space", "Space": "space",
    "Up": "Up", "Down": "Down", "Left": "Left", "Right": "Right",
    "Home": "Home", "End": "End", "Page_Up": "Prior", "Page_Down": "Next",
    "ctrl": "Control_L", "Control_L": "Control_L",
    "alt": "Alt_L", "Alt_L": "Alt_L",
    "shift": "Shift_L", "Shift_L": "Shift_L",
    "super": "Super_L", "Super_L": "Super_L",
}


def _map_key(key: str) -> str:
    if "+" in key:
        return "+".join(KEY_MAP.get(k.strip(), k.strip()) for k in key.split("+"))
    return KEY_MAP.get(key, key)


class AnthropicProvider:
    name = "anthropic"

    def __init__(self) -> None:
        self._api_key = os.environ.get("ANTHROPIC_API_KEY", "")

    def is_configured(self) -> bool:
        return len(self._api_key) > 0

    async def run_task(self, options: TaskOptions) -> TaskResult:
        client = anthropic.Anthropic(api_key=self._api_key, max_retries=4)
        model = "claude-sonnet-4-6"
        messages: list[dict] = [{"role": "user", "content": options.query}]

        date_str = datetime.now().strftime("%A, %B %d, %Y")
        system_prompt = SYSTEM_PROMPT.format(date=date_str)

        while True:
            response = await asyncio.to_thread(
                client.beta.messages.create,
                max_tokens=4096,
                messages=messages,
                model=model,
                system=[{"type": "text", "text": system_prompt, "cache_control": {"type": "ephemeral"}}],
                tools=[{
                    "type": "computer_20251124",
                    "name": "computer",
                    "display_width_px": options.viewport_width,
                    "display_height_px": options.viewport_height,
                    "display_number": 1,
                }],
                betas=["computer-use-2025-01-24", "prompt-caching-2024-07-31"],
                thinking={"type": "enabled", "budget_tokens": 1024},
            )

            assistant_content = []
            for block in response.content:
                if block.type == "thinking":
                    assistant_content.append({
                        "type": "thinking",
                        "thinking": block.thinking,
                        "signature": block.signature,
                    })
                elif block.type == "text":
                    assistant_content.append({"type": "text", "text": block.text})
                elif block.type == "tool_use":
                    assistant_content.append({
                        "type": "tool_use",
                        "id": block.id,
                        "name": block.name,
                        "input": block.input,
                    })

            messages.append({"role": "assistant", "content": assistant_content})

            if response.stop_reason == "end_turn":
                text = " ".join(
                    b.text for b in response.content if b.type == "text"
                )
                return TaskResult(result=text, provider=self.name)

            # Process tool calls
            tool_results = []
            for block in response.content:
                if block.type != "tool_use":
                    continue
                action = block.input.get("action", "")
                try:
                    screenshot = await self._execute_action(
                        options, action, block.input,
                    )
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block.id,
                        "content": [{
                            "type": "image",
                            "source": {
                                "type": "base64",
                                "media_type": "image/png",
                                "data": screenshot,
                            },
                        }],
                    })
                except Exception as exc:
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block.id,
                        "content": [{"type": "text", "text": f"Error: {exc}"}],
                        "is_error": True,
                    })

            if tool_results:
                messages.append({"role": "user", "content": tool_results})
            else:
                text = " ".join(
                    b.text for b in response.content if b.type == "text"
                )
                return TaskResult(result=text or "(no response)", provider=self.name)

    async def _execute_action(
        self, options: TaskOptions, action: str, params: dict
    ) -> str:
        computer = options.kernel.browsers.computer

        if action == "screenshot":
            pass
        elif action == "key":
            key = _map_key(params.get("key", ""))
            await asyncio.to_thread(
                computer.press_key, options.session_id, keys=[key]
            )
        elif action == "hold_key":
            key = _map_key(params.get("key", ""))
            duration = params.get("duration", 500)
            await asyncio.to_thread(
                computer.press_key, options.session_id,
                keys=[key], duration=duration,
            )
        elif action == "type":
            text = params.get("text", "")
            await asyncio.to_thread(
                computer.type_text, options.session_id, text=text,
            )
        elif action in ("left_click", "right_click", "middle_click"):
            x, y = params.get("coordinate", [0, 0])
            button = {"left_click": "left", "right_click": "right", "middle_click": "middle"}[action]
            await asyncio.to_thread(
                computer.click_mouse, options.session_id, x=x, y=y, button=button,
            )
        elif action == "double_click":
            x, y = params.get("coordinate", [0, 0])
            await asyncio.to_thread(
                computer.click_mouse, options.session_id, x=x, y=y, num_clicks=2,
            )
        elif action == "triple_click":
            x, y = params.get("coordinate", [0, 0])
            await asyncio.to_thread(
                computer.click_mouse, options.session_id, x=x, y=y, num_clicks=3,
            )
        elif action == "mouse_move":
            x, y = params.get("coordinate", [0, 0])
            await asyncio.to_thread(
                computer.move_mouse, options.session_id, x=x, y=y,
            )
        elif action == "left_click_drag":
            sx, sy = params.get("start_coordinate", [0, 0])
            ex, ey = params.get("coordinate", [0, 0])
            await asyncio.to_thread(
                computer.drag_mouse, options.session_id,
                path=[[sx, sy], [ex, ey]],
            )
        elif action == "scroll":
            x, y = params.get("coordinate", [0, 0])
            direction = params.get("direction", "down")
            amount = params.get("amount", 3)
            dx = -amount if direction == "left" else amount if direction == "right" else 0
            dy = -amount if direction == "up" else amount if direction == "down" else 0
            await asyncio.to_thread(
                computer.scroll, options.session_id,
                x=x, y=y, delta_x=dx, delta_y=dy,
            )
        elif action == "wait":
            duration = params.get("duration", 1000)
            await asyncio.sleep(duration / 1000)
        elif action == "cursor_position":
            pass
        else:
            raise ValueError(f"Unknown action: {action}")

        # Screenshot after every action
        await asyncio.sleep(0.5)
        resp = await asyncio.to_thread(
            computer.capture_screenshot, options.session_id,
        )
        return base64.b64encode(resp.read()).decode()
