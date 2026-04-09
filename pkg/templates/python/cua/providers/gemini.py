"""Gemini CUA provider adapter using the Google GenAI SDK."""

from __future__ import annotations

import asyncio
import base64
import os
from datetime import datetime

from google import genai
from google.genai.types import (
    Content,
    GenerateContentConfig,
    Part,
    ThinkingConfig,
    Tool,
    ComputerUse,
    Environment,
)

from . import CuaProvider, TaskOptions, TaskResult

COORDINATE_SCALE = 1000
DEFAULT_WIDTH = 1200
DEFAULT_HEIGHT = 800

def _system_prompt() -> str:
    date = datetime.now().strftime("%A, %B %d, %Y")
    return (
        "You are a helpful assistant that can use a web browser.\n"
        "You are operating a Chrome browser through computer use tools.\n"
        "The browser is already open and ready for use.\n"
        "When you need to navigate to a page, use the navigate action.\n"
        "After each action, carefully evaluate the screenshot.\n"
        f"Current date: {date}."
    )


class GeminiProvider:
    name = "gemini"

    def __init__(self) -> None:
        self._api_key = os.environ.get("GOOGLE_API_KEY", "")

    def is_configured(self) -> bool:
        return len(self._api_key) > 0

    async def run_task(self, options: TaskOptions) -> TaskResult:
        width = options.viewport_width or DEFAULT_WIDTH
        height = options.viewport_height or DEFAULT_HEIGHT
        client = genai.Client(api_key=self._api_key)
        model = options.model or "gemini-2.5-computer-use-preview-10-2025"

        contents: list[Content] = [
            Content(role="user", parts=[Part(text=options.query)]),
        ]

        for _i in range(50):
            response = await asyncio.to_thread(
                client.models.generate_content,
                model=model,
                contents=contents,
                config=GenerateContentConfig(
                    temperature=1,
                    top_p=0.95,
                    top_k=40,
                    max_output_tokens=8192,
                    system_instruction=_system_prompt(),
                    tools=[Tool(computer_use=ComputerUse(environment=Environment.ENVIRONMENT_BROWSER))],
                    thinking_config=ThinkingConfig(include_thoughts=True),
                ),
            )

            if not response.candidates or not response.candidates[0].content:
                break

            candidate = response.candidates[0]
            contents.append(candidate.content)

            # Extract text and function calls
            text_parts = [
                p.text for p in (candidate.content.parts or [])
                if hasattr(p, "text") and p.text
            ]
            function_calls = [
                p.function_call for p in (candidate.content.parts or [])
                if hasattr(p, "function_call") and p.function_call
            ]

            if not function_calls:
                return TaskResult(
                    result=" ".join(text_parts) or "(no response)",
                    provider=self.name,
                )

            # Execute function calls
            responses: list[Part] = []
            for fc in function_calls:
                if not fc.name:
                    continue
                args = dict(fc.args) if fc.args else {}

                safety = args.get("safety_decision", {})
                if isinstance(safety, dict) and safety.get("decision") == "require_confirmation":
                    print(f"Safety check: {safety.get('explanation', '')}")

                result = await self._execute_action(
                    options, fc.name, args, width, height,
                )

                if result.get("error"):
                    responses.append(Part.from_function_response(
                        name=fc.name,
                        response={"error": result["error"], "url": "about:blank"},
                    ))
                else:
                    responses.append(Part.from_function_response(
                        name=fc.name,
                        response={"url": result.get("url", "about:blank")},
                    ))
                    if result.get("screenshot"):
                        responses.append(Part(inline_data={
                            "mime_type": "image/png",
                            "data": result["screenshot"],
                        }))

            contents.append(Content(role="user", parts=responses))

        return TaskResult(result="(max iterations reached)", provider=self.name)

    def _denorm(self, value: float | None, dimension: int) -> int:
        if value is None:
            return 0
        return round((value / COORDINATE_SCALE) * dimension)

    async def _execute_action(
        self,
        options: TaskOptions,
        name: str,
        args: dict,
        width: int,
        height: int,
    ) -> dict:
        computer = options.kernel.browsers.computer

        try:
            if name == "click_at":
                x = self._denorm(args.get("x"), width)
                y = self._denorm(args.get("y"), height)
                await asyncio.to_thread(computer.click_mouse, options.session_id, x=x, y=y)

            elif name == "hover_at":
                x = self._denorm(args.get("x"), width)
                y = self._denorm(args.get("y"), height)
                await asyncio.to_thread(computer.move_mouse, options.session_id, x=x, y=y)

            elif name == "type_text_at":
                x = self._denorm(args.get("x"), width)
                y = self._denorm(args.get("y"), height)
                await asyncio.to_thread(computer.click_mouse, options.session_id, x=x, y=y)
                text = args.get("text", "")
                if text:
                    await asyncio.to_thread(computer.type_text, options.session_id, text=text)

            elif name in ("scroll_document", "scroll_at"):
                if name == "scroll_at":
                    x = self._denorm(args.get("x"), width)
                    y = self._denorm(args.get("y"), height)
                else:
                    x, y = width // 2, height // 2
                magnitude = args.get("magnitude", 3)
                direction = args.get("direction", "down")
                dy = -magnitude if direction == "up" else magnitude if direction == "down" else 0
                dx = -magnitude if direction == "left" else magnitude if direction == "right" else 0
                await asyncio.to_thread(
                    computer.scroll, options.session_id, x=x, y=y, delta_x=dx, delta_y=dy,
                )

            elif name == "wait_5_seconds":
                await asyncio.sleep(5)

            elif name == "go_back":
                await asyncio.to_thread(
                    computer.press_key, options.session_id, keys=["Left"], hold_keys=["Alt_L"],
                )

            elif name == "go_forward":
                await asyncio.to_thread(
                    computer.press_key, options.session_id, keys=["Right"], hold_keys=["Alt_L"],
                )

            elif name in ("navigate", "search"):
                url = args.get("url") or args.get("text", "")
                await asyncio.to_thread(
                    computer.batch, options.session_id, actions=[
                        {"type": "press_key", "press_key": {"keys": ["l"], "hold_keys": ["Control_L"]}},
                        {"type": "sleep", "sleep": {"duration_ms": 200}},
                        {"type": "press_key", "press_key": {"keys": ["a"], "hold_keys": ["Control_L"]}},
                        {"type": "type_text", "type_text": {"text": url}},
                        {"type": "press_key", "press_key": {"keys": ["Return"]}},
                    ],
                )
                await asyncio.sleep(1.5)

            elif name == "key_combination":
                combo = args.get("key_combination", "")
                parts = [k.strip() for k in combo.split("+")]
                hold_keys = parts[:-1] if len(parts) > 1 else []
                keys = parts[-1:] if parts else []
                kwargs: dict = {"keys": keys or parts}
                if hold_keys:
                    kwargs["hold_keys"] = hold_keys
                await asyncio.to_thread(
                    computer.press_key, options.session_id, **kwargs,
                )

            elif name == "drag_and_drop":
                sx = self._denorm(args.get("start_x"), width)
                sy = self._denorm(args.get("start_y"), height)
                ex = self._denorm(args.get("end_x"), width)
                ey = self._denorm(args.get("end_y"), height)
                await asyncio.to_thread(
                    computer.drag_mouse, options.session_id, path=[[sx, sy], [ex, ey]],
                )

            elif name == "open_web_browser":
                pass

            else:
                return {"error": f"Unknown action: {name}"}

            # Screenshot after every action
            await asyncio.sleep(0.5)
            resp = await asyncio.to_thread(
                computer.capture_screenshot, options.session_id,
            )
            screenshot = base64.b64encode(resp.read()).decode()
            return {"screenshot": screenshot, "url": "about:blank"}

        except Exception as exc:
            return {"error": str(exc)}
